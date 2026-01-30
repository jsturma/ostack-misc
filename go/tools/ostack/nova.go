package ostack

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/gophercloud/gophercloud/v2"
	"github.com/gophercloud/gophercloud/v2/openstack/compute/v2/servers"
	"github.com/gophercloud/gophercloud/v2/openstack/compute/v2/tags"
	"github.com/gophercloud/gophercloud/v2/pagination"
)

// ValidateVM checks that the server has id, name, and status.
func ValidateVM(ctx context.Context, client *gophercloud.ServiceClient, serverID string) bool {
	s, err := servers.Get(ctx, client, serverID).Extract()
	if err != nil || s == nil {
		return false
	}
	return s.ID != "" && s.Name != "" && s.Status != ""
}

// IsVMBackupSupported returns true for ACTIVE, SHUTOFF, PAUSED, SUSPENDED.
func IsVMBackupSupported(status string) bool {
	return BackupSupportedStatuses[status]
}

// GetVMID returns the server ID for a VM by name, or an error.
func GetVMID(ctx context.Context, client *gophercloud.ServiceClient, name string) (string, error) {
	pages, err := servers.List(client, servers.ListOpts{Name: name}).AllPages(ctx)
	if err != nil {
		return "", err
	}
	all, err := servers.ExtractServers(pages)
	if err != nil || len(all) == 0 {
		return "", fmt.Errorf("VM not found: %s", name)
	}
	id := all[0].ID
	if !ValidateVM(ctx, client, id) {
		return "", fmt.Errorf("invalid VM: %s", name)
	}
	return id, nil
}

// CheckVMTags returns true if the VM matches all tag/metadata filters.
func CheckVMTags(ctx context.Context, client *gophercloud.ServiceClient, vmID, filter string) bool {
	if filter == "" {
		return true
	}
	tagPairs := strings.Split(filter, ",")
	for _, pair := range tagPairs {
		kv := strings.SplitN(strings.TrimSpace(pair), ":", 2)
		key := strings.TrimSpace(kv[0])
		var expected string
		if len(kv) > 1 {
			expected = strings.TrimSpace(kv[1])
		}
		found := false
		// OpenStack server tags
		tagList, err := tags.List(ctx, client, vmID).Extract()
		if err == nil {
			for _, t := range tagList {
				if t == key {
					found = true
					break
				}
			}
		}
		if !found {
		meta, err := servers.Metadata(ctx, client, vmID).Extract()
		if err == nil && meta != nil {
			if v, ok := meta[key]; ok && v == expected {
				found = true
			}
		}
		}
		if !found {
			return false
		}
	}
	return true
}

func matchNameFilter(name, pattern string) bool {
	if pattern == "" {
		return true
	}
	matched, _ := filepath.Match(pattern, name)
	return matched
}

// DiscoverAllVMs returns VMs from Nova with pagination, filtered by status, name pattern, and tags.
func DiscoverAllVMs(ctx context.Context, client *gophercloud.ServiceClient, cfg *Config) ([]VMPair, error) {
	log.Println("Discovering VMs from OpenStack...")
	var result []VMPair
	opts := servers.ListOpts{AllTenants: true, Limit: 1000}
	err := servers.List(client, opts).EachPage(ctx, func(ctx context.Context, page pagination.Page) (bool, error) {
		srvList, err := servers.ExtractServers(page)
		if err != nil {
			return false, err
		}
		for _, s := range srvList {
			if s.Name == "" || s.ID == "" {
				continue
			}
			if s.Status == "ERROR" || s.Status == "DELETED" {
				log.Printf("Skipping %s (%s)", s.Name, s.Status)
				continue
			}
			if !IsVMBackupSupported(s.Status) {
				log.Printf("Skipping %s (unsupported status: %s)", s.Name, s.Status)
				continue
			}
			if !matchNameFilter(s.Name, cfg.VMFilter) {
				continue
			}
			if !CheckVMTags(ctx, client, s.ID, cfg.VMTags) {
				continue
			}
			if !ValidateVM(ctx, client, s.ID) {
				log.Printf("Skipping %s (invalid OpenStack VM structure)", s.Name)
				continue
			}
			result = append(result, VMPair{s.Name, s.ID})
		}
		return true, nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

// BackupVMConfig writes vm-config.json, vm-tags.json, and vm-metadata.json into backupDir.
func BackupVMConfig(ctx context.Context, client *gophercloud.ServiceClient, vmID, backupDir string) error {
	log.Println("Backing up VM configuration")
	s, err := servers.Get(ctx, client, vmID).Extract()
	if err != nil {
		return err
	}
	cfgJSON, _ := json.MarshalIndent(map[string]interface{}{"server": s}, "", "  ")
	if err := os.WriteFile(filepath.Join(backupDir, "vm-config.json"), cfgJSON, 0644); err != nil {
		log.Printf("Warning: Failed to save VM config: %v", err)
	}
	log.Println("Backing up VM tags")
	tagList, err := tags.List(ctx, client, vmID).Extract()
	if err != nil {
		_ = os.WriteFile(filepath.Join(backupDir, "vm-tags.json"), []byte(`{"tags":[]}`), 0644)
		log.Println("No tags found, saved empty tags file")
	} else {
		tagsJSON, _ := json.MarshalIndent(map[string]interface{}{"tags": tagList}, "", "  ")
		_ = os.WriteFile(filepath.Join(backupDir, "vm-tags.json"), tagsJSON, 0644)
		log.Println("VM tags saved to vm-tags.json")
	}
	log.Println("Backing up VM metadata")
	meta, err := servers.Metadata(ctx, client, vmID).Extract()
	if err != nil {
		_ = os.WriteFile(filepath.Join(backupDir, "vm-metadata.json"), []byte(`{"metadata":{}}`), 0644)
		log.Println("No metadata found, saved empty metadata file")
	} else {
		metaJSON, _ := json.MarshalIndent(map[string]interface{}{"metadata": meta}, "", "  ")
		_ = os.WriteFile(filepath.Join(backupDir, "vm-metadata.json"), metaJSON, 0644)
		log.Println("VM metadata saved to vm-metadata.json")
	}
	log.Println("VM configuration, tags, and metadata saved")
	return nil
}
