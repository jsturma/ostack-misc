# OpenStack VM and Volume Backup Script

A bash script for backing up OpenStack VMs and their attached Cinder volumes. The script automatically discovers service endpoints from Keystone, discovers VMs, saves VM configuration, and creates backups of all attached volumes.

## Features

- **Automatic Endpoint Discovery**: Automatically discovers Cinder, Nova, and Glance endpoints from Keystone service catalog
- **VM Discovery**: Automatically discovers all VMs or uses a manual list
- **VM Configuration Backup**: Saves complete VM configuration (metadata, flavor, networks, security groups, etc.) as JSON
- **Volume Backup**: Creates snapshots, temporary volumes, and images to download RAW backups
- **Automatic Cleanup**: Cleans up temporary resources (snapshots, volumes, images) after backup
- **Error Handling**: Comprehensive error handling with timeouts and status checks
- **Flexible Filtering**: Filter VMs by name pattern or metadata tags
- **OpenStack Compatibility**: Validates VMs are valid OpenStack servers and only processes backup-supported VM states
- **Pagination Support**: Handles large VM lists with pagination

## Requirements

- Bash 4.0+
- `curl` - For API calls
- `jq` - For JSON parsing
- OpenStack credentials with access to:
  - Keystone (authentication)
  - Nova (VM discovery)
  - Cinder (volume management)
  - Glance (image management)

## Installation

1. Download the script:
```bash
wget https://raw.githubusercontent.com/your-repo/protect-ostack.sh
chmod +x protect-ostack.sh
```

2. Install dependencies:

**On Ubuntu/Debian:**
```bash
sudo apt-get update
sudo apt-get install curl jq
```

**On RHEL/CentOS:**
```bash
sudo yum install curl jq
```

**On macOS:**
```bash
brew install curl jq
```

## Usage

### Basic Usage (Minimal - Auto-discovery)

The script only requires the Keystone URL and credentials. All other service endpoints are automatically discovered:

```bash
./protect-ostack.sh \
  --keystone-url https://keystone.example.com:5000/v3 \
  --project myproject \
  --user myuser \
  --password mypass
```

### With Manual Endpoints

If you need to specify endpoints manually (e.g., for internal endpoints):

```bash
./protect-ostack.sh \
  --keystone-url https://keystone.internal:5000/v3 \
  --cinder-url https://cinder.internal:8776/v3 \
  --nova-url https://nova.internal:8774/v2.1 \
  --glance-url https://glance.internal:9292/v2/images \
  --project backup \
  --user backup-user \
  --password secret
```

### With VM Filtering

Backup only VMs matching a name pattern:

```bash
./protect-ostack.sh \
  --keystone-url https://keystone.example.com:5000/v3 \
  --project myproject \
  --user myuser \
  --password mypass \
  --vm-filter "prod-*"
```

### With VM Tag Filtering

Backup only VMs that have specific tags or metadata. The script uses OpenStack's `/servers/{server_id}/tags` and `/servers/{server_id}/metadata` endpoints:

**OpenStack Tags** (from `/servers/{server_id}/tags`):
- Tags are just names (no values)
- Format: `tagname:` or `tagname` (value part is ignored)
- Example: `--vm-tags backup:` or `--vm-tags production:`

**Metadata** (from `/servers/{server_id}/metadata`):
- Metadata are key:value pairs
- Format: `key:value`
- Example: `--vm-tags backup:true` or `--vm-tags env:prod`

```bash
# Using OpenStack tags (tag name only)
./protect-ostack.sh \
  --keystone-url https://keystone.example.com:5000/v3 \
  --project myproject \
  --user myuser \
  --password mypass \
  --vm-tags backup:

# Using metadata (key:value pairs)
./protect-ostack.sh \
  --keystone-url https://keystone.example.com:5000/v3 \
  --project myproject \
  --user myuser \
  --password mypass \
  --vm-tags backup:true

# Multiple filters: backup VMs with both env=prod AND team=ops (all must match)
./protect-ostack.sh \
  --keystone-url https://keystone.example.com:5000/v3 \
  --project myproject \
  --user myuser \
  --password mypass \
  --vm-tags env:prod,team:ops

# Combine with name filter
./protect-ostack.sh \
  --keystone-url https://keystone.example.com:5000/v3 \
  --project myproject \
  --user myuser \
  --password mypass \
  --vm-filter "prod-*" \
  --vm-tags backup:enabled
```

