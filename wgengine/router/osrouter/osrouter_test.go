// Copyright (c) Tailscale Inc & contributors
// SPDX-License-Identifier: BSD-3-Clause

package osrouter

import (
	"net/netip"
	"testing"
)

//lint:ignore U1000 used in Windows/Linux tests only
func mustCIDRs(ss ...string) []netip.Prefix {
	var ret []netip.Prefix
	for _, s := range ss {
		ret = append(ret, netip.MustParsePrefix(s))
	}
	return ret
}

func TestIsLocalHostRoute(t *testing.T) {
	assigned := mustCIDRs("fd42::10/128", "fd42::21/64", "10.0.0.16/32", "10.0.0.17/32")
	for _, tc := range []struct {
		prefix string
		skip   bool
	}{
		{"fd42::10/124", false}, {"fd42::10/128", true}, {"fd42::21/128", true},
		{"fd42::22/128", false}, {"10.0.0.16/28", false}, {"10.0.0.17/32", true},
		{"100.100.100.100/32", false}, {"::/0", false},
	} {
		if got := isLocalHostRoute(netip.MustParsePrefix(tc.prefix), assigned); got != tc.skip {
			t.Errorf("skip %s = %v, want %v", tc.prefix, got, tc.skip)
		}
	}
}
