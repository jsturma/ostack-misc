// protect-ostack — Backup OpenStack VMs and their Cinder volumes (Go + Gophercloud).
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/ostack-misc/protect-ostack/tools/ostack"
)

func usage() {
	fmt.Fprintf(os.Stderr, `Usage: protect-ostack [OPTIONS]

Config: defaults from cfg/config.yaml (or --config PATH). CLI overrides config file.

Required (in config or CLI): --keystone-url URL --project NAME --user NAME --password PASSWORD
Optional: [--config PATH] [--region NAME] [--domain NAME] [--backup-dir DIR] [--disk-format FORMAT]
         [--max-parallel-snap N] [--max-parallel-vol N] [--discover-all] [--vm-filter PATTERN] [--vm-tags KEY:VALUE] [--vm-list VM1 VM2 ...]
         [--help]

Examples:
  protect-ostack --keystone-url https://keystone.example.com:5000/v3 --project myproject --user myuser --password mypass
  protect-ostack --config cfg/config.yaml
`)
	os.Exit(0)
}

func configPathFromArgs() string {
	for i, a := range os.Args {
		if (a == "--config" || a == "-config") && i+1 < len(os.Args) {
			return os.Args[i+1]
		}
	}
	return ostack.DefaultConfigPath
}

func parseFlags() *ostack.Config {
	configPath := configPathFromArgs()
	cfg, err := ostack.LoadConfig(configPath)
	if err != nil {
		log.Fatalf("Load config %s: %v", configPath, err)
	}

	var configFilePath string
	flag.StringVar(&configFilePath, "config", configPath, "Path to config file (YAML)")
	flag.StringVar(&cfg.KeystoneURL, "keystone-url", cfg.KeystoneURL, "Keystone endpoint (e.g. https://keystone.example.com:5000/v3)")
	flag.StringVar(&cfg.Project, "project", cfg.Project, "OpenStack project")
	flag.StringVar(&cfg.User, "user", cfg.User, "OpenStack user")
	flag.StringVar(&cfg.Password, "password", cfg.Password, "OpenStack password")
	flag.StringVar(&cfg.Domain, "domain", cfg.Domain, "Domain")
	flag.StringVar(&cfg.Region, "region", cfg.Region, "OpenStack region for service discovery")
	flag.StringVar(&cfg.BackupDir, "backup-dir", cfg.BackupDir, "Backup directory")
	flag.StringVar(&cfg.DiskFormat, "disk-format", cfg.DiskFormat, "Disk format: qcow2, raw, vmdk, vdi")
	flag.IntVar(&cfg.MaxParallelSnapShots, "max-parallel-snap", cfg.MaxParallelSnapShots, "Max concurrent VM backup tasks (snapshots); 0 = unlimited")
	flag.IntVar(&cfg.MaxParallelVolumes, "max-parallel-vol", cfg.MaxParallelVolumes, "Max concurrent volume backups across all VMs; 0 = unlimited")
	flag.BoolVar(&cfg.DiscoverAll, "discover-all", cfg.DiscoverAll, "Discover all VMs")
	flag.BoolFunc("no-discover-all", "Use manual VM list", func(s string) error { cfg.DiscoverAll = false; return nil })
	flag.StringVar(&cfg.VMFilter, "vm-filter", cfg.VMFilter, "Filter VMs by name (e.g. prod-*)")
	flag.StringVar(&cfg.VMTags, "vm-tags", cfg.VMTags, "Filter by tags/metadata (e.g. backup:true)")
	flag.Func("vm-list", "Manual VM list (space-separated)", func(s string) error {
		cfg.VMList = strings.Fields(s)
		cfg.DiscoverAll = false
		return nil
	})
	flag.Usage = usage
	flag.Parse()

	if cfg.KeystoneURL == "" || cfg.Project == "" || cfg.User == "" || cfg.Password == "" {
		log.Fatal("Missing required: keystone_url, project, user, password (set in cfg/config.yaml or via CLI)")
	}
	if !ostack.SupportedDiskFormats[cfg.DiskFormat] {
		log.Fatalf("Invalid disk format: %s (supported: qcow2, raw, vmdk, vdi)", cfg.DiskFormat)
	}
	return cfg
}

func main() {
	log.SetFlags(log.Ldate | log.Ltime)
	cfg := parseFlags()
	if err := os.MkdirAll(cfg.BackupDir, 0755); err != nil {
		log.Fatalf("Cannot create backup dir: %v", err)
	}
	log.Printf("Starting backup - Keystone: %s, Project: %s, Region: %s, Dir: %s",
		cfg.KeystoneURL, cfg.Project, cfg.Region, cfg.BackupDir)

	ctx := context.Background()
	provider, err := ostack.NewProvider(ctx, cfg)
	if err != nil {
		log.Fatalf("Auth failed: %v", err)
	}
	log.Println("Authenticated with OpenStack (Gophercloud)")

	if err := ostack.Run(ctx, provider, cfg); err != nil {
		log.Fatal(err)
	}
	log.Println("=== ALL BACKUPS COMPLETED ===")
}
