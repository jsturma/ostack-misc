package ostack

import (
	"context"

	"github.com/gophercloud/gophercloud/v2"
	"github.com/gophercloud/gophercloud/v2/openstack"
)

// NewProvider authenticates with Keystone (v3) and returns a Gophercloud ProviderClient.
// Endpoints for Compute, Block Storage, and Image are discovered from the catalog using cfg.Region.
func NewProvider(ctx context.Context, cfg *Config) (*gophercloud.ProviderClient, error) {
	opts := gophercloud.AuthOptions{
		IdentityEndpoint: cfg.KeystoneURL,
		Username:         cfg.User,
		Password:         cfg.Password,
		DomainName:       cfg.Domain,
		Scope: &gophercloud.AuthScope{
			ProjectName: cfg.Project,
			DomainName:  cfg.Domain,
		},
	}
	return openstack.AuthenticatedClient(ctx, opts)
}
