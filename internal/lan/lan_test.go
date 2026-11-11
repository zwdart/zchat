package lan

import (
	"net"
	"strconv"
	"testing"
	"time"

	"xin.dart.zchat/internal/model"
	"xin.dart.zchat/internal/proto"
)

func freePort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()
	return port
}

// A manual connection is registered on both ends, so the peers can send
// to each other; before, ConnectAddress handshook and forgot the socket.
func TestConnectAddressRegisters(t *testing.T) {
	portA, portB := freePort(t), freePort(t)
	bConnected := make(chan model.Peer, 1)
	frames := make(chan proto.Frame, 1)

	a := New(Options{
		Profile:          model.Profile{ID: "u_a", Name: "A", Status: model.Online},
		Port:             portA,
		TransferPort:     43000,
		DisableDiscovery: true,
	})
	b := New(Options{
		Profile:          model.Profile{ID: "u_b", Name: "B", Status: model.Online},
		Port:             portB,
		TransferPort:     43001,
		DisableDiscovery: true,
		OnConnect:        func(p model.Peer) { bConnected <- p },
		OnFrame:          func(_ string, f proto.Frame) { frames <- f },
	})
	if err := a.Start(); err != nil {
		t.Fatal(err)
	}
	defer a.Stop()
	if err := b.Start(); err != nil {
		t.Fatal(err)
	}
	defer b.Stop()

	peer, err := a.ConnectAddress("127.0.0.1:" + strconv.Itoa(portB))
	if err != nil {
		t.Fatal(err)
	}
	if peer.ID != "u_b" || peer.TransferPort != 43001 {
		t.Fatalf("peer = %+v", peer)
	}
	if !a.Connected(peer.ID) {
		t.Fatal("the outgoing side did not keep the connection")
	}
	select {
	case p := <-bConnected:
		if p.ID != "u_a" || p.TransferPort != 43000 {
			t.Fatalf("incoming peer = %+v", p)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("the incoming side did not register the connection")
	}
	if !b.Connected("u_a") {
		t.Fatal("the incoming side does not report the peer connected")
	}

	msg := model.Message{ID: "m_1", Text: "hi"}
	if !a.Send("u_b", proto.NewFrame("u_a", proto.TypeChat, proto.Chat{Message: msg})) {
		t.Fatal("Send reported no connection")
	}
	select {
	case f := <-frames:
		if f.Type != proto.TypeChat || f.From != "u_a" {
			t.Fatalf("frame = %+v", f)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("the frame did not arrive")
	}
}

// A peer that announces itself invisible is listed offline, and this
// device ignores its own announcements.
func TestApplyAnnouncement(t *testing.T) {
	n := New(Options{Profile: model.Profile{ID: "u_me", Name: "Me"}, DisableDiscovery: true})
	n.applyAnnouncement(announcement{
		App: appTag, V: proto.Version, Kind: announceHello, ID: "u_hidden",
		Name: "Hidden", Status: string(model.Invisible), Port: 42224,
	}, net.ParseIP("192.168.1.9"))
	n.applyAnnouncement(announcement{
		App: appTag, V: proto.Version, Kind: announceHello, ID: "u_me",
		Name: "Me", Status: string(model.Online), Port: 42224,
	}, net.ParseIP("192.168.1.10"))

	p, ok := n.Peer("u_hidden")
	if !ok {
		t.Fatal("the announced peer is missing")
	}
	if p.Status != model.Offline {
		t.Fatalf("status = %q, want offline", p.Status)
	}
	if _, ok := n.Peer("u_me"); ok {
		t.Fatal("the node added itself as a peer")
	}
}

// Peers that can receive are listed before those that cannot.
func TestPeersSortAvailableFirst(t *testing.T) {
	n := New(Options{Profile: model.Profile{ID: "u_me", Name: "Me"}, DisableDiscovery: true})
	n.applyAnnouncement(announcement{
		App: appTag, V: proto.Version, Kind: announceHello, ID: "u_off",
		Name: "Offline", Status: string(model.Offline), Port: 42224,
	}, net.ParseIP("192.168.1.11"))
	n.applyAnnouncement(announcement{
		App: appTag, V: proto.Version, Kind: announceHello, ID: "u_on",
		Name: "Online", Status: string(model.Online), Port: 42224,
	}, net.ParseIP("192.168.1.12"))
	peers := n.Peers()
	if len(peers) != 2 || peers[0].ID != "u_on" {
		t.Fatalf("peers = %+v", peers)
	}
}
