package auth

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/gophercloud/gophercloud/v2"
	"github.com/gophercloud/gophercloud/v2/openstack"
	"github.com/gophercloud/utils/v2/openstack/clientconfig"
	"github.com/notque/openstack-mcp-server/internal/config"
)

// Provider manages OpenStack authentication and provides service clients.
type Provider struct {
	providerClient *gophercloud.ProviderClient
	region         string
	cfg            *config.Config
}

// NewProvider creates an authenticated provider using clouds.yaml or OS_* env vars.
func NewProvider(cfg *config.Config) (*Provider, error) {
	// Handle OS_PW_CMD: execute command to get password if OS_PASSWORD is not set.
	if os.Getenv("OS_PASSWORD") == "" {
		if pwCmd := os.Getenv("OS_PW_CMD"); pwCmd != "" {
			out, err := exec.Command("sh", "-c", pwCmd).Output()
			if err != nil {
				return nil, fmt.Errorf("executing OS_PW_CMD (%s): %w", pwCmd, err)
			}
			os.Setenv("OS_PASSWORD", strings.TrimSpace(string(out)))
		}
	}

	var authOpts *gophercloud.AuthOptions
	var region string

	if cfg.UseEnvAuth {
		// Build auth options from OS_* environment variables directly.
		// We handle this ourselves rather than using gophercloud's AuthOptionsFromEnv()
		// because SAP CC uses separate user/project domain names which that function
		// doesn't handle cleanly.
		authOpts = &gophercloud.AuthOptions{
			IdentityEndpoint: os.Getenv("OS_AUTH_URL"),
			Username:         os.Getenv("OS_USERNAME"),
			UserID:           os.Getenv("OS_USERID"),
			Password:         os.Getenv("OS_PASSWORD"),
			DomainName:       os.Getenv("OS_USER_DOMAIN_NAME"),
			DomainID:         os.Getenv("OS_USER_DOMAIN_ID"),
			AllowReauth:      true,
			Scope: &gophercloud.AuthScope{
				ProjectName: os.Getenv("OS_PROJECT_NAME"),
				ProjectID:   os.Getenv("OS_PROJECT_ID"),
				DomainName:  os.Getenv("OS_PROJECT_DOMAIN_NAME"),
				DomainID:    os.Getenv("OS_PROJECT_DOMAIN_ID"),
			},
		}
		region = cfg.Region
	} else {
		// Use clouds.yaml
		opts := &clientconfig.ClientOpts{
			Cloud: cfg.Cloud,
		}
		if cfg.Region != "" {
			opts.RegionName = cfg.Region
		}

		cloudAuthOpts, err := clientconfig.AuthOptions(opts)
		if err != nil {
			return nil, fmt.Errorf("reading auth options from clouds.yaml for cloud %q: %w", cfg.Cloud, err)
		}
		cloudAuthOpts.AllowReauth = true
		authOpts = cloudAuthOpts

		region = cfg.Region
		if region == "" {
			region = opts.RegionName
		}
	}

	provider, err := openstack.AuthenticatedClient(context.Background(), *authOpts)
	if err != nil {
		return nil, fmt.Errorf("authenticating to OpenStack: %w", err)
	}

	return &Provider{
		providerClient: provider,
		region:         region,
		cfg:            cfg,
	}, nil
}

// --- Standard OpenStack Service Clients ---

// ComputeClient returns an authenticated Nova (compute v2) client.
func (p *Provider) ComputeClient() (*gophercloud.ServiceClient, error) {
	return openstack.NewComputeV2(p.providerClient, gophercloud.EndpointOpts{
		Region: p.region,
	})
}

// NetworkClient returns an authenticated Neutron (network v2) client.
func (p *Provider) NetworkClient() (*gophercloud.ServiceClient, error) {
	return openstack.NewNetworkV2(p.providerClient, gophercloud.EndpointOpts{
		Region: p.region,
	})
}

// BlockStorageClient returns an authenticated Cinder (block storage v3) client.
func (p *Provider) BlockStorageClient() (*gophercloud.ServiceClient, error) {
	return openstack.NewBlockStorageV3(p.providerClient, gophercloud.EndpointOpts{
		Region: p.region,
	})
}

// IdentityClient returns an authenticated Keystone (identity v3) client.
func (p *Provider) IdentityClient() (*gophercloud.ServiceClient, error) {
	return openstack.NewIdentityV3(p.providerClient, gophercloud.EndpointOpts{
		Region: p.region,
	})
}

// ObjectStorageClient returns an authenticated Swift (object storage v1) client.
func (p *Provider) ObjectStorageClient() (*gophercloud.ServiceClient, error) {
	return openstack.NewObjectStorageV1(p.providerClient, gophercloud.EndpointOpts{
		Region: p.region,
	})
}

// DNSClient returns an authenticated Designate (DNS v2) client.
func (p *Provider) DNSClient() (*gophercloud.ServiceClient, error) {
	return openstack.NewDNSV2(p.providerClient, gophercloud.EndpointOpts{
		Region: p.region,
	})
}

