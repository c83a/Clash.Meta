//go:build linux

package ipset

import (
	"net/netip"
	"net"
	N "github.com/vishvananda/netlink"
)

// Test whether the ip is in the set or not
func Test(setName string, ip netip.Addr) (bool, error) {
	return N.IpsetTest(setName, &N.IPSetEntry{
		IP: net.IP(ip.AsSlice()),
	})
}

// Verify dumps a specific ipset to check if we can use the set normally
func Verify(setName string) error {
	_, err := N.IpsetList(setName)
	return err
}
