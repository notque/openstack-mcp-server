package server

import (
	"fmt"

	mcpserver "github.com/mark3labs/mcp-go/server"

	"github.com/notque/openstack-mcp-server/internal/auth"
	"github.com/notque/openstack-mcp-server/internal/config"
	"github.com/notque/openstack-mcp-server/internal/tools/archer"
	"github.com/notque/openstack-mcp-server/internal/tools/cinder"
	"github.com/notque/openstack-mcp-server/internal/tools/hermes"
	"github.com/notque/openstack-mcp-server/internal/tools/keppel"
	"github.com/notque/openstack-mcp-server/internal/tools/keystone"
	"github.com/notque/openstack-mcp-server/internal/tools/limes"
	"github.com/notque/openstack-mcp-server/internal/tools/maia"
	"github.com/notque/openstack-mcp-server/internal/tools/neutron"
	"github.com/notque/openstack-mcp-server/internal/tools/nova"
)

// Server wraps the MCP server with OpenStack tools.
type Server struct {
	mcp      *mcpserver.MCPServer
	cfg      *config.Config
	provider *auth.Provider
}

// New creates a new MCP server with all OpenStack tools registered.
func New(cfg *config.Config, provider *auth.Provider) (*Server, error) {
	s := mcpserver.NewMCPServer(
		"openstack-mcp-server",
		"0.1.0",
		mcpserver.WithToolCapabilities(true),
		mcpserver.WithRecovery(),
	)

	srv := &Server{
		mcp:      s,
		cfg:      cfg,
		provider: provider,
	}

	srv.registerTools()

	return srv, nil
}

// Run starts the MCP server with the configured transport.
func (s *Server) Run() error {
	switch s.cfg.Transport {
	case "stdio":
		return mcpserver.ServeStdio(s.mcp)
	case "sse":
		addr := fmt.Sprintf(":%d", s.cfg.Port)
		sseServer := mcpserver.NewSSEServer(s.mcp)
		return sseServer.Start(addr)
	default:
		return fmt.Errorf("unsupported transport: %s (use 'stdio' or 'sse')", s.cfg.Transport)
	}
}

// registerTools registers all OpenStack service tools with the MCP server.
func (s *Server) registerTools() {
	// Standard OpenStack services
	nova.Register(s.mcp, s.provider)
	neutron.Register(s.mcp, s.provider)
	cinder.Register(s.mcp, s.provider)
	keystone.Register(s.mcp, s.provider)

	// SAP CC-specific services
	hermes.Register(s.mcp, s.provider)
	limes.Register(s.mcp, s.provider)
	keppel.Register(s.mcp, s.provider)
	archer.Register(s.mcp, s.provider)
	maia.Register(s.mcp, s.provider)
}
