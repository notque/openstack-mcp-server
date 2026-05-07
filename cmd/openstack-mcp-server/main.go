// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"github.com/sapcc/go-bits/logg"
	"github.com/sapcc/go-bits/must"
	"github.com/sapcc/go-bits/osext"
	"github.com/spf13/cobra"

	"github.com/notque/openstack-mcp-server/internal/auth"
	"github.com/notque/openstack-mcp-server/internal/config"
	"github.com/notque/openstack-mcp-server/internal/server"
)

// version is injected at build time via -ldflags.
var version = "dev"

func main() {
	logg.ShowDebug = osext.GetenvBool("MCP_DEBUG")

	rootCmd := &cobra.Command{
		Use:     "openstack-mcp-server",
		Short:   "MCP server for OpenStack and SAP Converged Cloud",
		Version: version,
		Args:    cobra.NoArgs,
		Run:     run,
	}

	must.Succeed(rootCmd.Execute())
}

func run(_ *cobra.Command, _ []string) {
	cfg := must.Return(config.Load())

	provider := must.Return(auth.NewProvider(cfg))

	srv := must.Return(server.New(cfg, provider))

	logg.Info("starting openstack-mcp-server %s (transport=%s)", version, cfg.Transport)
	must.Succeed(srv.Run())
}