// LoadBalancerClient returns an authenticated Octavia (LB v2) client.
func (p *Provider) LoadBalancerClient() (*gophercloud.ServiceClient, error) {
	return openstack.NewLoadBalancerV2(p.providerClient, gophercloud.EndpointOpts{
		Region: p.region,
	})
}

// --- SAP CC Service Clients ---
// These follow the pattern from github.com/sapcc/gophercloud-sapcc/v2/clients

// HermesClient returns an authenticated Hermes (audit v1) client.
func (p *Provider) HermesClient() (*gophercloud.ServiceClient, error) {
	endpointOpts := gophercloud.EndpointOpts{Region: p.region}
	endpointOpts.ApplyDefaults("audit-data")

	url, err := p.providerClient.EndpointLocator(endpointOpts)
	if err != nil {
		// Fall back to configured endpoint
		if p.cfg.SAPCC.HermesEndpoint != "" {
			url = p.cfg.SAPCC.HermesEndpoint
		} else {
			return nil, fmt.Errorf("hermes endpoint not found in catalog and not configured: %w", err)
		}
	}

	return &gophercloud.ServiceClient{
		ProviderClient: p.providerClient,
		Endpoint:       url,
		Type:           "audit-data",
		ResourceBase:   url,
	}, nil
}

// LimesClient returns an authenticated Limes (resources v1) client.
func (p *Provider) LimesClient() (*gophercloud.ServiceClient, error) {
	endpointOpts := gophercloud.EndpointOpts{Region: p.region}
	endpointOpts.ApplyDefaults("resources")

	url, err := p.providerClient.EndpointLocator(endpointOpts)
	if err != nil {
		if p.cfg.SAPCC.LimesEndpoint != "" {
			url = p.cfg.SAPCC.LimesEndpoint
		} else {
			return nil, fmt.Errorf("limes endpoint not found in catalog and not configured: %w", err)
		}
	}

	return &gophercloud.ServiceClient{
		ProviderClient: p.providerClient,
		Endpoint:       url + "v1/",
		Type:           "resources",
	}, nil
}

// KeppelClient returns an authenticated Keppel (container registry) client.
func (p *Provider) KeppelClient() (*gophercloud.ServiceClient, error) {
	endpointOpts := gophercloud.EndpointOpts{Region: p.region}
	endpointOpts.ApplyDefaults("keppel")

	url, err := p.providerClient.EndpointLocator(endpointOpts)
	if err != nil {
		if p.cfg.SAPCC.KeppelEndpoint != "" {
			url = p.cfg.SAPCC.KeppelEndpoint
		} else {
			return nil, fmt.Errorf("keppel endpoint not found in catalog and not configured: %w", err)
		}
	}

	return &gophercloud.ServiceClient{
		ProviderClient: p.providerClient,
		Endpoint:       url,
		Type:           "keppel",
		ResourceBase:   url,
	}, nil
}

// ArcherClient returns an authenticated Archer (endpoint service) client.
func (p *Provider) ArcherClient() (*gophercloud.ServiceClient, error) {
	endpointOpts := gophercloud.EndpointOpts{Region: p.region}
	endpointOpts.ApplyDefaults("endpoint-services")

	url, err := p.providerClient.EndpointLocator(endpointOpts)
	if err != nil {
		if p.cfg.SAPCC.ArcherEndpoint != "" {
			url = p.cfg.SAPCC.ArcherEndpoint
		} else {
			return nil, fmt.Errorf("archer endpoint not found in catalog and not configured: %w", err)
		}
	}

	return &gophercloud.ServiceClient{
		ProviderClient: p.providerClient,
		Endpoint:       url,
		Type:           "endpoint-services",
		ResourceBase:   url,
	}, nil
}

// MaiaClient returns an authenticated Maia (prometheus/metrics) client.
func (p *Provider) MaiaClient() (*gophercloud.ServiceClient, error) {
	endpointOpts := gophercloud.EndpointOpts{Region: p.region}
	endpointOpts.ApplyDefaults("metrics")

	url, err := p.providerClient.EndpointLocator(endpointOpts)
	if err != nil {
		if p.cfg.SAPCC.MaiaEndpoint != "" {
			url = p.cfg.SAPCC.MaiaEndpoint
		} else {
			return nil, fmt.Errorf("maia endpoint not found in catalog and not configured: %w", err)
		}
	}

	return &gophercloud.ServiceClient{
		ProviderClient: p.providerClient,
		Endpoint:       url,
		Type:           "metrics",
		ResourceBase:   url,
	}, nil
}

// --- Accessors ---

// Token returns the current auth token for direct HTTP calls.
func (p *Provider) Token() string {
	return p.providerClient.TokenID
}

// Region returns the configured region.
func (p *Provider) Region() string {
	return p.region
}

// Config returns the server configuration.
func (p *Provider) Config() *config.Config {
	return p.cfg
}
