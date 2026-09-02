package main

import (
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// testPeer starts a server on the loopback with a known token and returns its
// address. The stores are real files under t.TempDir(), so what the handlers do
// to them is what they would do to a backlog.
func testPeer(t *testing.T, inbox scope) (addr string, project, global *store) {
	t.Helper()
	dir := t.TempDir()
	project = &store{scope: scopeProject, path: filepath.Join(dir, "proj", ".cats-todo", "todos.json")}
	global = &store{scope: scopeGlobal, path: filepath.Join(dir, "global", "todos.json")}
	for _, s := range []*store{project, global} {
		if err := s.save(); err != nil {
			t.Fatal(err)
		}
	}
	ps := &peerServer{name: "testbox", token: "s3cret", project: project, global: global, inbox: inbox}
	ln, srv, err := listenPeer(ps, "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = srv.Close() })
	return ln.Addr().String(), project, global
}

// withToken points the client's settings at a config directory holding the
// token the test server wants. The client reads it from settings on every
// request, which is the seam the tests have to move.
func withToken(t *testing.T, token string) {
	t.Helper()
	t.Setenv(configDirEnvVar, t.TempDir())
	set := loadSettings()
	set.peerToken = token
	if err := set.save(); err != nil {
		t.Fatal(err)
	}
}

func TestPeerRefusesWithoutTheToken(t *testing.T) {
	addr, _, _ := testPeer(t, scopeProject)
	withToken(t, "") // no token configured on this side

	if _, err := peerHello(addr); err == nil {
		t.Fatal("a request with no token should be refused")
	}
	withToken(t, "wrong")
	if _, err := peerHello(addr); err == nil {
		t.Fatal("a request with the wrong token should be refused")
	}

	// And the refusal is a 401 with a hint, not a silent close.
	resp, err := http.Get("http://" + addr + "/v1/hello")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "token") {
		t.Errorf("body = %q, want it to say what is missing", body)
	}
}

func TestPeerHelloAndFetch(t *testing.T) {
	addr, project, _ := testPeer(t, scopeProject)
	withToken(t, "s3cret")
	if err := project.add(Todo{ID: "p1", Title: "over there", Prompt: "the body"}); err != nil {
		t.Fatal(err)
	}

	info, err := peerHello(addr)
	if err != nil {
		t.Fatal(err)
	}
	if info.Name != "testbox" || info.Version != version {
		t.Errorf("hello = %+v", info)
	}
	if len(info.Backlogs) != 2 || info.Backlogs[0].ID != "project" || info.Backlogs[0].Open != 1 {
		t.Errorf("backlogs = %+v, want the project's first with its open count", info.Backlogs)
	}

	b, _, err := peerFetch(addr)
	if err != nil {
		t.Fatal(err)
	}
	if len(b.Todos) != 1 || b.Todos[0].Title != "over there" {
		t.Fatalf("fetched %+v", b.Todos)
	}
}

func TestPeerReceivesABundleIntoTheInbox(t *testing.T) {
	addr, project, global := testPeer(t, scopeGlobal)
	withToken(t, "s3cret")

	b, files, _ := buildBundle(nil, []Todo{
		{ID: "a", Title: "sent one", Prompt: "one"},
		{ID: "b", Title: "sent two", Prompt: "two"},
	}, "test", "elsewhere")
	data, _, err := encodeBundle(b, files)
	if err != nil {
		t.Fatal(err)
	}

	reply, err := peerSend(addr, data)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(reply, "2 prompts") || !strings.Contains(reply, "global") {
		t.Errorf("reply = %q, want it to say what landed where", reply)
	}
	if err := global.reload(); err != nil {
		t.Fatal(err)
	}
	if len(global.todos) != 2 {
		t.Fatalf("global holds %+v, want the two sent", global.todos)
	}
	if err := project.reload(); err != nil {
		t.Fatal(err)
	}
	if len(project.todos) != 0 {
		t.Errorf("the inbox was global; the project backlog should be untouched: %+v", project.todos)
	}

	// Sending the same bundle again lands nothing, and says so — the receiver's
	// own words, which is what the sender's status line shows.
	reply, err = peerSend(addr, data)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(reply, "already") {
		t.Errorf("second send = %q, want it to report the duplicates", reply)
	}
}

// A prompt that arrives carrying a schedule must not arm a timer here: the pane
// and cwd it names are on another machine.
func TestPeerStripsAnArrivingSchedule(t *testing.T) {
	addr, project, _ := testPeer(t, scopeProject)
	withToken(t, "s3cret")

	b := Bundle{Schema: 1, Todos: []Todo{{
		ID: "a", Title: "timed", Prompt: "p",
		Schedule: &Schedule{At: time.Now(), Kind: scheduleKindPane, Pane: 7},
	}}}
	data, err := json.Marshal(b)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := peerSend(addr, data); err != nil {
		t.Fatal(err)
	}
	if err := project.reload(); err != nil {
		t.Fatal(err)
	}
	if len(project.todos) != 1 {
		t.Fatalf("todos = %+v", project.todos)
	}
	if project.todos[0].Schedule != nil {
		t.Error("an arriving prompt must never be scheduled")
	}
}

func TestPeerRefusesOffNetworkRequests(t *testing.T) {
	ps := &peerServer{name: "x", token: "s3cret"}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/hello", nil)
	req.RemoteAddr = "203.0.113.9:5000" // a public address
	req.Header.Set("Authorization", "Bearer s3cret")

	if ps.allow(rec, req) {
		t.Fatal("a request from off the local network should be refused")
	}
	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403", rec.Code)
	}

	// …unless the operator deliberately asked for it.
	ps.allowRemote = true
	if !ps.allow(httptest.NewRecorder(), req) {
		t.Error("--allow-remote should let it through")
	}
}

