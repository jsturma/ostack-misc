package ostack

import (
	"context"
	"fmt"

	"github.com/gophercloud/gophercloud/v2"
	"github.com/gophercloud/gophercloud/v2/openstack/blockstorage/v3/volumes"
	"github.com/gophercloud/gophercloud/v2/pagination"
)

// GetAttachedVolumes returns volume IDs attached to the given server.
func GetAttachedVolumes(ctx context.Context, client *gophercloud.ServiceClient, serverID string) ([]string, error) {
	var ids []string
	err := volumes.List(client, volumes.ListOpts{AllTenants: true}).EachPage(ctx, func(ctx context.Context, page pagination.Page) (bool, error) {
		volList, err := volumes.ExtractVolumes(page)
		if err != nil {
			return false, err
		}
		for _, v := range volList {
			for _, a := range v.Attachments {
				if a.ServerID == serverID {
					ids = append(ids, v.ID)
					break
				}
			}
		}
		return true, nil
	})
	return ids, err
}

// GetVolumeSize returns the volume size in GB.
func GetVolumeSize(ctx context.Context, client *gophercloud.ServiceClient, volID string) (int, error) {
	v, err := volumes.Get(ctx, client, volID).Extract()
	if err != nil {
		return 0, err
	}
	if v.Size == 0 {
		return 0, fmt.Errorf("invalid volume size")
	}
	return v.Size, nil
}
