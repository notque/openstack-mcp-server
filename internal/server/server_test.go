// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company
// SPDX-License-Identifier: Apache-2.0

package server

import (
	"testing"

	mcpserver "github.com/mark3labs/mcp-go/server"

	"github.com/notque/openstack-mcp-server/internal/tools/archer"
	"github.com/notque/openstack-mcp-server/internal/tools/barbican"
	"github.com/notque/openstack-mcp-server/internal/tools/castellum"
	"github.com/notque/openstack-mcp-server/internal/tools/cinder"
	"github.com/notque/openstack-mcp-server/internal/tools/cronus"
	"github.com/notque/openstack-mcp-server/internal/tools/designate"
	"github.com/notque/openstack-mcp-server/internal/tools/glance"
	"github.com/notque/openstack-mcp-server/internal/tools/hermes"
	"github.com/notque/openstack-mcp-server/internal/tools/ironic"
	"github.com/notque/openstack-mcp-server/internal/tools/keppel"
	"github.com/notque/openstack-mcp-server/internal/tools/keystone"
	"github.com/notque/openstack-mcp-server/internal/tools/limes"
	"github.com/notque/openstack-mcp-server/internal/tools/maia"
	"github.com/notque/openstack-mcp-server/internal/tools/manila"
	"github.com/notque/openstack-mcp-server/internal/tools/neutron"
	"github.com/notque/openstack-mcp-server/internal/tools/nova"
	"github.com/notque/openstack-mcp-server/internal/tools/octavia"
	"github.com/notque/openstack-mcp-server/internal/tools/swift"
)

// TestAllModulesRegisterWithoutPanic ensures that all service module Register
// functions can be called without panicking. This validates tool definitions,
// handler closures, and import wiring. A nil provider is passed because
// Register only captures a reference — handlers aren't invoked here.
func TestAllModulesRegisterWithoutPanic(t *testing.T) {
	s := mcpserver.NewMCPServer("test", "0.0.1", mcpserver.WithToolCapabilities(true))

	// Each Register call captures the provider pointer in closures.
	// Passing nil is safe because we never invoke the handlers.
	nova.Register(s, nil, false, false)
	neutron.Register(s, nil, false, false)
	cinder.Register(s, nil, false, false)
	keystone.Register(s, nil, false, false)
	designate.Register(s, nil, false, false)
	barbican.Register(s, nil, false)
	swift.Register(s, nil, false, false)
	manila.Register(s, nil, false)
	octavia.Register(s, nil, false, false)
	glance.Register(s, nil, false)
	hermes.Register(s, nil)
	limes.Register(s, nil)
	keppel.Register(s, nil)
	archer.Register(s, nil)
	maia.Register(s, nil)
	castellum.Register(s, nil)
	cronus.Register(s, nil)
	ironic.Register(s, nil, false, false)

	// If we reach here, all 18 modules registered without panic.
	t.Logf("All 18 service modules registered successfully")
}
