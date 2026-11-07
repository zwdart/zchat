// Package lan finds zchat peers on the local network over UDP multicast
// and broadcast, and carries frames between them over TCP.
package lan

import (
	"bufio"
	"errors"
	"fmt"
	"net"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/ipv4"

	"xin.dart.zchat/internal/i18n"
	"xin.dart.zchat/internal/model"
	"xin.dart.zchat/internal/netutil"
	"xin.dart.zchat/internal/proto"
)

const (
	defaultPort      = 42224
	defaultDiscovery = 42223
	announceEvery    = 4 * time.Second
	peerTimeout      = 16 * time.Second
	dialEvery        = 5 * time.Second
	dialTimeout      = 3 * time.Second
	handshakeTimeout = 8 * time.Second
)

// Options configures a Node.
type Options struct {
	Profile model.Profile
	// Port is the TCP port. Zero means 42224.
	Port int
	// TransferPort is the TCP port files are fetched from. Zero means the
	// peer advertised no socket transfer service.
	TransferPort int
	// DiscoveryPort is the UDP port. Zero means 42223.
	DiscoveryPort int
	// DisableDiscovery keeps the TCP listener and manual connections but
	// sends and listens to no discovery packets.
	DisableDiscovery bool
	// OnPeer runs when a peer is discovered or changes.
	OnPeer func(model.Peer)
	// OnConnect runs as a TCP connection with a peer is ready.
	OnConnect func(model.Peer)
	// OnDisconnect runs as a connection closes.
	OnDisconnect func(peerID string)
	// OnFrame runs for frames the app handles, on a network goroutine.
	OnFrame func(peerID string, f proto.Frame)
	// OnError reports discovery problems that do not stop the node.
	OnError func(error)
}

type peerState struct {
	peer     model.Peer
	instance string
	seenAt   time.Time
	dialedAt time.Time
	dialing  bool
}

// Node is this device's presence on the LAN and its connections.
type Node struct {
	opts     Options
	instance string

	mu      sync.RWMutex
	profile model.Profile
	port    int
	closed  bool
	peers   map[string]*peerState
	conns   map[string]*peerConn

	ln     net.Listener
	udp    net.PacketConn
	pc     *ipv4.PacketConn
	ifaces []net.Interface
	dport  int
	stop   chan struct{}
	wg     sync.WaitGroup
}

// New makes a node, not yet listening.
func New(opts Options) *Node {
	if opts.Port == 0 {
		opts.Port = defaultPort
	}
	if opts.DiscoveryPort == 0 {
		opts.DiscoveryPort = defaultDiscovery
	}
	if opts.Profile.Status == "" {
		opts.Profile.Status = model.Online
	}
	return &Node{
		opts:     opts,
		instance: model.NewID("boot"),
		profile:  opts.Profile,
		peers:    map[string]*peerState{},
		conns:    map[string]*peerConn{},
		stop:     make(chan struct{}),
	}
}

// Start listens for peers and starts announcing.
func (n *Node) Start() error {
	ln, err := net.Listen("tcp", fmt.Sprintf(":%d", n.opts.Port))
	if err != nil {
		return fmt.Errorf("zchat: listen on %d: %w", n.opts.Port, err)
	}
	n.mu.Lock()
	n.ln = ln
	n.port = ln.Addr().(*net.TCPAddr).Port
	n.mu.Unlock()

	n.wg.Add(1)
	go func() {
		defer n.wg.Done()
		n.acceptLoop(ln)
	}()

	if n.opts.DisableDiscovery {
		// No discovery socket: the node still listens for manual peers.
	} else if err := n.startDiscovery(); err != nil {
		n.report(err)
		n.closeDiscovery()
	}

	n.wg.Add(1)
	go func() {
		defer n.wg.Done()
		n.announceLoop()
	}()
	n.AnnounceNow()
	return nil
}

