// peer.go — sending prompts to another machine on the local network.
//
// cats' control socket is a unix socket: it reaches the cats on *this*
// machine and nothing else, by design. So "the other box on the desk" is not
// something the existing client can be pointed at — it needs a service of its
// own, and this is it: `cats-todo serve` opens a small HTTP endpoint that
// speaks bundles (bundle.go), and the manager's export and import pickers list
// the machines that answer.
//
//	GET  /v1/hello                → who this is, and which backlogs it offers
//	GET  /v1/bundle?backlog=<id>  → that backlog as a bundle
//	POST /v1/bundle               → take this bundle into the inbox backlog
//
// Three rules hold the security model up, and each of them is a refusal rather
// than a warning:
//
//  1. **A token is required.** It lives in ~/.config/cats-todo/settings.json,
//     is generated on the first `serve`, and has to be on every request as
//     `Authorization: Bearer …`. A port with no token in front of it is a
//     stranger's write access to your backlog, so a serve with no token
//     refuses to start rather than opening one. (cathost in cats takes the
//     same line for the same reason — see cmd/cathost/listen.go there.)
//  2. **Local network only.** A request from outside this machine's own
//     private ranges is refused, because "a machine on the local network" is
//     the entire feature; `--allow-remote` is there for the person who
//     genuinely tunnelled in and knows it.
//  3. **Nothing that arrives is ever run.** A bundle becomes rows in a
//     backlog. Schedules are stripped on the way in (importBundle), attachment
//     names are reduced to a bare file name (safeAttachmentName), and sizes
//     are capped. Getting a prompt into someone's list is not the same as
//     getting it into their agent, and the distance between those two is a
//     keystroke they make themselves.
package main

import (
	"bytes"
	crand "crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
)

// peerDefaultPort is where `cats-todo serve` listens unless told otherwise, and
// what a host typed without one is assumed to mean. 8422 is cathost's port plus
// nothing in particular — it is simply unclaimed and next door to a number this
// family of tools already uses.
const peerDefaultPort = 8422

// peerTimeout bounds every request to another machine. The picker calls these
// on the UI thread (like the rest of export), so this is also how long the
// manager can appear to hang: long enough for a sleepy laptop to answer, short
// enough that a machine which has gone away is a message rather than a freeze.
const peerTimeout = 5 * time.Second

// peer is a machine that answered — from the beacon, or remembered in
// settings.
type peer struct {
	name string
	addr string // host:port
	// note is why the row is greyed, when it is: a remembered peer that did not
	// answer this time still shows, because "the studio is asleep" is more use
	// than an empty list.
	note string
}

// describe is the peer row's description in a picker.
func (p peer) describe() string {
	if p.note != "" {
		return p.addr + " · " + p.note
	}
	return p.addr
}

// peerHelloInfo is what /v1/hello answers: who is there, and what they hold.
type peerHelloInfo struct {
	Name     string           `json:"name"`
	Host     string           `json:"host"`
	Version  string           `json:"version"`
	Backlogs []peerBacklogRef `json:"backlogs"`
}

// peerBacklogRef is one backlog a peer offers.
type peerBacklogRef struct {
	ID    string `json:"id"`    // "project" or "global"
	Label string `json:"label"` // the project's name, or "global"
	Open  int    `json:"open"`  // how many prompts are still open
}

// --- The client -----------------------------------------------------------------

// peerURL builds a request URL for a peer address, filling in the default port
// when the user gave only a host.
func peerURL(addr, path string, query url.Values) (string, error) {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return "", errors.New("no address")
	}
	addr = strings.TrimPrefix(strings.TrimPrefix(addr, "http://"), "https://")
	addr = strings.TrimSuffix(addr, "/")
	if _, _, err := net.SplitHostPort(addr); err != nil {
		addr = net.JoinHostPort(addr, strconv.Itoa(peerDefaultPort))
	}
	u := "http://" + addr + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	return u, nil
}

