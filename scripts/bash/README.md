# OpenStack VM and Volume Backup Script

A bash script for backing up OpenStack VMs and their attached Cinder volumes. The script automatically discovers service endpoints from Keystone, discovers VMs, saves VM configuration (including tags and metadata), and creates backups of all attached volumes.

## Features

- **Automatic Endpoint Discovery**: Discovers Cinder, Nova, and Glance endpoints from Keystone service catalog when not provided
- **VM Discovery**: Automatically discovers all VMs or uses a manual list
- **VM Configuration Backup**: Saves complete VM configuration, OpenStack tags, and metadata as JSON files
- **Volume Backup**: Creates snapshots, temporary volumes, and images to download disk backups (default: QCOW2)
- **Automatic Cleanup**: Cleans up temporary resources (snapshots, volumes, images) after backup
- **Error Handling**: Timeouts, status checks, and validation
- **Filtering**: Filter VMs by name pattern or by OpenStack tags/metadata
- **OpenStack Compatibility**: Validates VMs and only processes backup-supported states (ACTIVE, SHUTOFF, PAUSED, SUSPENDED)
- **Pagination**: Handles large VM lists

## Requirements

- Bash 4.0+
- `curl`
- `jq`
- OpenStack credentials with access to Keystone, Nova, Cinder, and Glance

## Installation

```bash
chmod +x protect-ostack.sh
```

Install dependencies:

- **Ubuntu/Debian:** `sudo apt-get install curl jq`
- **RHEL/CentOS:** `sudo yum install curl jq`
- **macOS:** `brew install curl jq`

## Usage

### Basic (endpoints auto-discovered from Keystone)

```bash
./protect-ostack.sh \
  --keystone-url https://keystone.example.com:5000/v3 \
  --project myproject \
  --user myuser \
  --password mypass
```

### With VM name filter

```bash
./protect-ostack.sh \
  --keystone-url https://keystone.example.com:5000/v3 \
  --project myproject \
  --user myuser \
  --password mypass \
  --vm-filter "prod-*"
```

### With VM tags or metadata filter

```bash
# Single tag/metadata
./protect-ostack.sh ... --vm-tags backup:true

# Multiple (all must match)
./protect-ostack.sh ... --vm-tags env:prod,team:ops
```

### Manual VM list

```bash
./protect-ostack.sh ... --vm-list vm1 vm2 vm3
```

### Custom backup dir and disk format

```bash
./protect-ostack.sh ... --backup-dir /mnt/backups/openstack --disk-format raw
```

## Command-Line Options

| Option | Description |
|--------|-------------|
| `--keystone-url URL` | **(Required)** Keystone endpoint |
| `--project NAME` | **(Required)** OpenStack project |
| `--user NAME` | **(Required)** OpenStack user |
| `--password PASSWORD` | **(Required)** OpenStack password |
| `--cinder-url URL` | Cinder endpoint (auto-discovered if omitted) |
| `--nova-url URL` | Nova endpoint (auto-discovered if omitted) |
| `--glance-url URL` | Glance endpoint (auto-discovered if omitted) |
| `--domain NAME` | Domain (default: Default) |
| `--backup-dir DIR` | Backup directory (default: /backup/openstack) |
| `--disk-format FORMAT` | qcow2 (default), raw, vmdk, vdi |
| `--discover-all` | Discover all VMs (default) |
| `--no-discover-all` | Use manual VM list |
| `--vm-filter PATTERN` | Filter VMs by name (e.g. prod-*) |
| `--vm-tags KEY:VALUE,...` | Filter by tags/metadata (all must match) |
| `--vm-list VM1 VM2 ...` | Manual VM list |
| `--help`, `-h` | Show usage |

## How It Works

### Backup flow diagram

**[diagram.drawio](../../diagrams/diagram.drawio)** — Backup flow diagram (edit with [draw.io](https://app.diagrams.net/) or VS Code Draw.io extension)

### Steps

1. **Authentication** — Authenticate with Keystone and get the service catalog.
2. **Endpoint discovery** — Use catalog to get Cinder, Nova, Glance URLs if not provided.
3. **VM discovery** — List all VMs (or use `--vm-list`), filter by status (only ACTIVE, SHUTOFF, PAUSED, SUSPENDED), name pattern, and tags/metadata. Validate OpenStack server structure.
4. **VM config backup** — For each VM: save full server details to `vm-config.json`, tags to `vm-tags.json`, metadata to `vm-metadata.json`.
5. **Volume backup** — For each attached volume: create snapshot → create temp volume → create Glance image → download image file → delete temp image, volume, snapshot.
6. **Layout** — Backups under `BACKUP_DIR/VM_NAME/YYYY-MM-DD_HH-MM/`.

## Backup Directory Structure

```
/backup/openstack/
├── vm1/
│   └── 2026-01-27_14-30/
│       ├── vm-config.json
│       ├── vm-tags.json
│       ├── vm-metadata.json
│       ├── volume-uuid-1.qcow2
│       └── volume-uuid-2.qcow2
├── vm2/
│   └── 2026-01-27_14-30/
│       ├── vm-config.json
│       ├── vm-tags.json
│       ├── vm-metadata.json
│       └── volume-uuid-3.qcow2
└── ...
```

- **vm-config.json** — Full VM details (flavor, networks, security groups, etc.).
- **vm-tags.json** — OpenStack server tags (`/servers/{id}/tags`).
- **vm-metadata.json** — VM metadata (`/servers/{id}/metadata`).
- **volume-*.qcow2** (or raw/vmdk/vdi) — Volume disk images.

## Configuration (script header)

- `STATUS_TIMEOUT=1800` — Max wait for resource status (seconds).
- `STATUS_INTERVAL=5` — Poll interval (seconds).

## Troubleshooting

- **Auth failed** — Check Keystone URL, user, password, project, domain.
- **Endpoint discovery failed** — Ensure catalog has Cinder/Nova/Glance; or pass `--cinder-url`, `--nova-url`, `--glance-url`.
- **VM discovery** — User needs list servers; only ACTIVE/SHUTOFF/PAUSED/SUSPENDED are backed up.
- **Tag/metadata filter** — Uses `/servers/{id}/tags` and `/servers/{id}/metadata`; tags are name-only, metadata is key:value.
- **Volume backup** — Check quota for snapshots/volumes; ensure backup dir has space and write access.

## Security

- Store credentials securely (e.g. env vars or secret manager).
- Use a dedicated backup user with minimal required roles.
- Prefer HTTPS for all endpoints.
- Restrict permissions on the backup directory.

## Limitations

- Backs up only volumes attached to VMs (not unattached volumes).
- VM config/tags/metadata are for reference; no built-in restore.
- Temporary resources require quota during backup.
- No incremental backup or checksum verification.

## License

This project is licensed under the MIT License — see [LICENSE](../../LICENSE).
