package ostack

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// defaultConfigYAML is written when cfg/config.yaml does not exist.
const defaultConfigYAML = `# protect-ostack default config (edit as needed)
# Required at runtime or via CLI: keystone_url, project, user, password

keystone_url: ""
project: ""
user: ""
password: ""

domain: "Default"
region: "RegionOne"
backup_dir: "/backup/openstack"
disk_format: "qcow2"
discover_all: true
max_parallel_snap_shots: 0
max_parallel_volumes: 0
status_timeout_sec: 1800
status_interval_sec: 5
vm_filter: ""
vm_tags: ""
vm_list: []
`

// LoadConfig reads config from path (YAML). If the file does not exist,
// it writes the default config to path and then loads it.
func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			if err := ensureDefaultConfig(path); err != nil {
				return nil, fmt.Errorf("create default config: %w", err)
			}
			data, err = os.ReadFile(path)
			if err != nil {
				return nil, fmt.Errorf("read config: %w", err)
			}
		} else {
			return nil, fmt.Errorf("read config: %w", err)
		}
	}
	cfg := &Config{}
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	return cfg, nil
}

func ensureDefaultConfig(path string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(defaultConfigYAML), 0644)
}