// Stop closes connections and announces that this device left.
func (n *Node) Stop() {
	n.mu.Lock()
	if n.closed {
		n.mu.Unlock()
		return
	}
	n.closed = true
	close(n.stop)
	ln := n.ln
	n.ln = nil
	conns := make([]*peerConn, 0, len(n.conns))
	for _, c := range n.conns {
		conns = append(conns, c)
	}
	n.conns = map[string]*peerConn{}
	n.mu.Unlock()

	n.announce(announceBye, model.Offline)
	for _, c := range conns {
		c.send(proto.NewFrame(n.profile.ID, proto.TypeBye, map[string]string{"reason": "quit"}))
		c.close()
	}
	if ln != nil {
		_ = ln.Close()
	}
	n.closeDiscovery()
	n.wg.Wait()
}

// Port is the TCP port peers reach.
func (n *Node) Port() int {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return n.port
}

// Self is this device's profile.
func (n *Node) Self() model.Profile {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return n.profile
}

// SetProfile changes the announced profile and tells connected peers.
func (n *Node) SetProfile(p model.Profile) {
	n.mu.Lock()
	n.profile = p
	n.mu.Unlock()
	update := proto.PresenceUpdate{Name: p.Name, Signature: p.Signature, Avatar: p.Avatar, Status: p.Status}
	n.broadcast(proto.NewFrame(p.ID, proto.TypePresence, update))
	n.AnnounceNow()
}

// SetStatus changes the presence and announces it.
func (n *Node) SetStatus(status model.Presence) {
	n.mu.Lock()
	n.profile.Status = status
	p := n.profile
	n.mu.Unlock()
	n.broadcast(proto.NewFrame(p.ID, proto.TypePresence, proto.PresenceUpdate{
		Name: p.Name, Signature: p.Signature, Avatar: p.Avatar, Status: status,
	}))
	n.AnnounceNow()
}

// Peers returns the peers known, online first and then by name.
func (n *Node) Peers() []model.Peer {
	n.mu.RLock()
	out := make([]model.Peer, 0, len(n.peers))
	for _, ps := range n.peers {
		p := ps.peer
		p.Connected = ps.peer.Connected
		out = append(out, p)
	}
	n.mu.RUnlock()
	sort.Slice(out, func(i, j int) bool {
		ai, aj := out[i].Status.Available(), out[j].Status.Available()
		if ai != aj {
			return ai
		}
		return strings.ToLower(out[i].Title()) < strings.ToLower(out[j].Title())
	})
	return out
}

// Peer returns a peer by ID.
func (n *Node) Peer(id string) (model.Peer, bool) {
	n.mu.RLock()
	defer n.mu.RUnlock()
	ps, ok := n.peers[id]
	if !ok {
		return model.Peer{}, false
	}
	p := ps.peer
	p.Connected = ps.peer.Connected
	return p, true
}

// Connected reports whether a TCP connection to the peer is open.
func (n *Node) Connected(id string) bool {
	n.mu.RLock()
	defer n.mu.RUnlock()
	_, ok := n.conns[id]
	return ok
}

// Send delivers a frame to one peer, reporting whether it was connected.
func (n *Node) Send(id string, f proto.Frame) bool {
	n.mu.RLock()
	c := n.conns[id]
	n.mu.RUnlock()
	if c == nil {
		return false
	}
	return c.send(f)
}

// Broadcast sends a frame to every connected peer.
func (n *Node) Broadcast(f proto.Frame) {
	n.broadcast(f)
}

func (n *Node) broadcast(f proto.Frame) {
	n.mu.RLock()
	conns := make([]*peerConn, 0, len(n.conns))
	for _, c := range n.conns {
		conns = append(conns, c)
	}
	n.mu.RUnlock()
	for _, c := range conns {
		c.send(f)
	}
}

