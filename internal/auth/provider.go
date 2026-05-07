package auth

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/gophercloud/gophercloud/v2"
	"github.com/gophercloud/gophercloud/v2/openstack"
	"github.com/gophercloud/gophercloud/v2/openstack/identity/v3/tokens"
	"github.com/gophercloud/utils/v2/openstack/clientconfig"

	"github.com/notque/openstack-mcp-server/internal/config"
	"github.com/notque/openstack-mcp-server/internal/tools/shared"
)

// ensureTrailingSlash adds a trailing slash if not present.
func ensureTrailingSlash(url string) string {
	if !strings.HasSuffix(url, "/") {
		return url + "/"
	}
	return url
}

// Provider manages OpenStack authentication and provides service clients.
type Provider struct {
	providerClient *gophercloud.ProviderClient
	region         string
	cfg            *config.Config
	userID         string // cached user ID from auth result
}

// resolveSecret retrieves a secret from an env var, falling back to a command env var.
// For example: resolveSecret("OS_PASSWORD", "OS_PW_CMD") checks OS_PASSWORD first,
// then executes the OS_PW_CMD command if the password is empty.
// Returns the secret value (never stored in environment).
func resolveSecret(envKey, cmdEnvKey string) (string, error) {
	if val := os.Getenv(envKey); val != "" {
		return val, nil
	}
	if cmd := os.Getenv(cmdEnvKey); cmd != "" {
		out, err := exec.Command("sh", "-c", cmd).Output()
		if err != nil {
			return "", fmt.Errorf("executing %s: %w", cmdEnvKey, err)
		}
		result := strings.TrimSpace(string(out))
		if result == "" {
			return "", fmt.Errorf("%s returned empty output; ensure the credential store entry exists and access is granted", cmdEnvKey)
		}
		return result, nil
	}
	return "", nil
}

