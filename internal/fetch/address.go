package fetch

import (
	"net/netip"
	"slices"
)

// nonPublic are the ranges the standard library's predicates miss: shared, reserved, documentation and benchmark
// networks, and the IPv6 prefixes that embed an IPv4 address a router would translate to a private one.
var nonPublic = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"),
	netip.MustParsePrefix("100.64.0.0/10"),
	netip.MustParsePrefix("192.0.0.0/24"),
	netip.MustParsePrefix("192.0.2.0/24"),
	netip.MustParsePrefix("198.18.0.0/15"),
	netip.MustParsePrefix("198.51.100.0/24"),
	netip.MustParsePrefix("203.0.113.0/24"),
	netip.MustParsePrefix("240.0.0.0/4"),
	netip.MustParsePrefix("::/96"),
	netip.MustParsePrefix("64:ff9b::/96"),
	netip.MustParsePrefix("64:ff9b:1::/48"),
	netip.MustParsePrefix("100::/64"),
	netip.MustParsePrefix("2001:db8::/32"),
	netip.MustParsePrefix("2002::/16"),
	netip.MustParsePrefix("fec0::/10"),
}

// IsPublic says whether an address is reachable on the internet at large rather than inside the host, its network or
// its cloud: loopback, private, link-local (cloud metadata lives there), multicast and reserved addresses are not.
func IsPublic(addr netip.Addr) bool {
	addr = addr.Unmap()
	return addr.IsGlobalUnicast() && !addr.IsPrivate() &&
		!slices.ContainsFunc(nonPublic, func(prefix netip.Prefix) bool { return prefix.Contains(addr) })
}