// peerRequest performs one request against a peer, with the token attached and
// the timeout applied. It is the whole client transport, the way ctlproto.Call
// is for the control socket: build, send, read, close.
func peerRequest(method, addr, path string, query url.Values, body []byte) ([]byte, string, error) {
	u, err := peerURL(addr, path, query)
	if err != nil {
		return nil, "", err
	}
	var rdr io.Reader
	if body != nil {
		rdr = bytes.NewReader(body)
	}
	req, err := http.NewRequest(method, u, rdr)
	if err != nil {
		return nil, "", err
	}
	if tok := loadSettings().peerToken; tok != "" {
		req.Header.Set("Authorization", "Bearer "+tok)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/octet-stream")
	}
	client := &http.Client{Timeout: peerTimeout}
	resp, err := client.Do(req)
	if err != nil {
		return nil, "", peerDialError(addr, err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxBundleBytes+1))
	if err != nil {
		return nil, "", err
	}
	if resp.StatusCode != http.StatusOK {
		msg := strings.TrimSpace(string(data))
		if msg == "" {
			msg = resp.Status
		}
		return nil, "", errors.New(msg)
	}
	return data, resp.Header.Get("Content-Type"), nil
}

// peerDialError turns a transport failure into something worth reading. "no
// token" and "asleep" and "wrong port" all arrive as the same connection
// refused, and the first guess is nearly always that nothing is serving there.
func peerDialError(addr string, err error) error {
	var nerr net.Error
	if errors.As(err, &nerr) && nerr.Timeout() {
		return fmt.Errorf("%s did not answer within %s", addr, peerTimeout)
	}
	return fmt.Errorf("%s: %w (is `cats-todo serve` running there?)", addr, err)
}

// peerHello asks a peer who it is. Used to confirm a hand-typed address before
// anything is sent to it, and to label the row afterwards.
func peerHello(addr string) (peerHelloInfo, error) {
	data, _, err := peerRequest(http.MethodGet, addr, "/v1/hello", nil, nil)
	if err != nil {
		return peerHelloInfo{}, err
	}
	var info peerHelloInfo
	if err := json.Unmarshal(data, &info); err != nil {
		return peerHelloInfo{}, fmt.Errorf("%s answered, but not as a cats-todo", addr)
	}
	return info, nil
}

// peerFetch pulls a peer's backlog as a bundle. Which backlog: the one the peer
// offers first, which is its project's if it has one (see peerBacklogs) — the
// machine being asked knows better than this one which of its backlogs is the
// interesting one.
func peerFetch(addr string) (Bundle, bundleOpener, error) {
	data, _, err := peerRequest(http.MethodGet, addr, "/v1/bundle", nil, nil)
	if err != nil {
		return Bundle{}, nil, err
	}
	return readBundleBytes(data)
}