// NewProvider creates an authenticated provider using clouds.yaml or OS_* env vars.
func NewProvider(cfg *config.Config) (*Provider, error) {
	var authOpts *gophercloud.AuthOptions
	var region string

	if cfg.UseEnvAuth {
		// Check for application credential auth first (preferred for MCP servers).
		appCredID := os.Getenv("OS_APPLICATION_CREDENTIAL_ID")
		appCredName := os.Getenv("OS_APPLICATION_CREDENTIAL_NAME")

		if appCredID != "" || appCredName != "" {
			// SECURITY: Retrieve app credential secret from keychain/vault if needed.
			// The secret is held only in a local variable, never set in the environment.
			appCredSecret, err := resolveSecret("OS_APPLICATION_CREDENTIAL_SECRET", "OS_APPCRED_SECRET_CMD")
			if err != nil {
				return nil, err
			}
			if appCredSecret == "" {
				return nil, errors.New("OS_APPLICATION_CREDENTIAL_SECRET or OS_APPCRED_SECRET_CMD is required when using application credentials")
			}

			// App credentials carry their own scope — no Scope block needed.
			authOpts = &gophercloud.AuthOptions{
				IdentityEndpoint:            os.Getenv("OS_AUTH_URL"),
				ApplicationCredentialID:     appCredID,
				ApplicationCredentialName:   appCredName,
				ApplicationCredentialSecret: appCredSecret,
				AllowReauth:                 true,
			}
			// If using name-based app credentials, username + domain are needed for identification
			if appCredID == "" && appCredName != "" {
				authOpts.Username = os.Getenv("OS_USERNAME")
				authOpts.DomainName = os.Getenv("OS_USER_DOMAIN_NAME")
			}
		} else {
			// SECURITY: Retrieve password from keychain/vault.
			// The password is held only in a local variable, never set in the environment.
			password, err := resolveSecret("OS_PASSWORD", "OS_PW_CMD")
			if err != nil {
				return nil, err
			}

			// Build auth options from OS_* environment variables directly.
			// We handle this ourselves rather than using gophercloud's AuthOptionsFromEnv()
			// because SAP CC uses separate user/project domain names which that function
			// doesn't handle cleanly.
			authOpts = &gophercloud.AuthOptions{
				IdentityEndpoint: os.Getenv("OS_AUTH_URL"),
				Username:         os.Getenv("OS_USERNAME"),
				UserID:           os.Getenv("OS_USERID"),
				Password:         password,
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

	// SECURITY: Register the current token with the sanitizer for positive assertion.
	// This ensures the exact token string is redacted from any response, regardless of format.
	shared.SetCurrentToken(provider.TokenID)

	// SECURITY: Wrap the existing ReauthFunc to update the sanitizer when tokens refresh.
	// gophercloud's AllowReauth triggers silent token renewal on 401 — without this hook,
	// the sanitizer would hold a stale token and fail to redact the new one.
	if origReauth := provider.ReauthFunc; origReauth != nil {
		provider.ReauthFunc = func(ctx context.Context) error {
			if err := origReauth(ctx); err != nil {
				return err
			}
			shared.SetCurrentToken(provider.TokenID)
			return nil
		}
	}

	// Extract user ID from auth result for user-scoped APIs (e.g., application credentials).
	var userID string
	if authResult := provider.GetAuthResult(); authResult != nil {
		if createResult, ok := authResult.(tokens.CreateResult); ok {
			if user, err := createResult.ExtractUser(); err == nil {
				userID = user.ID
			}
		}
	}

	return &Provider{
		providerClient: provider,
		region:         region,
		cfg:            cfg,
		userID:         userID,
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

// KeyManagerClient returns an authenticated Barbican (key manager v1) client.
func (p *Provider) KeyManagerClient() (*gophercloud.ServiceClient, error) {
	return openstack.NewKeyManagerV1(p.providerClient, gophercloud.EndpointOpts{
		Region: p.region,
	})
}

// SharedFileSystemClient returns an authenticated Manila (shared file system v2) client.
func (p *Provider) SharedFileSystemClient() (*gophercloud.ServiceClient, error) {
	return openstack.NewSharedFileSystemV2(p.providerClient, gophercloud.EndpointOpts{
		Region: p.region,
	})
}

// ImageClient returns an authenticated Glance (image v2) client.
func (p *Provider) ImageClient() (*gophercloud.ServiceClient, error) {
	return openstack.NewImageV2(p.providerClient, gophercloud.EndpointOpts{
		Region: p.region,
	})
}

// BareMetalClient returns an authenticated Ironic (bare metal v1) client.
func (p *Provider) BareMetalClient() (*gophercloud.ServiceClient, error) {
	return openstack.NewBareMetalV1(p.providerClient, gophercloud.EndpointOpts{
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
		Endpoint:       ensureTrailingSlash(url),
		Type:           "endpoint-services",
		ResourceBase:   ensureTrailingSlash(url),
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

// CastellumClient returns an authenticated Castellum (autoscaling) client.
func (p *Provider) CastellumClient() (*gophercloud.ServiceClient, error) {
	endpointOpts := gophercloud.EndpointOpts{Region: p.region}
	endpointOpts.ApplyDefaults("castellum")

	url, err := p.providerClient.EndpointLocator(endpointOpts)
	if err != nil {
		return nil, fmt.Errorf("castellum endpoint not found in catalog: %w", err)
	}

	return &gophercloud.ServiceClient{
		ProviderClient: p.providerClient,
		Endpoint:       ensureTrailingSlash(url),
		Type:           "castellum",
		ResourceBase:   ensureTrailingSlash(url),
	}, nil
}

// CronusClient returns an authenticated Cronus (email/notification) client.
func (p *Provider) CronusClient() (*gophercloud.ServiceClient, error) {
	endpointOpts := gophercloud.EndpointOpts{Region: p.region}
	endpointOpts.ApplyDefaults("email-aws")

	url, err := p.providerClient.EndpointLocator(endpointOpts)
	if err != nil {
		return nil, fmt.Errorf("cronus endpoint not found in catalog: %w", err)
	}

	return &gophercloud.ServiceClient{
		ProviderClient: p.providerClient,
		Endpoint:       ensureTrailingSlash(url),
		Type:           "email-aws",
		ResourceBase:   ensureTrailingSlash(url),
	}, nil
}

// --- Accessors ---

// Token returns the current auth token for internal API calls.
// SECURITY: This value must NEVER be included in MCP tool responses.
// It is used only for server-side OpenStack API authentication.
func (p *Provider) Token() string {
	return p.providerClient.TokenID
}

// UserID returns the authenticated user's ID (from the auth token response).
// Required for user-scoped APIs like application credential management.
func (p *Provider) UserID() string {
	return p.userID
}

// Region returns the configured region.
func (p *Provider) Region() string {
	return p.region
}

// Config returns the server configuration.
func (p *Provider) Config() *config.Config {
	return p.cfg
}