// ConnectAddress dials a peer by address, such as 192.168.1.8:42224, for
// when discovery is blocked.
func (n *Node) ConnectAddress(addr string) (model.Peer, error) {
	if !strings.Contains(addr, ":") {
		addr = net.JoinHostPort(addr, fmt.Sprint(n.opts.Port))
	}
	conn, err := net.DialTimeout("tcp", addr, handshakeTimeout)
	if err != nil {
		return model.Peer{}, err
	}
	peer, br, err := n.handshake(conn, true)
	if err != nil {
		conn.Close()
		return model.Peer{}, err
	}
	if peer.ID == n.Self().ID {
		conn.Close()
		return model.Peer{}, errors.New(i18n.T("zchat: 不能连接自己"))
	}
	n.register(conn, br, peer, true)
	return peer, nil
}

// AnnounceNow sends a discovery packet at once, and asks others to do the
// same.
func (n *Node) AnnounceNow() {
	n.announce(announceHello, "")
	if n.pc != nil {
		n.announce(announceProbe, "")
	}
}

func (n *Node) acceptLoop(ln net.Listener) {
	for {
		conn, err := ln.Accept()
		if err != nil {
			select {
			case <-n.stop:
				return
			default:
			}
			n.report(err)
			continue
		}
		go n.handleConn(conn)
	}
}

func (n *Node) handleConn(conn net.Conn) {
	peer, br, err := n.handshake(conn, false)
	if err != nil {
		conn.Close()
		return
	}
	if peer.ID == n.Self().ID {
		conn.Close()
		return
	}
	n.register(conn, br, peer, false)
}

func (n *Node) handshake(conn net.Conn, outbound bool) (model.Peer, *bufio.Reader, error) {
	_ = conn.SetDeadline(time.Now().Add(handshakeTimeout))
	defer conn.SetDeadline(time.Time{})

	self := n.Self()
	port := n.Port()
	ip := netutil.PrimaryIP()
	peer := self.Peer(ip, port)
	peer.TransferPort = n.opts.TransferPort
	hello := proto.NewFrame(self.ID, proto.TypeHello, proto.Hello{Peer: peer})
	if outbound {
		if err := proto.WriteFrame(conn, hello); err != nil {
			return model.Peer{}, nil, err
		}
	}
	br := bufio.NewReader(conn)
	f, err := proto.ReadFrame(br)
	if err != nil {
		return model.Peer{}, nil, err
	}
	if f.Type != proto.TypeHello {
		return model.Peer{}, nil, fmt.Errorf("zchat: expected hello, got %s", f.Type)
	}
	var h proto.Hello
	if err := f.Decode(&h); err != nil {
		return model.Peer{}, nil, err
	}
	if h.Peer.ID == "" {
		return model.Peer{}, nil, errors.New("zchat: hello without an id")
	}
	if !outbound {
		if err := proto.WriteFrame(conn, hello); err != nil {
			return model.Peer{}, nil, err
		}
	}
	p := h.Peer
	if p.Port == 0 {
		p.Port = defaultPort
	}
	if p.Status == "" {
		p.Status = model.Online
	}
	p.LastSeen = time.Now()
	if host, _, err := net.SplitHostPort(conn.RemoteAddr().String()); err == nil {
		p.IP = host
	}
	p.Connected = true
	// The reader above may have buffered frames sent after hello; the
	// registered connection must go on with the same reader.
	return p, br, nil
}

func (n *Node) register(conn net.Conn, br *bufio.Reader, p model.Peer, outbound bool) {
	n.mu.Lock()
	if n.closed {
		n.mu.Unlock()
		conn.Close()
		return
	}
	ps := n.peers[p.ID]
	if ps == nil {
		ps = &peerState{}
		n.peers[p.ID] = ps
	}
	ps.peer = p
	ps.peer.Connected = true
	ps.seenAt = time.Now()
	if old := n.conns[p.ID]; old != nil {
		// When both sides dial at once, keep the same physical connection
		// on both ends: the side with the smaller ID keeps its outbound.
		keepOutbound := strings.Compare(n.profile.ID, p.ID) < 0
		if outbound != keepOutbound {
			n.mu.Unlock()
			conn.Close()
			return
		}
		delete(n.conns, p.ID)
		old.close()
	}
	c := newPeerConn(n, conn, br, p.ID)
	n.conns[p.ID] = c
	n.mu.Unlock()

	c.start()
	if n.opts.OnConnect != nil {
		n.opts.OnConnect(p)
	}
	n.notifyPeer(p)
}