func TestIsLocalRequest(t *testing.T) {
	local := []string{"127.0.0.1:1", "[::1]:1", "192.168.1.20:8422", "10.0.0.3:1", "172.16.4.4:1", "169.254.3.3:1"}
	remote := []string{"203.0.113.9:1", "8.8.8.8:1", "not-an-address", ""}
	for _, a := range local {
		if !isLocalRequest(a) {
			t.Errorf("isLocalRequest(%q) = false, want true", a)
		}
	}
	for _, a := range remote {
		if isLocalRequest(a) {
			t.Errorf("isLocalRequest(%q) = true, want false", a)
		}
	}
}

func TestPeerURLFillsInThePort(t *testing.T) {
	cases := map[string]string{
		"studio":                 "http://studio:8422/v1/hello",
		"studio:9000":            "http://studio:9000/v1/hello",
		"http://studio":          "http://studio:8422/v1/hello",
		"192.168.1.20":           "http://192.168.1.20:8422/v1/hello",
		"http://192.168.1.20:80": "http://192.168.1.20:80/v1/hello",
	}
	for in, want := range cases {
		got, err := peerURL(in, "/v1/hello", nil)
		if err != nil || got != want {
			t.Errorf("peerURL(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	if _, err := peerURL("", "/v1/hello", nil); err == nil {
		t.Error("an empty address should be an error")
	}
}

// A machine that is not there is an error a person can act on, rather than a
// transport failure quoted at them.
func TestPeerDialErrorSaysWhatToCheck(t *testing.T) {
	// Port 1 on the loopback: nothing listens there, and the connection is
	// refused immediately rather than after a timeout.
	_, err := peerHello("127.0.0.1:1")
	if err == nil {
		t.Fatal("want an error")
	}
	if !strings.Contains(err.Error(), "cats-todo serve") {
		t.Errorf("error = %q, want the hint about `cats-todo serve`", err)
	}
}

func TestBearerMatches(t *testing.T) {
	if !bearerMatches("Bearer abc", "abc") {
		t.Error("a matching bearer should pass")
	}
	for _, h := range []string{"", "abc", "Bearer ", "Bearer abcd", "Basic abc"} {
		if bearerMatches(h, "abc") {
			t.Errorf("bearerMatches(%q) = true, want false", h)
		}
	}
}

func TestNewPeerTokenIsUnique(t *testing.T) {
	a, err := newPeerToken()
	if err != nil {
		t.Fatal(err)
	}
	b, _ := newPeerToken()
	if a == b || len(a) != 32 {
		t.Errorf("tokens = %q %q, want two distinct 32-character strings", a, b)
	}
}

// mergePeers keeps a remembered machine that did not answer, with its note, and
// lets a discovered one replace its remembered row.
func TestMergePeers(t *testing.T) {
	known := []peer{
		{name: "studio", addr: "10.0.0.5:8422", note: "remembered — not answering yet"},
		{name: "asleep", addr: "10.0.0.9:8422", note: "remembered — not answering yet"},
	}
	found := []peer{{name: "studio", addr: "10.0.0.5:8422"}}

	got := mergePeers(known, found)
	if len(got) != 2 {
		t.Fatalf("merged = %+v, want two rows", got)
	}
	if got[0].addr != "10.0.0.5:8422" || got[0].note != "" {
		t.Errorf("the machine that answered should lead, without a note: %+v", got[0])
	}
	if got[1].name != "asleep" || got[1].note == "" {
		t.Errorf("the one that did not answer should keep its note: %+v", got[1])
	}
}

// The beacon answers a question with this machine's name and port. Multicast is
// not available everywhere a test suite runs (a sandbox, a container with no
// group membership), so an environment that cannot join the group skips rather
// than fails.
func TestBeaconAnswersAQuestion(t *testing.T) {
	closer, err := serveBeacon("testbox", 8477)
	if err != nil {
		t.Skipf("no multicast here: %v", err)
	}
	defer closer.Close()

	// The listen always runs its whole deadline, so this is time the suite
	// spends every run: long enough for a reply that has to cross the kernel's
	// multicast path, short enough not to be felt.
	found := discoverPeers(800 * time.Millisecond)
	for _, p := range found {
		if p.name == "testbox" {
			if _, port, _ := net.SplitHostPort(p.addr); port != "8477" {
				t.Errorf("peer = %+v, want the served port", p)
			}
			return
		}
	}
	t.Skipf("the beacon did not reach this host's own query (found %+v) — multicast loopback is off here", found)
}
