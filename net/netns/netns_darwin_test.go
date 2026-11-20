// Copyright (c) Tailscale Inc & contributors
// SPDX-License-Identifier: BSD-3-Clause

package netns

import (
	"errors"
	"net"
	"syscall"
	"testing"

	"tailscale.com/net/netmon"
)

func TestGetInterfaceIndex(t *testing.T) {
	oldVal := bindToInterfaceByRoute.Load()
	t.Cleanup(func() { bindToInterfaceByRoute.Store(oldVal) })
	bindToInterfaceByRoute.Store(true)

	tests := []struct {
		name string
		addr string
		err  string
	}{
		{
			name: "IP_and_port",
			addr: "8.8.8.8:53",
		},
		{
			name: "bare_ip",
			addr: "8.8.8.8",
		},
		{
			name: "invalid",
			addr: "!!!!!",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			idx, err := getInterfaceIndex(t.Logf, nil, tc.addr)
			if err != nil {
				if tc.err == "" {
					t.Fatalf("got unexpected error: %v", err)
				}
				if errstr := err.Error(); errstr != tc.err {
					t.Errorf("expected error %q, got %q", errstr, tc.err)
				}
			} else {
				t.Logf("getInterfaceIndex(%q) = %d", tc.addr, idx)
				if tc.err != "" {
					t.Fatalf("wanted error %q", tc.err)
				}
				if idx < 0 {
					t.Fatalf("got invalid index %d", idx)
				}
			}
		})
	}

	t.Run("NoTailscale", func(t *testing.T) {
		tsif, err := tailscaleInterface()
		if err != nil {
			t.Fatal(err)
		}
		if tsif == nil {
			t.Skip("no tailscale interface on this machine")
		}

		defaultIdx, err := netmon.DefaultRouteInterfaceIndex()
		if err != nil {
			t.Fatal(err)
		}

		idx, err := getInterfaceIndex(t.Logf, nil, "100.100.100.100:53")
		if err != nil {
			t.Fatal(err)
		}

		t.Logf("tailscaleIdx=%d defaultIdx=%d idx=%d", tsif.Index, defaultIdx, idx)

		if idx == tsif.Index {
			t.Fatalf("got idx=%d; wanted not Tailscale interface", idx)
		} else if idx != defaultIdx {
			t.Fatalf("got idx=%d, want %d", idx, defaultIdx)
		}
	})
}

func TestRegisteredTunnelBinding(t *testing.T) {
	name, _ := netmon.TailscaleInterfaceName()
	index, _ := netmon.TailscaleInterfaceIndex()
	t.Cleanup(func() { netmon.SetTailscaleInterfaceProps(name, index) })
	lo, err := net.InterfaceByName("lo0")
	if err != nil {
		t.Fatal(err)
	}
	// The registered index may now belong to another device.
	netmon.SetTailscaleInterfaceProps(lo.Name, lo.Index+1)
	if got, err := tailscaleInterface(); err != nil || got.Index != lo.Index {
		t.Fatalf("registered tunnel = %v, %v, want %v", got, err, lo)
	}
	if _, err := checkDefaultInterface(lo.Index, nil); !errors.Is(err, errTunnelUnderlay) {
		t.Fatalf("tunnel as default: %v", err)
	}
	if got, err := checkDefaultInterface(lo.Index+1, nil); err != nil || got != lo.Index+1 {
		t.Fatalf("reused index = %v, %v", got, err)
	}
	t.Run("self-default-socket", func(t *testing.T) {
		idx, err := netmon.DefaultRouteInterfaceIndex()
		if errors.Is(err, netmon.ErrNoGatewayIndexFound) {
			t.Skip("requires a default route")
		}
		if err != nil {
			t.Fatal(err)
		}
		iface, err := net.InterfaceByIndex(idx)
		if err != nil {
			t.Fatal(err)
		}
		netmon.SetTailscaleInterfaceProps(iface.Name, lo.Index)
		oldRoute := bindToInterfaceByRoute.Swap(false)
		t.Cleanup(func() { bindToInterfaceByRoute.Store(oldRoute) })
		lc := net.ListenConfig{Control: func(network, _ string, c syscall.RawConn) error {
			return controlLogf(t.Logf, nil, network, "192.0.2.1:9", c)
		}}
		conn, err := lc.ListenPacket(t.Context(), "udp4", "0.0.0.0:0")
		if err == nil {
			conn.Close()
		}
		if !errors.Is(err, errTunnelUnderlay) {
			t.Fatalf("socket with tunnel as default: %v", err)
		}
	})
	netmon.SetTailscaleInterfaceProps("missing-registered-utun", lo.Index)
	if got, err := tailscaleInterface(); err == nil || got != nil {
		t.Fatalf("removed tunnel = %v, %v", got, err)
	}
}
