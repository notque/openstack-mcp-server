// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/sapcc/go-bits/osext"
	"gopkg.in/yaml.v3"
)

// Config holds the server configuration.
type Config struct {
	// Cloud is the cloud name from clouds.yaml to use.
	// If empty and OS_AUTH_URL is set, env-var-based auth is used instead.
	Cloud string `yaml:"cloud"`

	// Region overrides the region from clouds.yaml.
	Region string `yaml:"region"`

	// Transport is "stdio" or "sse" (default: stdio).
	Transport string `yaml:"transport"`

	// Port for SSE transport (default: 8080).
	Port int `yaml:"port"`

	// UseEnvAuth indicates that authentication should use OS_* env vars directly
	// instead of clouds.yaml. Set automatically when OS_AUTH_URL is present but
	// OS_CLOUD is not.
	UseEnvAuth bool `yaml:"-"`

	// ReadOnly disables mutating tools (server actions, credential deletion).
	// Enabled by default for safety. Set MCP_READ_ONLY=false to allow mutations.
	ReadOnly bool `yaml:"read_only"`

	// SAPCC holds SAP Converged Cloud-specific configuration.
	SAPCC SAPCCConfig `yaml:"sapcc"`
}

// SAPCCConfig holds endpoints for SAP CC-specific services.
type SAPCCConfig struct {
	// KeppelEndpoint overrides the keppel endpoint from the service catalog.
	KeppelEndpoint string `yaml:"keppel_endpoint"`

	// ArcherEndpoint overrides the archer endpoint from the service catalog.
	ArcherEndpoint string `yaml:"archer_endpoint"`

	// HermesEndpoint overrides the hermes endpoint from the service catalog.
	HermesEndpoint string `yaml:"hermes_endpoint"`

	// MaiaEndpoint overrides the maia endpoint from the service catalog.
	MaiaEndpoint string `yaml:"maia_endpoint"`

	// LimesEndpoint overrides the limes endpoint from the service catalog.
	LimesEndpoint string `yaml:"limes_endpoint"`

	// CastellumEndpoint overrides the castellum endpoint from the service catalog.
	CastellumEndpoint string `yaml:"castellum_endpoint"`

	// CronusEndpoint overrides the cronus endpoint from the service catalog.
	CronusEndpoint string `yaml:"cronus_endpoint"`
}

// Load reads configuration from the config file or environment variables.
// Priority: env vars > config file > defaults.
func Load() (*Config, error) {
	cfg := &Config{
		Transport: "stdio",
		Port:      8080,
		ReadOnly:  true, // safe default: disable mutating tools
	}

	// Try config file first
	if path := configPath(); path != "" {
		if err := loadFromFile(path, cfg); err != nil {
			return nil, fmt.Errorf("reading config file %s: %w", path, err)
		}
	}

	// Environment overrides (osext.GetenvOrDefault returns the env value or keeps the existing)
	cfg.Cloud = osext.GetenvOrDefault("OS_CLOUD", cfg.Cloud)
	cfg.Region = osext.GetenvOrDefault("OS_REGION_NAME", cfg.Region)
	cfg.Transport = osext.GetenvOrDefault("MCP_TRANSPORT", cfg.Transport)

	// MCP_READ_ONLY defaults true; explicitly set to "false" to enable mutations.
	if os.Getenv("MCP_READ_ONLY") == "false" {
		cfg.ReadOnly = false
	}

	// SAP CC endpoint overrides
	cfg.SAPCC.KeppelEndpoint = osext.GetenvOrDefault("SAPCC_KEPPEL_ENDPOINT", cfg.SAPCC.KeppelEndpoint)
	cfg.SAPCC.ArcherEndpoint = osext.GetenvOrDefault("SAPCC_ARCHER_ENDPOINT", cfg.SAPCC.ArcherEndpoint)
	cfg.SAPCC.HermesEndpoint = osext.GetenvOrDefault("SAPCC_HERMES_ENDPOINT", cfg.SAPCC.HermesEndpoint)
	cfg.SAPCC.MaiaEndpoint = osext.GetenvOrDefault("SAPCC_MAIA_ENDPOINT", cfg.SAPCC.MaiaEndpoint)
	cfg.SAPCC.LimesEndpoint = osext.GetenvOrDefault("SAPCC_LIMES_ENDPOINT", cfg.SAPCC.LimesEndpoint)
	cfg.SAPCC.CastellumEndpoint = osext.GetenvOrDefault("SAPCC_CASTELLUM_ENDPOINT", cfg.SAPCC.CastellumEndpoint)
	cfg.SAPCC.CronusEndpoint = osext.GetenvOrDefault("SAPCC_CRONUS_ENDPOINT", cfg.SAPCC.CronusEndpoint)

	if cfg.Cloud == "" {
		// If no cloud name but OS_AUTH_URL is set, use env-var-based auth
		if os.Getenv("OS_AUTH_URL") != "" {
			cfg.UseEnvAuth = true
		} else {
			return nil, errors.New("no cloud specified: set OS_CLOUD env var, OS_AUTH_URL for env-based auth, or 'cloud' in config file")
		}
	}

	return cfg, nil
}

func configPath() string {
	// Check explicit path
	if p := os.Getenv("OPENSTACK_MCP_CONFIG"); p != "" {
		return p
	}

	// Check XDG config
	xdg := os.Getenv("XDG_CONFIG_HOME")
	if xdg == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			home = ""
		}
		xdg = filepath.Join(home, ".config")
	}

	path := filepath.Join(xdg, "openstack-mcp-server", "config.yaml")
	if _, err := os.Stat(path); err == nil {
		return path
	}

	return ""
}

func loadFromFile(path string, cfg *Config) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return yaml.Unmarshal(data, cfg)
}