// peerSend posts a bundle to a peer's inbox.
func peerSend(addr string, data []byte) (string, error) {
	out, _, err := peerRequest(http.MethodPost, addr, "/v1/bundle", nil, data)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// --- The server -----------------------------------------------------------------

// peerServer holds what the handlers need: the backlogs to serve, and the rules
// to enforce.
type peerServer struct {
	name    string
	token   string
	project *store
	global  *store
	// inbox is the scope an arriving bundle lands in.
	inbox scope
	// allowRemote turns off the local-network check (rule 2 in the file
	// comment). Off unless the operator asked for it.
	allowRemote bool
}

// peerBacklogs is what this server offers, project first when there is one:
// the order is the answer to "which backlog did you mean" for a client that
// does not say.
func (ps *peerServer) peerBacklogs() []peerBacklogRef {
	var out []peerBacklogRef
	if ps.project != nil && ps.project.available() {
		out = append(out, peerBacklogRef{
			ID:    "project",
			Label: firstNonEmpty(baseName(backlogRoot(ps.project)), "project"),
			Open:  openCountOf(ps.project),
		})
	}
	if ps.global != nil && ps.global.available() {
		out = append(out, peerBacklogRef{ID: "global", Label: "global", Open: openCountOf(ps.global)})
	}
	return out
}

// openCountOf is how many prompts in a store are still open.
func openCountOf(s *store) int {
	n := 0
	for _, t := range s.todos {
		if !t.closed() {
			n++
		}
	}
	return n
}

// storeByID resolves a backlog id from the wire, defaulting to the first one
// offered.
func (ps *peerServer) storeByID(id string) (*store, error) {
	switch id {
	case "project":
		if ps.project != nil && ps.project.available() {
			return ps.project, nil
		}
		return nil, errors.New("this machine has no project backlog")
	case "global":
		if ps.global != nil && ps.global.available() {
			return ps.global, nil
		}
		return nil, errors.New("this machine has no global backlog")
	case "":
		if bs := ps.peerBacklogs(); len(bs) > 0 {
			return ps.storeByID(bs[0].ID)
		}
		return nil, errors.New("this machine has no backlog to offer")
	}
	return nil, fmt.Errorf("unknown backlog %q", id)
}

// handler builds the routes. A plain ServeMux: three paths, no middleware
// stack worth the name, and the two rules that do apply are one call each at
// the top of every handler — which is where a reader looking for "can this be
// reached from outside" should find them.
func (ps *peerServer) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/hello", func(w http.ResponseWriter, r *http.Request) {
		if !ps.allow(w, r) {
			return
		}
		host, _ := os.Hostname()
		writeJSON(w, peerHelloInfo{
			Name:     firstNonEmpty(ps.name, host, "cats-todo"),
			Host:     host,
			Version:  version,
			Backlogs: ps.peerBacklogs(),
		})
	})
	mux.HandleFunc("/v1/bundle", func(w http.ResponseWriter, r *http.Request) {
		if !ps.allow(w, r) {
			return
		}
		switch r.Method {
		case http.MethodGet:
			ps.serveBundle(w, r)
		case http.MethodPost:
			ps.receiveBundle(w, r)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})
	return mux
}