**Note**: The script checks both tags and metadata endpoints. For tags, only the tag name is checked (value is ignored). For metadata, both key and value must match. Tag filtering may be slower for large deployments - use name filtering first to reduce the number of VMs checked.

### Manual VM List

Specify VMs manually instead of auto-discovery:

```bash
./protect-ostack.sh \
  --keystone-url https://keystone.example.com:5000/v3 \
  --project myproject \
  --user myuser \
  --password mypass \
  --vm-list vm1 vm2 vm3
```

### Custom Backup Directory

Specify a custom backup directory:

```bash
./protect-ostack.sh \
  --keystone-url https://keystone.example.com:5000/v3 \
  --project myproject \
  --user myuser \
  --password mypass \
  --backup-dir /mnt/backups/openstack
```

### Custom Disk Format

Specify a different disk image format (default is `qcow2`):

```bash
# Use RAW format
./protect-ostack.sh \
  --keystone-url https://keystone.example.com:5000/v3 \
  --project myproject \
  --user myuser \
  --password mypass \
  --disk-format raw

# Use VMDK format (for VMware compatibility)
./protect-ostack.sh \
  --keystone-url https://keystone.example.com:5000/v3 \
  --project myproject \
  --user myuser \
  --password mypass \
  --disk-format vmdk
```

Supported formats: `qcow2` (default), `raw`, `vmdk`, `vdi`

## Command-Line Options

### Required Parameters

- `--keystone-url URL` - Keystone authentication endpoint (e.g., `https://keystone.example.com:5000/v3`)
- `--project NAME` - OpenStack project name
- `--user NAME` - OpenStack username
- `--password PASSWORD` - OpenStack password

### Optional Parameters

- `--cinder-url URL` - Cinder service endpoint (auto-discovered if omitted)
- `--nova-url URL` - Nova service endpoint (auto-discovered if omitted)
- `--glance-url URL` - Glance service endpoint (auto-discovered if omitted)
- `--domain NAME` - OpenStack domain name (default: `Default`)
- `--backup-dir DIR` - Backup directory (default: `/backup/openstack`)
- `--disk-format FORMAT` - Disk image format: `qcow2` (default), `raw`, `vmdk`, or `vdi`
- `--discover-all` - Discover all VMs automatically (default: `true`)
- `--no-discover-all` - Disable automatic VM discovery
- `--vm-filter PATTERN` - Filter VMs by name pattern (e.g., `"prod-*"`, `"web-*"`)
- `--vm-tags KEY:VALUE[,KEY2:VALUE2]` - Filter VMs by OpenStack tags or metadata (all must match)
  - For tags: use `tagname:` (checks `/servers/{id}/tags` endpoint)
  - For metadata: use `key:value` (checks `/servers/{id}/metadata` endpoint)
- `--vm-list VM1 VM2 ...` - Manual VM list (disables auto-discovery)
- `--help` or `-h` - Show usage information

## How It Works

1. **Authentication**: Authenticates with Keystone and retrieves the service catalog
2. **Endpoint Discovery**: Extracts Cinder, Nova, and Glance endpoints from the service catalog
3. **VM Discovery**: Discovers all VMs (or uses manual list) and filters by:
   - **OpenStack Validation**: Validates that VMs are valid OpenStack servers with required fields (id, name, status)
   - **Status Filtering**: Only processes VMs in backup-supported states:
     - `ACTIVE` - Running VMs
     - `SHUTOFF` - Stopped VMs
     - `PAUSED` - Paused VMs
     - `SUSPENDED` - Suspended VMs
   - Skips VMs in unsupported states: `ERROR`, `DELETED`, `BUILDING`, `MIGRATING`, etc.
   - Name pattern (if `--vm-filter` is specified)
   - Metadata tags (if `--vm-tags` is specified, all tags must match)
