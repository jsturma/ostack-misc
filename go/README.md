# protect-ostack (Go)

Go port of the [bash backup script](../scripts/bash/protect-ostack.sh), using [Gophercloud v2](https://github.com/gophercloud/gophercloud) for OpenStack API access.

OpenStack logic lives in the **ostack** library (`tools/ostack/`): Keystone v3 auth (project-scoped token), Nova (VMs, tags, metadata), Cinder (volumes, snapshots), Glance (images), and backup orchestration. Service endpoints are discovered from the Keystone catalog using the configured **region** (default: `RegionOne`). **Backups run in parallel**: all VMs are backed up concurrently, and within each VM all volume backups run concurrently. `main.go` handles CLI flags and calls into the lib.

## Build

```bash
go build -o protect-ostack .
```

## Run

Same options as the bash script. Example:

```bash
./protect-ostack \
  --keystone-url https://keystone.example.com:5000/v3 \
  --project myproject \
  --user myuser \
  --password mypass
```

Optional: `--region RegionOne` (default), `--domain Default`, `--backup-dir`, `--disk-format`, `--discover-all` / `--vm-list`, `--vm-filter`, `--vm-tags`. See [scripts/bash/README.md](../scripts/bash/README.md) for full documentation (features, options, backup layout, troubleshooting).

## Requirements

- Go 1.22+
- [Gophercloud v2](https://github.com/gophercloud/gophercloud) (go mod handles it)
