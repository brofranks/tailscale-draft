// Copyright (c) Tailscale Inc & contributors
// SPDX-License-Identifier: BSD-3-Clause

package osrouter

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/tailscale/wireguard-go/tun"
	"tailscale.com/health"
	"tailscale.com/net/netmon"
	"tailscale.com/types/logger"
	"tailscale.com/util/eventbus"
	"tailscale.com/util/usermetric"
	"tailscale.com/wgengine"
)

// This harness runs only on a disposable runner with the pinned Wintun DLL.
func TestNativeFirewallOwnership(t *testing.T) {
	mode := os.Getenv("HEADWIRE_FIREWALL_PROBE")
	if mode == "" {
		t.Skip("native firewall rehearsal")
	}
	idle := make(chan struct{}, 1)
	logf := func(f string, args ...any) {
		msg := fmt.Sprintf(f, args...)
		t.Log(msg)
		if strings.Contains(msg, "ending netsh goroutine") {
			select {
			case idle <- struct{}{}:
			default:
			}
		}
	}
	b := eventbus.New()
	defer b.Close()
	h := health.NewTracker(b)
	mon, err := netmon.New(b, logf)
	if err != nil {
		t.Fatal(err)
	}
	defer mon.Close()
	dev, err := tun.CreateTUN("HeadwireCleanupReview", 1280)
	if err != nil {
		t.Fatal(err)
	}
	router, err := newUserspaceRouter(logf, dev, mon, h, b)
	if err != nil {
		dev.Close()
		t.Fatal(err)
	}
	r := router.(*winRouter)
	switch mode {
	case "before":
		wanted := errors.New("controlled failure before router.Up")
		defer wgengine.HookNewBird.SetForTest(func(logger.Logf, string) (wgengine.Bird, error) { return nil, wanted })()
		other, e := wgengine.NewUserspaceEngine(logf, wgengine.Config{Tun: dev, Router: r, NetMon: mon, BIRDSocket: "unused", EventBus: b, Metrics: new(usermetric.Registry), HealthTracker: h})
		if other != nil {
			other.Close()
			t.Fatal("unexpected successful start")
		}
		if !errors.Is(e, wanted) {
			t.Fatalf("did not reach the pre-Up error: %v", e)
		}
	case "used":
		defer dev.Close()
		if err := r.Up(); err != nil {
			r.Close()
			t.Fatal(err)
		}
		if err := r.Close(); err != nil {
			t.Fatal(err)
		}
	default:
		t.Fatal("unknown probe mode")
	}
	deadline := time.NewTimer(90 * time.Second)
	defer deadline.Stop()
	for {
		r.firewall.mu.Lock()
		running, known := r.firewall.running, r.firewall.known
		r.firewall.mu.Unlock()
		if !running {
			t.Logf("RESULT native Wintun mode=%s firewall worker idle, completed netsh work=%v", mode, known)
			return
		}
		select {
		case <-idle:
		case <-deadline.C:
			t.Fatal("firewall worker did not finish")
		}
	}
}
