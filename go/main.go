// protect-ostack — Backup OpenStack VMs and their Cinder volumes (pure Go, stdlib only).
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/ostack-misc/protect-ostack/tools/ostack"
)

func usage() {
	fmt.Fprintf(os.Stderr, `Usage: protect-ostack [OPTIONS]

Required: --keystone-url URL --project NAME --user NAME --password PASSWORD
Optional: [--cinder-url URL] [--nova-url URL] [--glance-url URL] [--domain NAME]
         [--backup-dir DIR] [--disk-format FORMAT] [--discover-all] [--vm-filter PATTERN] [--vm-tags KEY:VALUE] [--vm-list VM1 VM2 ...]
         [--help]
         Format options: qcow2 (default), raw, vmdk, vdi
         Tag filter: --vm-tags backup:true or --vm-tags env:prod,team:ops

Examples:
  protect-ostack --keystone-url https://keystone.example.com:5000/v3 --project myproject --user myuser --password mypass
  protect-ostack --keystone-url https://keystone.internal:5000/v3 --project backup --user backup-user --password secret --vm-filter "prod-*"
`)
	os.Exit(0)
}

func parseFlags() *ostack.Config {
	cfg := &ostack.Config{
		Domain:      ostack.DefaultDomain,
		BackupDir:   "/backup/openstack",
		DiskFormat:  "qcow2",
		DiscoverAll: true,
	}
	flag.StringVar(&cfg.KeystoneURL, "keystone-url", "", "Keystone endpoint (required)")
	flag.StringVar(&cfg.CinderURL, "cinder-url", "", "Cinder endpoint (auto-discovered if omitted)")
	flag.StringVar(&cfg.NovaURL, "nova-url", "", "Nova endpoint (auto-discovered if omitted)")
	flag.StringVar(&cfg.GlanceURL, "glance-url", "", "Glance endpoint (auto-discovered if omitted)")
	flag.StringVar(&cfg.Project, "project", "", "OpenStack project (required)")
	flag.StringVar(&cfg.User, "user", "", "OpenStack user (required)")
	flag.StringVar(&cfg.Password, "password", "", "OpenStack password (required)")
	flag.StringVar(&cfg.Domain, "domain", ostack.DefaultDomain, "Domain")
	flag.StringVar(&cfg.BackupDir, "backup-dir", "/backup/openstack", "Backup directory")
	flag.StringVar(&cfg.DiskFormat, "disk-format", "qcow2", "Disk format: qcow2, raw, vmdk, vdi")
	flag.BoolVar(&cfg.DiscoverAll, "discover-all", true, "Discover all VMs")
	flag.BoolFunc("no-discover-all", "Use manual VM list", func(s string) error { cfg.DiscoverAll = false; return nil })
	flag.StringVar(&cfg.VMFilter, "vm-filter", "", "Filter VMs by name (e.g. prod-*)")
	flag.StringVar(&cfg.VMTags, "vm-tags", "", "Filter by tags/metadata (e.g. backup:true)")
	flag.Func("vm-list", "Manual VM list (space-separated)", func(s string) error {
		cfg.VMList = strings.Fields(s)
		cfg.DiscoverAll = false
		return nil
	})
	flag.Usage = usage
	flag.Parse()

	if cfg.KeystoneURL == "" || cfg.Project == "" || cfg.User == "" || cfg.Password == "" {
		log.Fatal("Missing required flags: --keystone-url, --project, --user, --password")
	}
	if !ostack.SupportedDiskFormats[cfg.DiskFormat] {
		log.Fatalf("Invalid disk format: %s (supported: qcow2, raw, vmdk, vdi)", cfg.DiskFormat)
	}
	ostack.NormalizeURLs(cfg)
	return cfg
}

func main() {
	log.SetFlags(log.Ldate | log.Ltime)
	cfg := parseFlags()
	if err := os.MkdirAll(cfg.BackupDir, 0755); err != nil {
		log.Fatalf("Cannot create backup dir: %v", err)
	}
	log.Printf("Starting backup - Keystone: %s, Project: %s, User: %s, Dir: %s",
		cfg.KeystoneURL, cfg.Project, cfg.User, cfg.BackupDir)

	client := ostack.NewClient()
	token, catalog, err := ostack.GetTokenAndCatalog(client, cfg)
	if err != nil {
		log.Fatalf("Auth failed: %v", err)
	}
	log.Println("Token obtained")

	if err := ostack.DiscoverServiceEndpoints(cfg, catalog); err != nil {
		log.Fatal(err)
	}
	if cfg.CinderURL == "" || cfg.NovaURL == "" || cfg.GlanceURL == "" {
		log.Fatal("Service URLs not configured")
	}
	log.Printf("Endpoints - Cinder: %s, Nova: %s, Glance: %s", cfg.CinderURL, cfg.NovaURL, cfg.GlanceURL)

	if err := ostack.Run(client, cfg, token); err != nil {
		log.Fatal(err)
	}
	log.Println("=== ALL BACKUPS COMPLETED ===")
}
