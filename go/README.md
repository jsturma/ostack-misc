# protect-ostack (Go)

**Source:** [github.com/jsturma/ostack-misc/tree/main/go](https://github.com/jsturma/ostack-misc/tree/main/go)

Go port of the [bash backup script](../scripts/bash/protect-ostack.sh), using [Gophercloud v2](https://github.com/gophercloud/gophercloud) for OpenStack API access.

OpenStack logic lives in the **ostack** library (`tools/ostack/`): Keystone v3 auth (project-scoped token), Nova (VMs, tags, metadata), Cinder (volumes, snapshots), Glance (images), and backup orchestration. **Defaults come from a YAML config file** (default path: `cfg/config.yaml`); CLI flags override. If the file is missing, it is created with defaults. Service endpoints are discovered from the Keystone catalog using the configured **region**. **Backups run in parallel**: all VMs are backed up concurrently, and within each VM all volume backups run concurrently. Use `--max-parallel-snap N` and `--max-parallel-vol N` to limit concurrency (0 = unlimited). `main.go` loads config and handles CLI flags.

## Build

```bash
go build -o protect-ostack .
```

## Config

All defaults live in **`cfg/config.yaml`** (create the file or run once to auto-create). Edit `domain`, `region`, `backup_dir`, `disk_format`, `discover_all`, `max_parallel_snap_shots`, `max_parallel_volumes`, etc. Use `--config PATH` to load another file. CLI flags override the config file.

## Run

Example (credentials from config or CLI):

```bash
./protect-ostack --keystone-url https://keystone.example.com:5000/v3 --project myproject --user myuser --password mypass
```

Or set `keystone_url`, `project`, `user`, `password` in `cfg/config.yaml` and run:

```bash
./protect-ostack
```

Optional flags: `--config`, `--region`, `--domain`, `--backup-dir`, `--disk-format`, `--max-parallel-snap N`, `--max-parallel-vol N`, `--discover-all` / `--vm-list`, `--vm-filter`, `--vm-tags`. See [scripts/bash/README.md](../scripts/bash/README.md) for full documentation (features, options, backup layout, troubleshooting).

## Requirements

- Go 1.22+
- [Gophercloud v2](https://github.com/gophercloud/gophercloud) (go mod handles it)