func (n *Node) removeConn(id string, c *peerConn) {
	n.mu.Lock()
	if n.conns[id] == c {
		delete(n.conns, id)
	}
	ps := n.peers[id]
	if ps != nil && n.conns[id] == nil {
		ps.peer.Connected = false
	}
	n.mu.Unlock()
	if n.opts.OnDisconnect != nil {
		n.opts.OnDisconnect(id)
	}
	if ps != nil {
		n.notifyPeer(ps.peer)
	}
}

func (n *Node) handleFrame(id string, f proto.Frame) {
	n.mu.Lock()
	ps := n.peers[id]
	if ps != nil {
		ps.seenAt = time.Now()
	}
	n.mu.Unlock()
	switch f.Type {
	case proto.TypeBye:
		n.markOffline(id)
		if c := n.conn(id); c != nil {
			c.close()
		}
		return
	case proto.TypePresence:
		var up proto.PresenceUpdate
		if f.Decode(&up) != nil {
			return
		}
		n.mu.Lock()
		ps := n.peers[id]
		if ps != nil {
			ps.peer.Name = up.Name
			ps.peer.Signature = up.Signature
			ps.peer.Avatar = up.Avatar
			ps.peer.Status = up.Status
			ps.peer.LastSeen = time.Now()
		}
		n.mu.Unlock()
		if ps != nil {
			n.notifyPeer(ps.peer)
		}
		return
	case proto.TypePing:
		n.Send(id, proto.NewFrame(n.Self().ID, proto.TypePong, nil))
		return
	case proto.TypePong:
		return
	}
	if n.opts.OnFrame != nil {
		n.opts.OnFrame(id, f)
	}
}

func (n *Node) conn(id string) *peerConn {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return n.conns[id]
}

func (n *Node) markOffline(id string) {
	n.mu.Lock()
	ps := n.peers[id]
	if ps != nil {
		ps.peer.Status = model.Offline
		ps.peer.Connected = false
	}
	n.mu.Unlock()
	if ps != nil {
		n.notifyPeer(ps.peer)
	}
}

func (n *Node) notifyPeer(p model.Peer) {
	if n.opts.OnPeer != nil {
		n.opts.OnPeer(p)
	}
}

func (n *Node) report(err error) {
	if err == nil || n.opts.OnError == nil {
		return
	}
	if strings.Contains(err.Error(), "use of closed network connection") {
		return
	}
	n.opts.OnError(err)
}

func (n *Node) announceLoop() {
	t := time.NewTicker(time.Second)
	defer t.Stop()
	last := time.Time{}
	lastDial := time.Time{}
	for {
		select {
		case <-n.stop:
			return
		case now := <-t.C:
			if now.Sub(last) >= announceEvery {
				last = now
				n.announce(announceHello, "")
			}
			if now.Sub(lastDial) >= 500*time.Millisecond {
				lastDial = now
				n.dialDiscovered()
			}
			n.expirePeers(now)
		}
	}
}

