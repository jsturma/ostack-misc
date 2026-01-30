package ostack

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/gophercloud/gophercloud/v2"
	"github.com/gophercloud/gophercloud/v2/openstack"
	"github.com/gophercloud/gophercloud/v2/openstack/blockstorage/v3/snapshots"
	"github.com/gophercloud/gophercloud/v2/openstack/blockstorage/v3/volumes"
	"github.com/gophercloud/gophercloud/v2/openstack/image/v2/imagedata"
	"github.com/gophercloud/gophercloud/v2/openstack/image/v2/images"
	"golang.org/x/sync/errgroup"
)

// BackupVolume creates a snapshot, temp volume, uploads to Glance, downloads the image file, then cleans up.
func BackupVolume(ctx context.Context, blockClient *gophercloud.ServiceClient, imageClient *gophercloud.ServiceClient, cfg *Config, volID, backupDir string) error {
	timestamp := time.Now().Format("2006-01-02_1504")
	log.Printf("Backing up volume %s", volID)

	// Create snapshot
	snap, err := snapshots.Create(ctx, blockClient, snapshots.CreateOpts{
		VolumeID: volID,
		Name:     "snap-" + volID + "-" + timestamp,
		Force:    true,
	}).Extract()
	if err != nil {
		return err
	}
	snapID := snap.ID
	defer func() {
		if err := snapshots.Delete(ctx, blockClient, snapID).ExtractErr(); err != nil {
			log.Printf("Warning: Failed to delete snapshot %s: %v", snapID, err)
		} else {
			log.Printf("Cleaned up snapshot: %s", snapID)
		}
	}()

	err = snapshots.WaitForStatus(ctx, blockClient, snapID, "available")
	if err != nil {
		return err
	}

	vol, err := volumes.Get(ctx, blockClient, volID).Extract()
	if err != nil {
		return err
	}
	volSize := vol.Size
	log.Printf("Creating temp volume (%dGB)", volSize)

	tmpVol, err := volumes.Create(ctx, blockClient, volumes.CreateOpts{
		SnapshotID: snapID,
		Size:       volSize,
		Name:       "tmp-" + volID + "-" + timestamp,
	}, nil).Extract()
	if err != nil {
		return err
	}
	tmpVolID := tmpVol.ID
	defer func() {
		if err := volumes.Delete(ctx, blockClient, tmpVolID, volumes.DeleteOpts{}).ExtractErr(); err != nil {
			log.Printf("Warning: Failed to delete volume %s: %v", tmpVolID, err)
		} else {
			log.Printf("Cleaned up volume: %s", tmpVolID)
		}
	}()

	err = volumes.WaitForStatus(ctx, blockClient, tmpVolID, "available")
	if err != nil {
		return err
	}

	log.Printf("Creating image (%s format)", cfg.DiskFormat)
	imgResult, err := volumes.UploadImage(ctx, blockClient, tmpVolID, volumes.UploadImageOpts{
		ImageName: "img-" + volID + "-" + timestamp,
		DiskFormat: cfg.DiskFormat,
		ContainerFormat: "bare",
	}).Extract()
	if err != nil {
		return err
	}
	imgID := imgResult.ImageID
	defer func() {
		if err := images.Delete(ctx, imageClient, imgID).ExtractErr(); err != nil {
			log.Printf("Warning: Failed to delete image %s: %v", imgID, err)
		} else {
			log.Printf("Cleaned up image: %s", imgID)
		}
	}()

	// Wait for image to become active
	deadline := time.Now().Add(StatusTimeout)
	for time.Now().Before(deadline) {
		img, err := images.Get(ctx, imageClient, imgID).Extract()
		if err != nil {
			return err
		}
		if img.Status == "active" {
			log.Printf("Image %s is active", imgID)
			break
		}
		if img.Status == "error" || img.Status == "killed" {
			return fmt.Errorf("image %s entered %s state", imgID, img.Status)
		}
		time.Sleep(StatusInterval)
	}

	outPath := filepath.Join(backupDir, volID+"."+cfg.DiskFormat)
	log.Printf("Downloading %s to %s", cfg.DiskFormat, outPath)
	res := imagedata.Download(ctx, imageClient, imgID)
	rc, err := res.Extract()
	if err != nil {
		return err
	}
	defer rc.Close()
	f, err := os.Create(outPath)
	if err != nil {
		return err
	}
	n, err := io.Copy(f, rc)
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

// Run performs the full backup using Gophercloud: discover or use VM list, then backs up all VMs in parallel; within each VM, volume backups run in parallel.
func Run(ctx context.Context, provider *gophercloud.ProviderClient, cfg *Config) error {
	computeClient, err := openstack.NewComputeV2(provider, gophercloud.EndpointOpts{Region: cfg.Region})
	if err != nil {
		return fmt.Errorf("compute client: %w", err)
	}
	blockClient, err := openstack.NewBlockStorageV3(provider, gophercloud.EndpointOpts{Region: cfg.Region})
	if err != nil {
		return fmt.Errorf("block storage client: %w", err)
	}
	imageClient, err := openstack.NewImageV2(provider, gophercloud.EndpointOpts{Region: cfg.Region})
	if err != nil {
		return fmt.Errorf("image client: %w", err)
	}

	var vms []VMPair
	if cfg.DiscoverAll {
		if cfg.VMFilter != "" {
			log.Printf("VM name filter: %s", cfg.VMFilter)
		}
		if cfg.VMTags != "" {
			log.Printf("VM tag filter: %s", cfg.VMTags)
		}
		discovered, err := DiscoverAllVMs(ctx, computeClient, cfg)
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
			id, err := GetVMID(ctx, computeClient, name)
			if err != nil {
				log.Printf("VM %q not found or invalid: %v", name, err)
				continue
			}
			vms = append(vms, VMPair{name, id})
		}
	}

	g, gCtx := errgroup.WithContext(ctx)
	for _, v := range vms {
		v := v
		if !ValidateVM(gCtx, computeClient, v.ID) {
			log.Printf("Skipping %s (invalid OpenStack VM)", v.Name)
			continue
		}
		g.Go(func() error {
			log.Printf("==== VM: %s (ID: %s) ====", v.Name, v.ID)
			vmDir := filepath.Join(cfg.BackupDir, v.Name, time.Now().Format("2006-01-02_15-04"))
			if err := os.MkdirAll(vmDir, 0755); err != nil {
				return fmt.Errorf("create %s: %w", vmDir, err)
			}
			if err := BackupVMConfig(gCtx, computeClient, v.ID, vmDir); err != nil {
				log.Printf("Failed VM config backup for %s: %v", v.Name, err)
			}
			vols, err := GetAttachedVolumes(gCtx, blockClient, v.ID)
			if err != nil {
				return fmt.Errorf("%s: list volumes: %w", v.Name, err)
			}
			if len(vols) == 0 {
				log.Printf("No volumes for %s", v.Name)
				return nil
			}
			g2, g2Ctx := errgroup.WithContext(gCtx)
			for _, volID := range vols {
				volID := volID
				g2.Go(func() error {
					if err := BackupVolume(g2Ctx, blockClient, imageClient, cfg, volID, vmDir); err != nil {
						return fmt.Errorf("volume %s: %w", volID, err)
					}
					return nil
				})
			}
			if err := g2.Wait(); err != nil {
				return fmt.Errorf("%s: %w", v.Name, err)
			}
			log.Printf("Completed: %s", v.Name)
			return nil
		})
	}
	return g.Wait()
}