4. **VM Configuration Backup** (for each VM):
   - Retrieves complete VM details from Nova API
   - Saves VM configuration as JSON file (`vm-config.json`)
   - Explicitly fetches and saves OpenStack tags from `/servers/{id}/tags` to `vm-tags.json`
   - Explicitly fetches and saves metadata from `/servers/{id}/metadata` to `vm-metadata.json`
   - Includes: flavor, networks, security groups, key pairs, image info, etc.
5. **Volume Backup Process** (for each volume):
   - Creates a snapshot of the volume
   - Creates a temporary volume from the snapshot
   - Creates a Glance image from the temporary volume (in specified format, default: QCOW2)
   - Downloads the image file to the backup directory
   - Cleans up temporary resources (snapshot, volume, image)
6. **Organization**: Backups are organized by VM name and timestamp: `BACKUP_DIR/VM_NAME/YYYY-MM-DD_HH-MM/`

## Backup Directory Structure

```
/backup/openstack/
├── vm1/
│   └── 2026-01-27_14-30/
│       ├── vm-config.json          # Complete VM configuration
│       ├── vm-tags.json            # OpenStack server tags
│       ├── vm-metadata.json        # VM metadata (key:value pairs)
│       ├── volume-uuid-1.qcow2     # Volume backups (default: QCOW2 format)
│       └── volume-uuid-2.qcow2
├── vm2/
│   └── 2026-01-27_14-30/
│       ├── vm-config.json
│       ├── vm-tags.json
│       ├── vm-metadata.json
│       └── volume-uuid-3.qcow2
└── vm3/
    └── 2026-01-27_14-35/
        ├── vm-config.json
        ├── vm-tags.json
        ├── vm-metadata.json
        └── volume-uuid-4.qcow2
```

### VM Configuration Files

**`vm-config.json`** - Complete VM configuration including:
- **Basic Info**: Name, ID, status, creation date
- **Flavor**: CPU, RAM, disk specifications
- **Networks**: Network interfaces, IP addresses, MAC addresses
- **Security Groups**: All associated security groups
- **Key Pairs**: SSH key pair information
- **Image**: Source image details
- **Availability Zone**: VM placement information

**`vm-tags.json`** - OpenStack server tags:
- Contains all tags associated with the VM
- Format: `{"tags": ["tag1", "tag2", ...]}`
- Retrieved from `/servers/{server_id}/tags` endpoint
- Empty tags array if no tags exist

**`vm-metadata.json`** - VM metadata (key:value pairs):
- Contains all metadata associated with the VM
- Format: `{"metadata": {"key1": "value1", "key2": "value2", ...}}`
- Retrieved from `/servers/{server_id}/metadata` endpoint
- Empty metadata object if no metadata exists

## Configuration

### Timeouts

The script includes configurable timeouts (in the script header):

- `STATUS_TIMEOUT=1800` - Maximum wait time for resource status changes (30 minutes)
- `STATUS_INTERVAL=5` - Interval between status checks (5 seconds)

### Proxy Support

If you need to use a proxy, uncomment and configure in the script:

```bash
export HTTP_PROXY="http://proxy.local:3128"
export HTTPS_PROXY="http://proxy.local:3128"
export NO_PROXY="127.0.0.1,localhost,10.0.0.0/8,.internal"
```

## Examples

### Production Backup with Filtering

```bash
./protect-ostack.sh \
  --keystone-url https://keystone.prod.example.com:5000/v3 \
  --project production \
  --user backup-service \
  --password $(cat /etc/backup/password) \
  --vm-filter "prod-*" \
  --backup-dir /mnt/backups/production
```

### Scheduled Backup (Cron)

Add to crontab for daily backups at 2 AM:

```bash
0 2 * * * /path/to/protect-ostack.sh --keystone-url https://keystone.example.com:5000/v3 --project backup --user backup-user --password "secret" --backup-dir /backup/openstack >> /var/log/ostack-backup.log 2>&1
```

### Multiple Projects

