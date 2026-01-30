package ostack

import (
	"strings"
	"time"
)

const (
	StatusTimeout  = 1800 * time.Second
	StatusInterval = 5 * time.Second
	DefaultDomain  = "Default"
)

var (
	SupportedDiskFormats   = map[string]bool{"qcow2": true, "raw": true, "vmdk": true, "vdi": true}
	BackupSupportedStatuses = map[string]bool{"ACTIVE": true, "SHUTOFF": true, "PAUSED": true, "SUSPENDED": true}
)

// Config holds OpenStack endpoints and backup options.
type Config struct {
	KeystoneURL string
	CinderURL   string
	NovaURL     string
	GlanceURL   string
	Project     string
	User        string
	Password    string
	Domain      string
	BackupDir   string
	DiskFormat  string
	DiscoverAll bool
	VMFilter    string
	VMTags      string
	VMList      []string
}

// VMPair holds a VM name and its OpenStack server ID.
type VMPair struct {
	Name string
	ID   string
}

// NormalizeURLs ensures Cinder/Nova/Glance URLs contain project path and correct path suffixes.
func NormalizeURLs(cfg *Config) {
	ensureTrailingSlash := func(s string) string {
		if s != "" && !strings.HasSuffix(s, "/") {
			return s + "/"
		}
		return s
	}
	if cfg.CinderURL != "" && !strings.Contains(cfg.CinderURL, cfg.Project) {
		cfg.CinderURL = strings.TrimSuffix(cfg.CinderURL, "/") + "/" + cfg.Project
	}
	if cfg.NovaURL != "" && !strings.Contains(cfg.NovaURL, cfg.Project) {
		cfg.NovaURL = strings.TrimSuffix(ensureTrailingSlash(cfg.NovaURL), "/")
		if !strings.HasSuffix(cfg.NovaURL, "/v2.1") && !strings.Contains(cfg.NovaURL, "/v2.1/") {
			cfg.NovaURL = cfg.NovaURL + "/v2.1/" + cfg.Project
		} else {
			cfg.NovaURL = cfg.NovaURL + "/" + cfg.Project
		}
	}
	if cfg.GlanceURL != "" && !strings.Contains(cfg.GlanceURL, "/images") {
		cfg.GlanceURL = strings.TrimSuffix(ensureTrailingSlash(cfg.GlanceURL), "/")
		if !strings.HasSuffix(cfg.GlanceURL, "/v2") {
			cfg.GlanceURL = cfg.GlanceURL + "/v2/images"
		} else {
			cfg.GlanceURL = cfg.GlanceURL + "/images"
		}
	}
}
