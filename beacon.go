// beacon.go — finding the other machines without asking the user for addresses.
//
// A picker that offered "type an IP address" and nothing else would be a
// feature nobody used twice. mDNS is the usual answer and it is a dependency
// (and a daemon, and a name conflict story); what this needs is much smaller
// than mDNS, so it is done directly.
//
// The exchange is a question, not a broadcast schedule:
//
//	the manager  →  multicast 239.255.42.99:45892   {"q":"cats-todo","v":1}
//	every server →  unicast back to the asker        {"cats-todo":1,"name":…,"port":…}
//
// Asking rather than listening is what makes the picker fast: a server that
// announced every few seconds would leave a picker opened between two
// announcements looking empty, and one that announced fast enough to avoid that
// would be chattering on the network all day for a screen nobody has open. A
// question is answered in milliseconds, so the picker fills in before the eye
// gets to it, and the network is silent the rest of the time.
//
// Multicast rather than broadcast: net.ListenMulticastUDP sets the address
// reuse a shared port needs, so a machine can run a server and a manager at
// once — which is the normal case, since the machine you are sending from is
// usually also one you send to.
//
// Nothing secret is in a datagram. A reply says a name, a port and a version;
// the token never leaves the config file it lives in, and it is the HTTP layer
// (peer.go) that decides whether the asker gets anything at all.
package main

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"strconv"
	"time"

	tea "charm.land/bubbletea/v2"
)

// The discovery group and port. Both are in the administratively scoped IPv4
// multicast block (239.0.0.0/8), which routers do not carry off the local
// network — so "the local network" is a property of the transport here, not
// only of the check in peer.go.
const (
	beaconGroup = "239.255.42.99"
	beaconPort  = 45892
)

// beaconWait is how long the manager listens for answers. Long enough for a
// machine across a switch to reply twice over, short enough to be invisible
// between a keystroke and a redraw.
const beaconWait = 700 * time.Millisecond

// beaconQuery is what the manager sends.
type beaconQuery struct {
	Q string `json:"q"`
	V int    `json:"v"`
}

// beaconReply is what a server answers with. The key is spelled out ("cats-todo")
// so a stray datagram from something else on the same group is rejected by
// shape rather than by hope.
type beaconReply struct {
	CatsTodo int    `json:"cats-todo"`
	Name     string `json:"name"`
	Port     int    `json:"port"`
	Version  string `json:"version"`
}

// discoverPeers asks the local network who is serving and collects the answers
// for wait. Every failure — no network, no permission to join a group, a
// firewall — comes back as an empty list rather than an error: discovery is a
// convenience, and a picker that showed an error where it should show rows
// would make a working feature look broken.
func discoverPeers(wait time.Duration) []peer {
	conn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4zero, Port: 0})
	if err != nil {
		return nil
	}
	defer conn.Close()

	msg, err := json.Marshal(beaconQuery{Q: "cats-todo", V: 1})
	if err != nil {
		return nil
	}
	group := &net.UDPAddr{IP: net.ParseIP(beaconGroup), Port: beaconPort}
	if _, err := conn.WriteToUDP(msg, group); err != nil {
		return nil
	}

	_ = conn.SetReadDeadline(time.Now().Add(wait))
	var found []peer
	seen := map[string]bool{}
	buf := make([]byte, 2048)
	for {
		n, from, err := conn.ReadFromUDP(buf)
		if err != nil {
			break // deadline, which is the normal way this loop ends
		}
		var rep beaconReply
		if json.Unmarshal(buf[:n], &rep) != nil || rep.CatsTodo != 1 || rep.Port <= 0 {
			continue
		}
		addr := net.JoinHostPort(from.IP.String(), strconv.Itoa(rep.Port))
		if seen[addr] {
			continue
		}
		seen[addr] = true
		found = append(found, peer{
			name: firstNonEmpty(rep.Name, from.IP.String()),
			addr: addr,
		})
	}
	return found
}

// peersMsg carries a finished discovery back into the model.
type peersMsg struct {
	peers []peer
}

// discoverPeersCmd runs a discovery off the UI thread. The picker opens with
// whatever is remembered and fills in when this lands — so a network that is
// slow, or absent, costs the screen nothing.
func discoverPeersCmd() tea.Cmd {
	return func() tea.Msg {
		return peersMsg{peers: discoverPeers(beaconWait)}
	}
}

// knownPeers is what a picker shows before a discovery has answered: the
// machines settings remembers, marked as unconfirmed. A peer that has gone to
// sleep is still worth a row — "the studio is not answering" is a more useful
// screen than an empty list — so the note says so rather than the row vanishing.
func knownPeers() []peer {
	set := loadSettings()
	out := make([]peer, 0, len(set.peers))
	for _, p := range set.peers {
		out = append(out, peer{name: p.Name, addr: p.Addr, note: "remembered — not answering yet"})
	}
	return out
}

// mergePeers folds a fresh discovery into the rows already on screen: a
// discovered machine replaces its remembered row (same address), and a
// remembered one that did not answer keeps its note.
func mergePeers(known, found []peer) []peer {
	out := make([]peer, 0, len(known)+len(found))
	live := make(map[string]bool, len(found))
	for _, p := range found {
		live[p.addr] = true
		out = append(out, p)
	}
	for _, p := range known {
		if !live[p.addr] {
			out = append(out, p)
		}
	}
	return out
}

// serveBeacon answers discovery questions for as long as the process runs. It
// is started beside the HTTP listener by `cats-todo serve`; a machine that
// cannot join the group still serves perfectly well over HTTP to anyone who
// knows its address, so a failure here is reported and shrugged off rather than
// fatal.
func serveBeacon(name string, port int) (io_Closer, error) {
	group := &net.UDPAddr{IP: net.ParseIP(beaconGroup), Port: beaconPort}
	conn, err := net.ListenMulticastUDP("udp4", nil, group)
	if err != nil {
		return nil, err
	}
	host, _ := os.Hostname()
	reply, err := json.Marshal(beaconReply{
		CatsTodo: 1,
		Name:     firstNonEmpty(name, host, "cats-todo"),
		Port:     port,
		Version:  version,
	})
	if err != nil {
		conn.Close()
		return nil, err
	}
	go func() {
		buf := make([]byte, 2048)
		for {
			n, from, err := conn.ReadFromUDP(buf)
			if err != nil {
				return // the connection was closed: the process is going down
			}
			var q beaconQuery
			if json.Unmarshal(buf[:n], &q) != nil || q.Q != "cats-todo" {
				continue
			}
			// Unicast back to the asker rather than to the group: only the
			// machine that asked has any use for the answer.
			if _, err := conn.WriteToUDP(reply, from); err != nil {
				fmt.Fprintln(os.Stderr, "beacon reply failed:", err)
			}
		}
	}()
	return conn, nil
}

// io_Closer is io.Closer, named locally so this file does not import io for one
// interface.
type io_Closer interface{ Close() error }