Create a wrapper script for multiple projects:

```bash
#!/bin/bash
for project in project1 project2 project3; do
  ./protect-ostack.sh \
    --keystone-url https://keystone.example.com:5000/v3 \
    --project "$project" \
    --user backup-user \
    --password secret \
    --backup-dir "/backup/openstack/$project"
done
```

## Troubleshooting

### Authentication Failures

- Verify Keystone URL is correct and accessible
- Check username, password, and project name
- Verify domain name if using non-default domain
- Check network connectivity to Keystone endpoint

### Endpoint Discovery Failures

- Ensure the service catalog contains the required services
- Check that the user has access to the project
- Verify service names in the catalog (cinder/volumev3, compute/nova, image/glance)
- Try specifying endpoints manually with `--cinder-url`, `--nova-url`, `--glance-url`

### VM Discovery Issues

- Verify the user has permissions to list servers in Nova
- Check if `all_tenants=1` parameter is required (script includes it)
- **VM Status**: Only VMs in `ACTIVE`, `SHUTOFF`, `PAUSED`, or `SUSPENDED` states are backed up. VMs in `ERROR`, `DELETED`, `BUILDING`, `MIGRATING`, or other states are automatically skipped
- **OpenStack Validation**: The script validates that discovered VMs are valid OpenStack servers. If a VM doesn't have required OpenStack fields (id, name, status), it will be skipped with a warning
- **Tag Filtering Performance**: Tag filtering uses dedicated OpenStack endpoints (`/servers/{id}/tags` and `/servers/{id}/metadata`) which is more efficient than fetching full VM details. However, it still requires an API call per VM, so consider using `--vm-filter` first to reduce the number of VMs checked
- **Tag Filtering**: 
  - For OpenStack tags: Only the tag name is checked (value part after `:` is ignored). Tags are case-sensitive
  - For metadata: Both key and value must match exactly (case-sensitive). Example: `backup:true` vs `backup:True` are different
  - The script checks both endpoints - if a tag exists in either location, it will match

### VM Configuration Backup Issues

- Verify the user has permissions to read server details in Nova
- Check that the VM ID is valid and accessible
- Ensure Nova API is responding correctly
- If VM config backup fails, the script will log a warning but continue with volume backups

### Volume Backup Failures

- Verify sufficient quota for creating snapshots and temporary volumes
- Check Cinder and Glance service availability
- Ensure the backup directory has sufficient disk space
- Check network connectivity for downloading images

### Timeout Issues

- Increase `STATUS_TIMEOUT` for slow environments
- Check OpenStack service performance
- Verify network latency to OpenStack endpoints

## Error Messages

- `Missing required parameter: --keystone-url` - Keystone URL not provided
- `Failed to obtain token` - Authentication failed
- `Failed to discover Cinder/Nova/Glance` - Service endpoint not found in catalog
- `Invalid disk format: ...` - Unsupported disk format specified (use: qcow2, raw, vmdk, vdi)
- `Timeout waiting for...` - Resource status change took too long
- `Failed to create snapshot/volume/image` - API call failed

## Security Considerations

- **Credentials**: Store passwords securely (use environment variables or secret management)
- **Permissions**: Use a dedicated backup user with minimal required permissions
- **Network**: Use HTTPS for all endpoints
- **Backup Storage**: Ensure backup directory has appropriate permissions (e.g., `chmod 700`)

## Limitations

- Only backs up volumes attached to VMs (not unattached volumes)
- VM configuration is saved as JSON (read-only backup, not directly restorable)
- Creates temporary resources during backup (requires sufficient quota)
- Default format is QCOW2 (can be changed with `--disk-format`)
- Supported formats: qcow2, raw, vmdk, vdi
- No incremental backup support
- No backup verification/checksum validation

## Contributing

Contributions are welcome! Please ensure:
- Code follows the existing style
- Script size stays under 20KB
- All functionality is tested
- Error handling is comprehensive

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.

## Support

For issues and questions:
- Open an issue on GitHub
- Check the troubleshooting section
- Review OpenStack documentation for API details
