package ostack

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// ValidateVM checks that the server has id, name, and status.
func ValidateVM(client *http.Client, novaURL, token, serverID string) bool {
	data, err := apiGet(client, novaURL+"/servers/"+serverID, token)
	if err != nil || len(data) == 0 {
		return false
	}
	var out struct {
		Server struct {
			ID     string `json:"id"`
			Name   string `json:"name"`
			Status string `json:"status"`
		} `json:"server"`
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return false
	}
	return out.Server.ID != "" && out.Server.Name != "" && out.Server.Status != ""
}

// IsVMBackupSupported returns true for ACTIVE, SHUTOFF, PAUSED, SUSPENDED.
func IsVMBackupSupported(status string) bool {
	return BackupSupportedStatuses[status]
}

// GetVMID returns the server ID for a VM by name, or an error.
func GetVMID(client *http.Client, novaURL, token, name string) (string, error) {
	url := novaURL + "/servers?name=" + name
	data, err := apiGet(client, url, token)
	if err != nil {
		return "", err
	}
	var out struct {
		Servers []struct {
			ID string `json:"id"`
		} `json:"servers"`
	}
	if err := json.Unmarshal(data, &out); err != nil || len(out.Servers) == 0 {
		return "", fmt.Errorf("VM not found: %s", name)
	}
	id := out.Servers[0].ID
	if !ValidateVM(client, novaURL, token, id) {
		return "", fmt.Errorf("invalid VM: %s", name)
	}
	return id, nil
}

// CheckVMTags returns true if the VM matches all tag/metadata filters (tags and metadata endpoints).
func CheckVMTags(client *http.Client, novaURL, token, vmID, filter string) bool {
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
		tagsData, err := apiGet(client, novaURL+"/servers/"+vmID+"/tags", token)
		if err == nil {
			var tagResp struct {
				Tags []string `json:"tags"`
			}
			if json.Unmarshal(tagsData, &tagResp) == nil {
				for _, t := range tagResp.Tags {
					if t == key {
						found = true
						break
					}
				}
			}
		}
		if !found {
			metaData, err := apiGet(client, novaURL+"/servers/"+vmID+"/metadata", token)
			if err == nil {
				var metaResp struct {
					Metadata map[string]string `json:"metadata"`
				}
				if json.Unmarshal(metaData, &metaResp) == nil && metaResp.Metadata != nil {
					if v, ok := metaResp.Metadata[key]; ok && v == expected {
						found = true
					}
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
func DiscoverAllVMs(client *http.Client, cfg *Config, token string) ([]VMPair, error) {
	log.Println("Discovering VMs from OpenStack...")
	var result []VMPair
	url := cfg.NovaURL + "/servers?all_tenants=1&limit=1000"
	for {
		data, err := apiGet(client, url, token)
		if err != nil {
			return nil, err
		}
		var page struct {
			Servers []struct {
				ID     string `json:"id"`
				Name   string `json:"name"`
				Status string `json:"status"`
			} `json:"servers"`
			Links []struct {
				Rel  string `json:"rel"`
				Href string `json:"href"`
			} `json:"servers_links"`
		}
		if err := json.Unmarshal(data, &page); err != nil {
			return nil, err
		}
		for _, s := range page.Servers {
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
			if !CheckVMTags(client, cfg.NovaURL, token, s.ID, cfg.VMTags) {
				continue
			}
			if !ValidateVM(client, cfg.NovaURL, token, s.ID) {
				log.Printf("Skipping %s (invalid OpenStack VM structure)", s.Name)
				continue
			}
			result = append(result, VMPair{s.Name, s.ID})
		}
		next := ""
		for _, l := range page.Links {
			if l.Rel == "next" {
				next = l.Href
				break
			}
		}
		if next == "" {
			break
		}
		log.Println("Fetching next page...")
		url = next
	}
	return result, nil
}

// BackupVMConfig writes vm-config.json, vm-tags.json, and vm-metadata.json into backupDir.
func BackupVMConfig(client *http.Client, novaURL, token, vmID, backupDir string) error {
	log.Println("Backing up VM configuration")
	data, err := apiGet(client, novaURL+"/servers/"+vmID, token)
	if err != nil {
		return err
	}
	var pretty bytes.Buffer
	if err := json.Indent(&pretty, data, "", "  "); err != nil {
		pretty.Write(data)
	}
	if err := os.WriteFile(filepath.Join(backupDir, "vm-config.json"), pretty.Bytes(), 0644); err != nil {
		log.Printf("Warning: Failed to save VM config: %v", err)
	}
	log.Println("Backing up VM tags")
	tagsData, err := apiGet(client, novaURL+"/servers/"+vmID+"/tags", token)
	if err != nil {
		_ = os.WriteFile(filepath.Join(backupDir, "vm-tags.json"), []byte(`{"tags":[]}`), 0644)
		log.Println("No tags found, saved empty tags file")
	} else {
		var pretty bytes.Buffer
		json.Indent(&pretty, tagsData, "", "  ")
		_ = os.WriteFile(filepath.Join(backupDir, "vm-tags.json"), pretty.Bytes(), 0644)
		log.Println("VM tags saved to vm-tags.json")
	}
	log.Println("Backing up VM metadata")
	metaData, err := apiGet(client, novaURL+"/servers/"+vmID+"/metadata", token)
	if err != nil {
		_ = os.WriteFile(filepath.Join(backupDir, "vm-metadata.json"), []byte(`{"metadata":{}}`), 0644)
		log.Println("No metadata found, saved empty metadata file")
	} else {
		var pretty bytes.Buffer
		json.Indent(&pretty, metaData, "", "  ")
		_ = os.WriteFile(filepath.Join(backupDir, "vm-metadata.json"), pretty.Bytes(), 0644)
		log.Println("VM metadata saved to vm-metadata.json")
	}
	log.Println("VM configuration, tags, and metadata saved")
	return nil
}
