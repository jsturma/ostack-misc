package ostack

import (
	"encoding/json"
	"fmt"
	"net/http"
)

// GetAttachedVolumes returns volume IDs attached to the given server.
func GetAttachedVolumes(client *http.Client, cinderURL, token, serverID string) ([]string, error) {
	data, err := apiGet(client, cinderURL+"/volumes?all_tenants=1", token)
	if err != nil {
		return nil, err
	}
	var out struct {
		Volumes []struct {
			ID          string `json:"id"`
			Attachments []struct {
				ServerID string `json:"server_id"`
			} `json:"attachments"`
		} `json:"volumes"`
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, err
	}
	var ids []string
	for _, v := range out.Volumes {
		for _, a := range v.Attachments {
			if a.ServerID == serverID {
				ids = append(ids, v.ID)
				break
			}
		}
	}
	return ids, nil
}

// GetVolumeSize returns the volume size in GB.
func GetVolumeSize(client *http.Client, cinderURL, token, volID string) (int, error) {
	data, err := apiGet(client, cinderURL+"/volumes/"+volID, token)
	if err != nil {
		return 0, err
	}
	var out struct {
		Volume struct {
			Size int `json:"size"`
		} `json:"volume"`
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return 0, err
	}
	if out.Volume.Size == 0 {
		return 0, fmt.Errorf("invalid volume size")
	}
	return out.Volume.Size, nil
}
