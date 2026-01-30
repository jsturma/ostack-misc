package ostack

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

// BackupVolume creates a snapshot, temp volume, Glance image, downloads the image file, then cleans up.
func BackupVolume(client *http.Client, cfg *Config, token, volID, backupDir string) error {
	timestamp := time.Now().Format("2006-01-02_1504")
	log.Printf("Backing up volume %s", volID)

	snapBody := map[string]interface{}{
		"snapshot": map[string]interface{}{
			"volume_id": volID,
			"name":      "snap-" + volID + "-" + timestamp,
			"force":     true,
		},
	}
	snapJSON, _ := json.Marshal(snapBody)
	snapResp, err := apiCall(client, http.MethodPost, cfg.CinderURL+"/snapshots", token, snapJSON)
	if err != nil {
		return err
	}
	var snapOut struct {
		Snapshot struct {
			ID string `json:"id"`
		} `json:"snapshot"`
	}
	if err := json.Unmarshal(snapResp, &snapOut); err != nil || snapOut.Snapshot.ID == "" {
		return fmt.Errorf("failed to create snapshot")
	}
	snapID := snapOut.Snapshot.ID
	defer CleanupResource(client, "snapshot", snapID, cfg.CinderURL, cfg.GlanceURL, token)

	if err := WaitStatus(client, "snapshot", snapID, "available", cfg.CinderURL, cfg.GlanceURL, token); err != nil {
		return err
	}

	volSize, err := GetVolumeSize(client, cfg.CinderURL, token, volID)
	if err != nil {
		return err
	}
	log.Printf("Creating temp volume (%dGB)", volSize)
	volBody := map[string]interface{}{
		"volume": map[string]interface{}{
			"snapshot_id": snapID,
			"size":        volSize,
			"name":        "tmp-" + volID + "-" + timestamp,
		},
	}
	volJSON, _ := json.Marshal(volBody)
	volResp, err := apiCall(client, http.MethodPost, cfg.CinderURL+"/volumes", token, volJSON)
	if err != nil {
		return err
	}
	var volOut struct {
		Volume struct {
			ID string `json:"id"`
		} `json:"volume"`
	}
	if err := json.Unmarshal(volResp, &volOut); err != nil || volOut.Volume.ID == "" {
		return fmt.Errorf("failed to create temp volume")
	}
	tmpVolID := volOut.Volume.ID
	defer CleanupResource(client, "volume", tmpVolID, cfg.CinderURL, cfg.GlanceURL, token)

	if err := WaitStatus(client, "volume", tmpVolID, "available", cfg.CinderURL, cfg.GlanceURL, token); err != nil {
		return err
	}

	log.Printf("Creating image (%s format)", cfg.DiskFormat)
	imgBody := map[string]interface{}{
		"name":             "img-" + volID + "-" + timestamp,
		"disk_format":      cfg.DiskFormat,
		"container_format": "bare",
		"volume_id":       tmpVolID,
	}
	imgJSON, _ := json.Marshal(imgBody)
	imgResp, err := apiCall(client, http.MethodPost, cfg.GlanceURL, token, imgJSON)
	if err != nil {
		return err
	}
	var imgOut struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(imgResp, &imgOut); err != nil || imgOut.ID == "" {
		return fmt.Errorf("failed to create image")
	}
	imgID := imgOut.ID
	defer CleanupResource(client, "image", imgID, cfg.CinderURL, cfg.GlanceURL, token)

	if err := WaitStatus(client, "image", imgID, "active", cfg.CinderURL, cfg.GlanceURL, token); err != nil {
		return err
	}

	outPath := filepath.Join(backupDir, volID+"."+cfg.DiskFormat)
	log.Printf("Downloading %s to %s", cfg.DiskFormat, outPath)
	req, err := http.NewRequest(http.MethodGet, cfg.GlanceURL+"/"+imgID+"/file", nil)
	if err != nil {
		return err
	}
	req.Header.Set("X-Auth-Token", token)
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("download image: HTTP %d", resp.StatusCode)
	}
	f, err := os.Create(outPath)
	if err != nil {
		return err
	}
	n, err := io.Copy(f, resp.Body)
	f.Close()
	if err != nil {
		os.Remove(outPath)
		return err
	}
	if n == 0 {
		os.Remove(outPath)
		return fmt.Errorf("downloaded file is empty")
	}
	log.Printf("Downloaded %s", outPath)
	log.Printf("Volume %s backed up", volID)
	return nil
}

// Run performs the full backup: discover or use VM list, then for each VM backup config and volumes.
func Run(client *http.Client, cfg *Config, token string) error {
	var vms []VMPair
	if cfg.DiscoverAll {
		if cfg.VMFilter != "" {
			log.Printf("VM name filter: %s", cfg.VMFilter)
		}
		if cfg.VMTags != "" {
			log.Printf("VM tag filter: %s", cfg.VMTags)
		}
		discovered, err := DiscoverAllVMs(client, cfg, token)
		if err != nil {
			return err
		}
		if len(discovered) == 0 {
			log.Println("No VMs found")
			return nil
		}
		log.Printf("Discovered %d VM(s)", len(discovered))
		vms = discovered
	} else {
		log.Println("Using manual VM list")
		for _, name := range cfg.VMList {
			id, err := GetVMID(client, cfg.NovaURL, token, name)
			if err != nil {
				log.Printf("VM %q not found or invalid: %v", name, err)
				continue
			}
			vms = append(vms, VMPair{name, id})
		}
	}

	for _, v := range vms {
		if !ValidateVM(client, cfg.NovaURL, token, v.ID) {
			log.Printf("Skipping %s (invalid OpenStack VM)", v.Name)
			continue
		}
		log.Printf("==== VM: %s (ID: %s) ====", v.Name, v.ID)
		vmDir := filepath.Join(cfg.BackupDir, v.Name, time.Now().Format("2006-01-02_15-04"))
		if err := os.MkdirAll(vmDir, 0755); err != nil {
			log.Printf("Failed to create %s: %v", vmDir, err)
			continue
		}
		if err := BackupVMConfig(client, cfg.NovaURL, token, v.ID, vmDir); err != nil {
			log.Printf("Failed VM config backup for %s: %v", v.Name, err)
		}
		vols, err := GetAttachedVolumes(client, cfg.CinderURL, token, v.ID)
		if err != nil {
			log.Printf("Failed to list volumes for %s: %v", v.Name, err)
			continue
		}
		if len(vols) == 0 {
			log.Printf("No volumes for %s", v.Name)
			continue
		}
		for _, volID := range vols {
			if err := BackupVolume(client, cfg, token, volID, vmDir); err != nil {
				log.Printf("Failed volume %s: %v", volID, err)
			}
		}
		log.Printf("Completed: %s", v.Name)
	}
	return nil
}
