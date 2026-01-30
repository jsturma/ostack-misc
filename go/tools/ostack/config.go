package ostack

import "time"

const (
	StatusTimeout  = 1800 * time.Second
	StatusInterval = 5 * time.Second
	DefaultDomain  = "Default"
	DefaultRegion  = "RegionOne"
)

var (
	SupportedDiskFormats   = map[string]bool{"qcow2": true, "raw": true, "vmdk": true, "vdi": true}
	BackupSupportedStatuses = map[string]bool{"ACTIVE": true, "SHUTOFF": true, "PAUSED": true, "SUSPENDED": true}
)

// Config holds OpenStack auth and backup options.
// With Gophercloud, Compute/Cinder/Glance endpoints are discovered from the catalog using Region.
type Config struct {
	KeystoneURL string
	CinderURL   string // unused with Gophercloud; kept for CLI compatibility
	NovaURL     string
	GlanceURL   string
	Project     string
	User        string
	Password    string
	Domain      string
	Region      string
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