// serveBundle answers with a backlog as a bundle.
func (ps *peerServer) serveBundle(w http.ResponseWriter, r *http.Request) {
	s, err := ps.storeByID(r.URL.Query().Get("backlog"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	if err := s.reload(); err != nil {
		http.Error(w, "could not read the backlog: "+err.Error(), http.StatusInternalServerError)
		return
	}
	host, _ := os.Hostname()
	source := firstNonEmpty(baseName(backlogRoot(s)), "global")
	if s.scope == scopeGlobal {
		source = "global"
	}
	b, files, _ := buildBundle(s, s.todos, "cats-todo v"+version+" on "+firstNonEmpty(ps.name, host), source)
	data, ext, err := encodeBundle(b, files)
	if err != nil {
		http.Error(w, "could not build the bundle: "+err.Error(), http.StatusInternalServerError)
		return
	}
	ct := "application/json"
	if ext == bundleExtZip {
		ct = "application/zip"
	}
	w.Header().Set("Content-Type", ct)
	_, _ = w.Write(data)
}

// receiveBundle takes a posted bundle into the inbox backlog.
//
// It answers with the sentence the sender's status line shows, so what the
// sender reports is what actually happened here rather than an optimistic
// restatement of what it sent.
func (ps *peerServer) receiveBundle(w http.ResponseWriter, r *http.Request) {
	dst, err := ps.storeByID(scopeID(ps.inbox))
	if err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	data, err := io.ReadAll(io.LimitReader(r.Body, maxBundleBytes+1))
	if err != nil {
		http.Error(w, "could not read the bundle: "+err.Error(), http.StatusBadRequest)
		return
	}
	b, open, err := readBundleBytes(data)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	res, err := importBundle(dst, b, open, importOpts{})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	msg := fmt.Sprintf("%s landed in the %s backlog", promptWord(res.added), strings.ToLower(ps.inbox.String()))
	if res.skipped > 0 {
		msg += fmt.Sprintf(" · %d were already there", res.skipped)
	}
	if res.noFiles > 0 {
		msg += fmt.Sprintf(" · %d without their attachments", res.noFiles)
	}
	fmt.Fprintln(os.Stderr, "← "+msg)
	_, _ = io.WriteString(w, msg)
}

// scopeID is the wire name of a scope.
func scopeID(s scope) string {
	if s == scopeGlobal {
		return "global"
	}
	return "project"
}

// allow enforces the two rules that gate every request: the caller is on this
// machine's own network, and they hold the token. Both answer with the status
// that says which rule they failed, since an operator debugging their own setup
// is the person who reads these far more often than anyone else.
func (ps *peerServer) allow(w http.ResponseWriter, r *http.Request) bool {
	if !ps.allowRemote && !isLocalRequest(r.RemoteAddr) {
		http.Error(w, "cats-todo serve answers the local network only", http.StatusForbidden)
		return false
	}
	if ps.token == "" || !bearerMatches(r.Header.Get("Authorization"), ps.token) {
		w.Header().Set("WWW-Authenticate", "Bearer")
		http.Error(w, "a bearer token is required — see `cats-todo serve`", http.StatusUnauthorized)
		return false
	}
	return true
}

// bearerMatches compares the presented credential with the configured one.
// Length-independent equality is not worth reaching for here — the token is a
// LAN secret, not a password database — but a constant-time compare costs one
// import and removes the question.
func bearerMatches(header, token string) bool {
	got, ok := strings.CutPrefix(header, "Bearer ")
	if !ok {
		return false
	}
	return subtleEqual(strings.TrimSpace(got), token)
}

// isLocalRequest reports whether a request came from this machine or from an
// address in one of the private ranges — which is what "the local network"
// means in practice, without asking the operator to describe their subnet.
func isLocalRequest(remoteAddr string) bool {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		host = remoteAddr
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast()
}

// writeJSON is the one response shape the server has besides bytes.
func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

// listenPeer opens the listener and serves until the process is stopped. Split
// from runPeerServer so a test can hand it a listener on port 0 and learn where
// it landed.
func listenPeer(ps *peerServer, addr string) (net.Listener, *http.Server, error) {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, nil, err
	}
	srv := &http.Server{
		Handler: ps.handler(),
		// A bundle can be tens of megabytes over a slow link; the read timeout
		// has to allow for that, while the header timeout stays short so a
		// half-open connection cannot hold a slot indefinitely.
		ReadHeaderTimeout: 10 * time.Second,
		WriteTimeout:      2 * time.Minute,
	}
	go func() { _ = srv.Serve(ln) }()
	return ln, srv, nil
}

// --- The "which machine" prompt --------------------------------------------------

// peerAddrPurpose is which picker asked for an address, and therefore what
// happens once one is typed.
type peerAddrPurpose int

const (
	peerAddrForExport peerAddrPurpose = iota // send the export's subject there
	peerAddrForImport                        // pull that machine's backlog
)

// beginPeerAddr opens the one-line host prompt. It is its own stage rather than
// a row that reads the picker's filter box: the filter is a filter, and a
// screen that quietly reinterpreted what was typed into it as an address would
// be the kind of cleverness nobody can undo when it guesses wrong.
func (m model) beginPeerAddr(purpose peerAddrPurpose) (tea.Model, tea.Cmd) {
	in := textinput.New()
	in.Placeholder = "hostname or 192.168.1.20 (:" + strconv.Itoa(peerDefaultPort) + " assumed)"
	in.SetWidth(max(m.width-4, 20))
	in.Focus()
	m.peerAddrInput = in
	m.peerAddrFor = purpose
	m.stage = stagePeerAddr
	return m, textinput.Blink
}

// updatePeerAddr is the prompt's key loop: esc goes back to the picker that
// asked, enter uses what was typed.
func (m model) updatePeerAddr(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		m.quitting = true
		return m, tea.Quit
	case "esc":
		if m.peerAddrFor == peerAddrForImport {
			m.stage = stageImport
		} else {
			m.stage = stageExport
		}
		return m, textinput.Blink
	case "enter":
		return m.usePeerAddr()
	}
	var cmd tea.Cmd
	m.peerAddrInput, cmd = m.peerAddrInput.Update(msg)
	return m, cmd
}

