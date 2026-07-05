package network

import (
	"net"
	"net/netip"
	"sort"
	"strings"

	"droponce/internal/domain/transfer"
)

func IsPrivateIPv4(value string) bool {
	addr, err := netip.ParseAddr(value)
	if err != nil || !addr.Is4() || addr.IsLoopback() || addr.IsLinkLocalUnicast() || addr.IsUnspecified() {
		return false
	}
	return addr.IsPrivate()
}

type PrivateIPv4Resolver struct{}

func (PrivateIPv4Resolver) Resolve() ([]transfer.NetworkEndpoint, error) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}
	var endpoints []transfer.NetworkEndpoint
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 || iface.Flags&net.FlagPointToPoint != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, raw := range addrs {
			prefix, err := netip.ParsePrefix(raw.String())
			if err != nil {
				continue
			}
			ip := prefix.Addr()
			if !ip.Is4() || !IsPrivateIPv4(ip.String()) {
				continue
			}
			endpoints = append(endpoints, transfer.NetworkEndpoint{
				InterfaceName: iface.Name,
				IPAddress:     ip.String(),
				DisplayName:   iface.Name + " - " + ip.String(),
				IsPrivateIPv4: true,
				IsUp:          true,
			})
		}
	}
	sort.Slice(endpoints, func(i, j int) bool {
		return rankInterface(endpoints[i].InterfaceName) < rankInterface(endpoints[j].InterfaceName)
	})
	return endpoints, nil
}

func rankInterface(name string) int {
	n := strings.ToLower(name)
	if strings.Contains(n, "wi") || strings.Contains(n, "wlan") || strings.Contains(n, "airport") {
		return 0
	}
	if strings.Contains(n, "eth") || strings.Contains(n, "en") {
		return 1
	}
	return 2
}
