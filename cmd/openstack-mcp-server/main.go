package main

import (
	"fmt"
	"log"
	"os"

	"github.com/notque/openstack-mcp-server/internal/auth"
	"github.com/notque/openstack-mcp-server/internal/config"
	"github.com/notque/openstack-mcp-server/internal/server"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to load config: %v\n", err)
		os.Exit(1)
	}

	provider, err := auth.NewProvider(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to initialize auth: %v\n", err)
		os.Exit(1)
	}

	srv, err := server.New(cfg, provider)
	if err != nil {
		log.Fatalf("failed to create server: %v", err)
	}

	if err := srv.Run(); err != nil {
		log.Fatalf("server error: %v", err)
	}
}
