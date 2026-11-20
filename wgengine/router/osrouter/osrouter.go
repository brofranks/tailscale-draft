// Copyright (c) Tailscale Inc & contributors
// SPDX-License-Identifier: BSD-3-Clause

// Package osrouter contains OS-specific router implementations.
// This package has no API; it exists purely to import
// for the side effect of it registering itself with the wgengine/router
// package.
package osrouter

import (
	"net/netip"
	"slices"

	"tailscale.com/wgengine/router"
)

// shutdownConfig is a routing configuration that removes all router
// state from the OS. It's the config used when callers pass in a nil
// Config.
var shutdownConfig router.Config

// isLocalHostRoute reports whether the kernel already supplies the host route
// for an address assigned to the adapter.
func isLocalHostRoute(route netip.Prefix, assigned []netip.Prefix) bool {
	return route.IsSingleIP() && slices.ContainsFunc(assigned, func(p netip.Prefix) bool {
		return p.Addr().Unmap() == route.Addr().Unmap()
	})
}