// usePeerAddr confirms the address before acting on it: a hello first, so a
// typo is "nothing is serving there" on this screen — where it can be fixed —
// rather than a failure reported from the list after the picker has closed.
// A machine that answers is remembered, so it has a row next time even before
// the beacon has spoken.
func (m model) usePeerAddr() (tea.Model, tea.Cmd) {
	addr := strings.TrimSpace(m.peerAddrInput.Value())
	if addr == "" {
		return m, nil
	}
	info, err := peerHello(addr)
	if err != nil {
		m.setStatus(err.Error(), true)
		return m, nil
	}
	name := firstNonEmpty(info.Name, addr)
	rememberPeer(name, addr)
	if m.peerAddrFor == peerAddrForImport {
		return m.importFromPeer(addr, name)
	}
	return m.sendToPeer(addr, name)
}

// viewPeerAddr draws the prompt: what it wants, the box, and the keys.
func (m model) viewPeerAddr() string {
	var b strings.Builder
	title := "Send to a machine"
	if m.peerAddrFor == peerAddrForImport {
		title = "Import from a machine"
	}
	b.WriteString(titleStyle.Render(title))
	b.WriteString("\n\n")
	b.WriteString(m.peerAddrInput.View())
	b.WriteString("\n\n")
	b.WriteString(footerStyle.Render(m.fitFooter([]string{
		"enter connect", "esc back",
		"the other machine runs `cats-todo serve` and shares its token",
	})))
	return b.String()
}

// rememberPeer records a machine in settings so it has a row next time. Best
// effort: an unwritable config directory must not turn a working transfer into
// a failure.
func rememberPeer(name, addr string) {
	set := loadSettings()
	for _, p := range set.peers {
		if p.Addr == addr {
			return
		}
	}
	set.peers = append(set.peers, settingsPeer{Name: name, Addr: addr})
	_ = set.save()
}

// sendToPeer posts the export's subject to a machine and lands back on the
// list with the answer the *other* end gave — what it says landed is the truth
// worth showing, not what this end sent.
func (m model) sendToPeer(addr, name string) (tea.Model, tea.Cmd) {
	b, files, dropped := m.subjectBundle()
	if len(b.Todos) == 0 {
		m.setStatus("nothing left to export — those prompts are gone", true)
		m.backToList()
		return m, nil
	}
	data, _, err := encodeBundle(b, files)
	if err != nil {
		m.finishExport()
		m.setStatus("could not build the bundle: "+err.Error(), true)
		return m, nil
	}
	reply, err := peerSend(addr, data)
	m.finishExport()
	if err != nil {
		m.setStatus("could not send to "+name+": "+err.Error(), true)
		return m, nil
	}
	note := name + ": " + firstNonEmpty(reply, promptWord(len(b.Todos))+" sent")
	if dropped > 0 {
		note += " · " + scheduleDropNote(dropped)
	}
	m.setStatus(note, false)
	return m, nil
}

// subtleEqual is a constant-time string comparison, so the token check cannot
// be timed. crypto/subtle wants byte slices; this is the one call site.
func subtleEqual(a, b string) bool {
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

// newPeerToken mints the shared secret `cats-todo serve` prints on first run:
// 32 hex characters from the cryptographic source, since this one — unlike a
// todo id — is a credential.
func newPeerToken() (string, error) {
	var b [16]byte
	if _, err := crand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}
