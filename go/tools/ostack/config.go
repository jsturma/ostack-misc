package ostack

const (
	DefaultConfigDir  = "cfg"
	DefaultConfigPath = "cfg/config.yaml"
)

var (
	SupportedDiskFormats   = map[string]bool{"qcow2": true, "raw": true, "vmdk": true, "vdi": true}
	BackupSupportedStatuses = map[string]bool{"ACTIVE": true, "SHUTOFF": true, "PAUSED": true, "SUSPENDED": true}
)

// Config holds OpenStack auth and backup options.
// Loaded from YAML (default: cfg/config.yaml); CLI flags override.
type Config struct {
	KeystoneURL string `yaml:"keystone_url"`
	CinderURL   string `yaml:"cinder_url"`   // unused with Gophercloud
	NovaURL     string `yaml:"nova_url"`
	GlanceURL   string `yaml:"glance_url"`
	Project     string `yaml:"project"`
	User        string `yaml:"user"`
	Password    string `yaml:"password"`
	Domain      string `yaml:"domain"`
	Region      string `yaml:"region"`
	BackupDir   string `yaml:"backup_dir"`
	DiskFormat  string `yaml:"disk_format"`
	DiscoverAll bool   `yaml:"discover_all"`
	VMFilter    string `yaml:"vm_filter"`
	VMTags      string `yaml:"vm_tags"`
	VMList      []string `yaml:"vm_list"`
	MaxParallelSnapShots int `yaml:"max_parallel_snap_shots"`
	MaxParallelVolumes   int `yaml:"max_parallel_volumes"`
	// StatusTimeoutSec is max wait (seconds) for snapshot/volume/image to reach target status.
	StatusTimeoutSec int `yaml:"status_timeout_sec"`
	// StatusIntervalSec is poll interval (seconds) while waiting.
	StatusIntervalSec int `yaml:"status_interval_sec"`
}

// VMPair holds a VM name and its OpenStack server ID.
type VMPair struct {
	Name string
	ID   string
}
