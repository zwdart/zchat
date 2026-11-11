// Package netutil finds the LAN addresses of this device and the
// broadcast address of each interface.
package netutil

import "net"

// InterfaceIPs lists the LAN IPv4 addresses of this device, without
// loopback or link-local addresses.
func InterfaceIPs() []string {
	ifs, err := net.Interfaces()
	if err != nil {
		return nil
	}
	var out []string
	for _, ifi := range ifs {
		if ifi.Flags&net.FlagUp == 0 || ifi.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := ifi.Addrs()
		if err != nil {
			continue
		}
		for _, a := range addrs {
			var ip net.IP
			switch v := a.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}
			if ip == nil || ip.To4() == nil || ip.IsLoopback() || ip.IsLinkLocalUnicast() {
				continue
			}
			out = append(out, ip.String())
		}
	}
	return out
}

// PrimaryIP is the first LAN IPv4 address, or 127.0.0.1.
func PrimaryIP() string {
	if ips := InterfaceIPs(); len(ips) > 0 {
		return ips[0]
	}
	return "127.0.0.1"
}

// Broadcast is the directed broadcast address of an IPv4 interface.
func Broadcast(addr net.Addr) (net.IP, bool) {
	ipnet, ok := addr.(*net.IPNet)
	if !ok {
		return nil, false
	}
	ip := ipnet.IP.To4()
	if ip == nil {
		return nil, false
	}
	mask := net.IP(ipnet.Mask).To4()
	if mask == nil {
		return nil, false
	}
	out := make(net.IP, net.IPv4len)
	for i := 0; i < net.IPv4len; i++ {
		out[i] = ip[i] | ^mask[i]
	}
	return out, true
}
