package lan

import (
	"encoding/json"
	"net"
	"strconv"
	"time"

	"golang.org/x/net/ipv4"

	"xin.dart.zchat/internal/netutil"
)

// The discovery socket sends to this group and to each interface's
// broadcast address, so networks that block one often deliver the other.
var discoverGroup = net.IPv4(239, 255, 77, 77)

const (
	announceHello = "hello"
	announceProbe = "probe"
	announceBye   = "bye"
)

// announcement is the small UDP packet peers exchange. It carries no
// avatar: the TCP hello does, once, so the heartbeat stays tiny.
type announcement struct {
	App       string `json:"app"`
	V         int    `json:"v"`
	Kind      string `json:"kind"`
	ID        string `json:"id"`
	Instance  string `json:"instance,omitempty"`
	Name      string `json:"name"`
	Signature string `json:"signature,omitempty"`
	Status    string `json:"status"`
	Version   string `json:"version,omitempty"`
	Platform  string `json:"platform,omitempty"`
	Port      int    `json:"port"`
	TS        int64  `json:"ts"`
}

func (a announcement) encode() ([]byte, error) {
	return json.Marshal(a)
}

func (n *Node) startDiscovery() error {
	conn, err := net.ListenPacket("udp4", ":0")
	if err != nil {
		return err
	}
	// The fixed port is what other nodes listen on. If it is taken, the
	// node can still open connections and be found by manual address, so
	// report the failure but keep the random socket for sending.
	sendConn := conn
	if fixed, err := net.ListenPacket("udp4", ":"+strconv.Itoa(n.opts.DiscoveryPort)); err == nil {
		sendConn = fixed
		conn.Close()
	} else {
		n.report(err)
	}
	pc := ipv4.NewPacketConn(sendConn)
	_ = pc.SetControlMessage(ipv4.FlagInterface, true)
	_ = pc.SetMulticastLoopback(true)
	group := &net.UDPAddr{IP: discoverGroup, Port: n.opts.DiscoveryPort}

	var ifaces []net.Interface
	if list, err := net.Interfaces(); err == nil {
		for _, ifi := range list {
			if ifi.Flags&net.FlagUp == 0 || ifi.Flags&net.FlagLoopback != 0 {
				continue
			}
			if ifi.Flags&net.FlagMulticast != 0 {
				if err := pc.JoinGroup(&ifi, group); err == nil {
					ifaces = append(ifaces, ifi)
				}
			}
		}
	}
	n.mu.Lock()
	n.udp = sendConn
	n.pc = pc
	n.ifaces = ifaces
	n.dport = n.opts.DiscoveryPort
	n.mu.Unlock()

	n.wg.Add(1)
	go func() {
		defer n.wg.Done()
		n.readDiscovery(pc)
	}()
	return nil
}

func (n *Node) readDiscovery(pc *ipv4.PacketConn) {
	buf := make([]byte, 4096)
	for {
		count, _, src, err := pc.ReadFrom(buf)
		if err != nil {
			select {
			case <-n.stop:
				return
			default:
			}
			n.report(err)
			time.Sleep(200 * time.Millisecond)
			continue
		}
		var a announcement
		if json.Unmarshal(buf[:count], &a) != nil {
			continue
		}
		var ip net.IP
		if udp, ok := src.(*net.UDPAddr); ok {
			ip = udp.IP
		}
		if a.Kind == announceBye {
			n.markOffline(a.ID)
			continue
		}
		n.applyAnnouncement(a, ip)
	}
}

func (n *Node) sendDiscovery(b []byte) {
	n.mu.RLock()
	pc := n.pc
	conn := n.udp
	ifaces := append([]net.Interface(nil), n.ifaces...)
	port := n.dport
	n.mu.RUnlock()
	if pc == nil || conn == nil {
		return
	}
	group := &net.UDPAddr{IP: discoverGroup, Port: port}
	for _, ifi := range ifaces {
		if _, err := pc.WriteTo(b, &ipv4.ControlMessage{IfIndex: ifi.Index}, group); err != nil {
			_, _ = conn.WriteTo(b, group)
		}
	}
	// Broadcast reaches devices on networks where the multicast group is
	// filtered, such as some guest Wi-Fi access points.
	if list, err := net.Interfaces(); err == nil {
		for _, ifi := range list {
			if ifi.Flags&net.FlagUp == 0 || ifi.Flags&net.FlagLoopback != 0 || ifi.Flags&net.FlagBroadcast == 0 {
				continue
			}
			addrs, err := ifi.Addrs()
			if err != nil {
				continue
			}
			for _, addr := range addrs {
				ipnet, ok := addr.(*net.IPNet)
				if !ok || ipnet.IP.To4() == nil {
					continue
				}
				if bcast, ok := netutil.Broadcast(addr); ok {
					dst := &net.UDPAddr{IP: bcast, Port: port}
					if _, err := pc.WriteTo(b, &ipv4.ControlMessage{IfIndex: ifi.Index}, dst); err != nil {
						_, _ = conn.WriteTo(b, dst)
					}
				}
			}
		}
	}
	// A last fallback for platforms that ignore interface control
	// messages: the limited broadcast address uses the default route.
	_, _ = conn.WriteTo(b, &net.UDPAddr{IP: net.IPv4bcast, Port: port})
}

func (n *Node) closeDiscovery() {
	n.mu.Lock()
	pc := n.pc
	conn := n.udp
	n.pc, n.udp, n.ifaces = nil, nil, nil
	n.mu.Unlock()
	if pc != nil {
		_ = pc.Close()
	}
	if conn != nil {
		_ = conn.Close()
	}
}