func (n *Node) dialDiscovered() {
	n.mu.Lock()
	self := n.profile.ID
	type target struct {
		id   string
		ip   string
		port int
	}
	var targets []target
	for id, ps := range n.peers {
		if id == self || ps.peer.Connected || n.conns[id] != nil || ps.dialing {
			continue
		}
		if !ps.peer.Status.Available() || ps.peer.IP == "" || ps.peer.Port == 0 {
			continue
		}
		if time.Since(ps.dialedAt) < dialEvery {
			continue
		}
		ps.dialedAt = time.Now()
		ps.dialing = true
		targets = append(targets, target{id: id, ip: ps.peer.IP, port: ps.peer.Port})
	}
	n.mu.Unlock()
	for _, t := range targets {
		go func(t target) {
			addr := net.JoinHostPort(t.ip, fmt.Sprint(t.port))
			conn, err := net.DialTimeout("tcp", addr, dialTimeout)
			var peer model.Peer
			var br *bufio.Reader
			if err == nil {
				peer, br, err = n.handshake(conn, true)
			}
			n.mu.Lock()
			if ps := n.peers[t.id]; ps != nil {
				ps.dialing = false
			}
			n.mu.Unlock()
			if err != nil {
				if conn != nil {
					conn.Close()
				}
				return
			}
			if peer.ID == n.Self().ID {
				conn.Close()
				return
			}
			n.register(conn, br, peer, true)
		}(t)
	}
}

func (n *Node) expirePeers(now time.Time) {
	n.mu.Lock()
	var lost []model.Peer
	for _, ps := range n.peers {
		if ps.peer.Connected || ps.peer.Status == model.Offline {
			continue
		}
		if now.Sub(ps.seenAt) > peerTimeout {
			ps.peer.Status = model.Offline
			ps.peer.Connected = false
			lost = append(lost, ps.peer)
		}
	}
	n.mu.Unlock()
	for _, p := range lost {
		n.notifyPeer(p)
	}
}

func (n *Node) applyAnnouncement(a announcement, src net.IP) {
	if a.App != appTag || a.V != proto.Version || a.ID == "" || a.ID == n.Self().ID {
		return
	}
	if a.Port <= 0 || a.Port > 65535 {
		return
	}
	status := model.Presence(a.Status)
	if status == "" {
		status = model.Online
	}
	if status == model.Invisible {
		status = model.Offline
	}
	n.mu.Lock()
	ps := n.peers[a.ID]
	fresh := ps == nil
	if ps == nil {
		ps = &peerState{}
		n.peers[a.ID] = ps
	}
	if src != nil {
		ps.peer.IP = src.String()
	}
	ps.peer.ID = a.ID
	ps.peer.Name = a.Name
	ps.peer.Signature = a.Signature
	ps.peer.Status = status
	ps.peer.Port = a.Port
	ps.peer.TransferPort = a.TransferPort
	ps.peer.Version = a.Version
	ps.peer.Platform = a.Platform
	ps.peer.LastSeen = time.Now()
	ps.seenAt = time.Now()
	ps.instance = a.Instance
	ps.peer.Connected = n.conns[a.ID] != nil || ps.peer.Connected
	p := ps.peer
	n.mu.Unlock()
	if a.Kind == announceProbe {
		n.announce(announceHello, "")
	}
	if fresh || a.Kind != announceProbe {
		n.notifyPeer(p)
	}
}

func (n *Node) announce(kind string, status model.Presence) {
	if n.pc == nil {
		return
	}
	self := n.Self()
	if status == "" {
		status = self.Status
	}
	a := announcement{
		App:          appTag,
		V:            proto.Version,
		Kind:         kind,
		ID:           self.ID,
		Instance:     n.instance,
		Name:         self.Name,
		Signature:    self.Signature,
		Status:       string(status),
		Version:      version,
		Platform:     runtime.GOOS + "/" + runtime.GOARCH,
		Port:         n.Port(),
		TransferPort: n.opts.TransferPort,
		TS:           time.Now().UnixMilli(),
	}
	b, err := a.encode()
	if err != nil {
		return
	}
	n.sendDiscovery(b)
}

func (n *Node) selfID() string { return n.Self().ID }

// version is sent in announcements; set by the app package through
// SetVersion.
var version = "dev"

// SetVersion names the app version in announcements.
func SetVersion(v string) { version = v }

var appTag = "zchat"
