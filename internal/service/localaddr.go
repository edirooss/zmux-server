package service

import (
	"context"
	"fmt"
	"net"
	"sort"
	"strings"
	"sync"
	"time"
)

// Service returns bindable IP addresses (and related interface info) with a small in-memory cache.
// Primary use case: power dropdowns for "localaddr" selection in the UI.
//
// Design choices:
// - Only "global" scoped addresses by default (skip loopback/link-local).
// - IPv4-only, because most ffmpeg/rtsp setups bind IPv4. You can extend to v6 if needed.
// - Read-heavy usage => RWMutex; return a copy of cached data to avoid caller mutations.
// - TTL is configurable; expose Invalidate for force-refresh via admin ops.
//
// If you need more detailed NIC info (multicast caps, etc.), switch to netlink later.

type LocalAddrListerOptions struct {
	TTL                time.Duration // Cache TTL, e.g., 15 * time.Second
	IncludeLoopback    bool          // Include 127.0.0.0/8
	IncludeLinkLocal   bool          // Include 169.254.0.0/16
	RequireInterfaceUp bool          // Only interfaces that are UP
	OnlyIPv4           bool          // Keep true unless you explicitly want v6 too
}

func (o *LocalAddrListerOptions) setDefaults() {
	if o.TTL <= 0 {
		o.TTL = 15 * time.Second
	}
	o.OnlyIPv4 = true
}

// NetIfAddr describes a single interface address.
type NetIfAddr struct {
	Family string `json:"family"` // "ipv4" | "ipv6"
	Addr   string `json:"addr"`   // "192.168.1.10"
	Scope  string `json:"scope"`  // "global" | "link" | "loopback"
}

// NetInterface describes an interface and its usable addresses.
type NetInterface struct {
	Name  string      `json:"name"` // "eth0"
	Index int         `json:"index"`
	MAC   string      `json:"mac"`
	MTU   int         `json:"mtu"`
	Addrs []NetIfAddr `json:"addrs"`
}

// IPv4Address is a flattened pair used by simple dropdowns.
type IPv4Address struct {
	Iface     string `json:"iface"`     // Interface name. e,g. "eth0"
	LocalAddr string `json:"localaddr"` // IPv4 address. e,g. "192.168.1.10"
}

// LocalAddrLister exposes cached IPv4 address listing.
type LocalAddrLister struct {
	mu      sync.RWMutex
	cache   []IPv4Address
	expires time.Time
	opts    LocalAddrListerOptions
	now     func() time.Time // for tests; default time.Now
}

// NewLocalAddrLister creates the service with provided options.
func NewLocalAddrLister(opts LocalAddrListerOptions) *LocalAddrLister {
	opts.setDefaults()
	return &LocalAddrLister{
		opts: opts,
		now:  time.Now,
	}
}

// Invalidate clears the cache so the next call refetches immediately.
func (s *LocalAddrLister) Invalidate() {
	s.mu.Lock()
	s.cache = nil
	s.expires = time.Time{}
	s.mu.Unlock()
}

// GetLocalAddrs returns a flattened list of IPv4 addresses by interface (cached).
func (s *LocalAddrLister) GetLocalAddrs(ctx context.Context) ([]IPv4Address, error) {
	// Fast path: read lock when cache is fresh
	s.mu.RLock()
	if s.cache != nil && s.now().Before(s.expires) {
		out := make([]IPv4Address, len(s.cache))
		copy(out, s.cache)
		s.mu.RUnlock()
		return out, nil
	}
	s.mu.RUnlock()

	// Slow path: refresh
	s.mu.Lock()
	defer s.mu.Unlock()

	// Another goroutine could have already refreshed; re-check
	if s.cache != nil && s.now().Before(s.expires) {
		out := make([]IPv4Address, len(s.cache))
		copy(out, s.cache)
		return out, nil
	}

	if ctx != nil && ctx.Err() != nil {
		return nil, ctx.Err()
	}

	ifaces, err := listInterfaces(s.opts)
	if err != nil {
		return nil, fmt.Errorf("list interfaces: %w", err)
	}

	ipv4List := flattenIPv4(ifaces)
	s.cache = ipv4List
	s.expires = s.now().Add(s.opts.TTL)

	out := make([]IPv4Address, len(s.cache))
	copy(out, s.cache)
	return out, nil
}

// -------------------- internals --------------------

func listInterfaces(opts LocalAddrListerOptions) ([]NetInterface, error) {
	sysIfaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}

	var out []NetInterface
	for _, ifc := range sysIfaces {
		if opts.RequireInterfaceUp && (ifc.Flags&net.FlagUp == 0) {
			continue
		}

		addrs, _ := ifc.Addrs()
		var ips []NetIfAddr

		for _, a := range addrs {
			var ip net.IP
			switch v := a.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			default:
				continue
			}
			if ip == nil {
				continue
			}

			// Family
			fam := "ipv6"
			if v4 := ip.To4(); v4 != nil {
				fam = "ipv4"
				ip = v4
			}
			if opts.OnlyIPv4 && fam != "ipv4" {
				continue
			}

			scope := classifyScope(ip)
			switch scope {
			case "loopback":
				if !opts.IncludeLoopback {
					continue
				}
			case "link":
				if !opts.IncludeLinkLocal {
					continue
				}
			}

			ips = append(ips, NetIfAddr{
				Family: fam,
				Addr:   ip.String(),
				Scope:  scope,
			})
		}
		if len(ips) == 0 {
			continue
		}

		out = append(out, NetInterface{
			Name:  ifc.Name,
			Index: ifc.Index,
			MAC:   ifc.HardwareAddr.String(),
			MTU:   ifc.MTU,
			Addrs: ips,
		})
	}

	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

func flattenIPv4(ifaces []NetInterface) []IPv4Address {
	var out []IPv4Address
	for _, iface := range ifaces {
		for _, addr := range iface.Addrs {
			if addr.Family == "ipv4" {
				out = append(out, IPv4Address{
					Iface:     iface.Name,
					LocalAddr: addr.Addr,
				})
			}
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Iface == out[j].Iface {
			return out[i].LocalAddr < out[j].LocalAddr
		}
		return out[i].Iface < out[j].Iface
	})
	return out
}

func classifyScope(ip net.IP) string {
	if ip.IsLoopback() {
		return "loopback"
	}
	// Link-local IPv4: 169.254.0.0/16
	if v4 := ip.To4(); v4 != nil {
		if v4[0] == 169 && v4[1] == 254 {
			return "link"
		}
		return "global"
	}
	// Link-local IPv6: fe80::/10
	if len(ip) == net.IPv6len && strings.HasPrefix(ip.String(), "fe80:") {
		return "link"
	}
	return "global"
}
