#!/usr/bin/env bash
set -euo pipefail

# Configuration
DISCOVER_ALL_VMS=true
VM_NAME_FILTER=""
VM_TAG_FILTER=""
VM_LIST=("vm1" "vm2")
KEYSTONE_URL=""
CINDER_URL=""
NOVA_URL=""
GLANCE_URL=""
PROJECT=""
USER=""
PASSWORD=""
DOMAIN="Default"
BACKUP_DIR="/backup/openstack"
DISK_FORMAT="qcow2"
STATUS_TIMEOUT=1800
STATUS_INTERVAL=5

# Functions
log() { echo "[$(date +%F_%T)] $*" >&2; }
error() { log "ERROR: $*"; exit 1; }

usage() {
  cat <<EOF
Usage: $0 [OPTIONS]

Required: --keystone-url URL --project NAME --user NAME --password PASSWORD
Optional: [--cinder-url URL] [--nova-url URL] [--glance-url URL] [--domain NAME]
         [--backup-dir DIR] [--disk-format FORMAT] [--discover-all] [--vm-filter PATTERN] [--vm-tags KEY:VALUE] [--vm-list VM1 VM2 ...]
         [--help]
         Format options: qcow2 (default), raw, vmdk, vdi
         Tag filter: --vm-tags backup:true or --vm-tags env:prod,team:ops

Examples:
  $0 --keystone-url https://keystone.example.com:5000/v3 --project myproject --user myuser --password mypass
  $0 --keystone-url https://keystone.internal:5000/v3 --project backup --user backup-user --password secret --vm-filter "prod-*"
EOF
  exit 1
}

check_dependencies() {
  for dep in curl jq; do
    command -v "$dep" &>/dev/null || error "Required tool '$dep' is not installed"
  done
}

parse_arguments() {
  while [[ $# -gt 0 ]]; do
    case $1 in
      --keystone-url) KEYSTONE_URL="$2"; shift 2 ;;
      --cinder-url) CINDER_URL="$2"; shift 2 ;;
      --nova-url) NOVA_URL="$2"; shift 2 ;;
      --glance-url) GLANCE_URL="$2"; shift 2 ;;
      --project) PROJECT="$2"; shift 2 ;;
      --user) USER="$2"; shift 2 ;;
      --password) PASSWORD="$2"; shift 2 ;;
      --domain) DOMAIN="$2"; shift 2 ;;
      --backup-dir) BACKUP_DIR="$2"; shift 2 ;;
      --disk-format) DISK_FORMAT="$2"; shift 2 ;;
      --discover-all) DISCOVER_ALL_VMS=true; shift ;;
      --no-discover-all) DISCOVER_ALL_VMS=false; shift ;;
      --vm-filter) VM_NAME_FILTER="$2"; shift 2 ;;
      --vm-tags) VM_TAG_FILTER="$2"; shift 2 ;;
      --vm-list)
        shift; VM_LIST=()
        while [[ $# -gt 0 ]] && [[ ! "$1" =~ ^-- ]]; do VM_LIST+=("$1"); shift; done
        DISCOVER_ALL_VMS=false ;;
      --help|-h) usage ;;
      *) error "Unknown option: $1. Use --help for usage." ;;
    esac
  done
  
  [[ -z "$KEYSTONE_URL" ]] && error "Missing: --keystone-url"
  [[ -z "$PROJECT" ]] && error "Missing: --project"
  [[ -z "$USER" ]] && error "Missing: --user"
  [[ -z "$PASSWORD" ]] && error "Missing: --password"
  
  [[ -n "$CINDER_URL" ]] && [[ ! "$CINDER_URL" =~ /$PROJECT ]] && CINDER_URL="$CINDER_URL/$PROJECT"
  [[ -n "$NOVA_URL" ]] && [[ ! "$NOVA_URL" =~ /$PROJECT ]] && NOVA_URL="$NOVA_URL/$PROJECT"
  [[ -n "$GLANCE_URL" ]] && [[ ! "$GLANCE_URL" =~ /images ]] && GLANCE_URL="$GLANCE_URL/images"
  [[ ! "$DISK_FORMAT" =~ ^(qcow2|raw|vmdk|vdi)$ ]] && error "Invalid disk format: $DISK_FORMAT (supported: qcow2, raw, vmdk, vdi)"
}

api_call() {
  local method=$1 url=$2; shift 2
  local response http_code
  response=$(curl -s -w "\n%{http_code}" -X "$method" "$url" -H "Content-Type: application/json" -H "X-Auth-Token: $TOKEN" "$@")
  http_code=$(echo "$response" | tail -n1)
  response=$(echo "$response" | sed '$d')
  [[ "$http_code" -ge 400 ]] && error "API failed: $method $url (HTTP $http_code)"
  echo "$response"
}

get_token_and_catalog() {
  local catalog_file=$1 response token catalog_json
  response=$(curl -s -i -X POST "$KEYSTONE_URL/auth/tokens" -H "Content-Type: application/json" \
    -d "{\"auth\":{\"identity\":{\"methods\":[\"password\"],\"password\":{\"user\":{\"name\":\"$USER\",\"domain\":{\"name\":\"$DOMAIN\"},\"password\":\"$PASSWORD\"}}},\"scope\":{\"project\":{\"name\":\"$PROJECT\",\"domain\":{\"name\":\"$DOMAIN\"}}}}}")
  echo "$response" | grep -qi "X-Subject-Token" || error "Failed to obtain token"
  token=$(echo "$response" | grep -Fi "X-Subject-Token" | awk '{print $2}' | tr -d $'\r')
  [[ -z "$token" ]] && error "Token is empty"
  catalog_json=$(echo "$response" | awk '/^$/{p=1;next} p')
  [[ -z "$catalog_json" ]] && error "Failed to extract catalog"
  [[ -n "$catalog_file" ]] && echo "$catalog_json" > "$catalog_file"
  echo "$token"
}

discover_endpoint() {
  local catalog_json=$1 service=$2
  echo "$catalog_json" | jq -r "(.token.catalog[]?//.token.serviceCatalog[]?//.catalog[]?//.serviceCatalog[]?)|select(.name==\"$service\" or .type==\"$service\" or .type|contains(\"$service\"))|.endpoints[]?|select(.interface==\"public\")|.url//empty" | head -n1 || \
  echo "$catalog_json" | jq -r "(.token.catalog[]?//.token.serviceCatalog[]?//.catalog[]?//.serviceCatalog[]?)|select(.name==\"$service\" or .type==\"$service\" or .type|contains(\"$service\"))|.endpoints[0]?.url//empty" | head -n1
}

discover_service_endpoints() {
  local catalog_json=$1 base
  log "Discovering endpoints from Keystone catalog..."
  if [[ -z "$CINDER_URL" ]]; then
    base=$(discover_endpoint "$catalog_json" "cinder" || discover_endpoint "$catalog_json" "volumev3")
    [[ -n "$base" && "$base" != "null" ]] && CINDER_URL="$base/$PROJECT" || error "Failed to discover Cinder"
    log "  ✓ Cinder: $CINDER_URL"
  fi
  if [[ -z "$NOVA_URL" ]]; then
    base=$(discover_endpoint "$catalog_json" "compute" || discover_endpoint "$catalog_json" "nova")
    if [[ -n "$base" && "$base" != "null" ]]; then
      [[ "$base" =~ /v2\.?1?/?$ ]] && NOVA_URL="$base/$PROJECT" || NOVA_URL="$base/v2.1/$PROJECT"
      log "  ✓ Nova: $NOVA_URL"
    else
      error "Failed to discover Nova"
    fi
  fi
  if [[ -z "$GLANCE_URL" ]]; then
    base=$(discover_endpoint "$catalog_json" "image" || discover_endpoint "$catalog_json" "glance")
    if [[ -n "$base" && "$base" != "null" ]]; then
      [[ "$base" =~ /v2/?$ ]] && GLANCE_URL="$base/images" || GLANCE_URL="$base/v2/images"
      log "  ✓ Glance: $GLANCE_URL"
    else
      error "Failed to discover Glance"
    fi
  fi
}

wait_status() {
  local type=$1 id=$2 status=$3 url="" current elapsed=0
  case $type in
    volume) url="$CINDER_URL/volumes/$id" ;;
    snapshot) url="$CINDER_URL/snapshots/$id" ;;
    image) url="$GLANCE_URL/$id" ;;
    *) error "Unknown type: $type" ;;
  esac
  log "Waiting for $type $id: $status"
  while true; do
    current=$(curl -s -H "X-Auth-Token: $TOKEN" "$url" | jq -r ".${type}.status//empty")
    [[ -z "$current" ]] && error "Failed to get status for $type $id"
    [[ "$current" == "$status" ]] && log "$type $id is $status" && return 0
    [[ "$current" == "error" ]] && error "$type $id entered error state"
    [[ $elapsed -ge $STATUS_TIMEOUT ]] && error "Timeout: $type $id (current: $current)"
    sleep $STATUS_INTERVAL
    elapsed=$((elapsed + STATUS_INTERVAL))
    [[ $((elapsed % 30)) -eq 0 ]] && log "Waiting... (${elapsed}s, status: $current)"
  done
}

cleanup_resource() {
  local type=$1 id=$2 url=""
  case $type in
    image) url="$GLANCE_URL/$id" ;;
    volume) url="$CINDER_URL/volumes/$id" ;;
    snapshot) url="$CINDER_URL/snapshots/$id" ;;
    *) return ;;
  esac
  log "Cleaning up $type: $id"
  curl -s -f -X DELETE "$url" -H "X-Auth-Token: $TOKEN" >/dev/null 2>&1 || log "Warning: Failed to delete $type: $id"
}

validate_openstack_vm() {
  local vm_id=$1 vm_data
  [[ -z "$vm_id" ]] && return 1
  vm_data=$(curl -s -H "X-Auth-Token: $TOKEN" "$NOVA_URL/servers/$vm_id")
  [[ -z "$vm_data" ]] && return 1
  echo "$vm_data" | jq -e '.server.id' >/dev/null 2>&1 || return 1
  echo "$vm_data" | jq -e '.server.name' >/dev/null 2>&1 || return 1
  echo "$vm_data" | jq -e '.server.status' >/dev/null 2>&1 || return 1
  return 0
}

is_vm_backup_supported() {
  local vm_status=$1
  case "$vm_status" in
    ACTIVE|SHUTOFF|PAUSED|SUSPENDED) return 0 ;;
    *) return 1 ;;
  esac
}

get_vm_id() {
  local vm_id vm_data
  vm_data=$(curl -s -H "X-Auth-Token: $TOKEN" "$NOVA_URL/servers?name=$1")
  vm_id=$(echo "$vm_data" | jq -r ".servers[0].id//empty")
  [[ -z "$vm_id" || "$vm_id" == "null" ]] && return 1
  validate_openstack_vm "$vm_id" || return 1
  echo "$vm_id"
}

get_attached_volumes() {
  curl -s -H "X-Auth-Token: $TOKEN" "$CINDER_URL/volumes?all_tenants=1" | jq -r ".volumes[]|select(.attachments[]?.server_id==\"$1\")|.id"
}

check_vm_tags() {
  local vm_id=$1 vm_tags vm_metadata tag_key expected_value actual_value found
  [[ -z "$VM_TAG_FILTER" ]] && return 0
  
  # Try OpenStack server tags endpoint (/servers/{server_id}/tags)
  vm_tags=$(curl -s -f -H "X-Auth-Token: $TOKEN" "$NOVA_URL/servers/$vm_id/tags" 2>/dev/null | jq -r '.tags[]?//empty' 2>/dev/null)
  
  # Try metadata endpoint (/servers/{server_id}/metadata)
  vm_metadata=$(curl -s -f -H "X-Auth-Token: $TOKEN" "$NOVA_URL/servers/$vm_id/metadata" 2>/dev/null | jq -r '.metadata//{}' 2>/dev/null)
  
  IFS=',' read -ra TAGS <<< "$VM_TAG_FILTER"
  for tag_pair in "${TAGS[@]}"; do
    IFS=':' read -r tag_key expected_value <<< "$tag_pair"
    found=0
    
    # Check OpenStack tags (tags are just names, no values)
    if [[ -n "$vm_tags" ]]; then
      while IFS= read -r tag; do
        [[ -n "$tag" && "$tag" == "$tag_key" ]] && found=1 && break
      done <<< "$vm_tags"
    fi
    
    # Check metadata (key:value pairs)
    if [[ $found -eq 0 ]] && [[ -n "$vm_metadata" && "$vm_metadata" != "{}" ]]; then
      actual_value=$(echo "$vm_metadata" | jq -r ".[\"$tag_key\"]//empty")
      [[ -n "$actual_value" && "$actual_value" == "$expected_value" ]] && found=1
    fi
    
    [[ $found -eq 0 ]] && return 1
  done
  return 0
}

discover_all_vms() {
  local response next_link vm_name vm_id vm_status
  log "Discovering VMs from OpenStack..."
  response=$(curl -s -H "X-Auth-Token: $TOKEN" "$NOVA_URL/servers?all_tenants=1&limit=1000")
  echo "$response" | jq -e '.servers' >/dev/null 2>&1 || error "Failed to retrieve VM list"
  
  while IFS= read -r vm_data; do
    vm_name=$(echo "$vm_data" | jq -r '.name//empty')
    vm_id=$(echo "$vm_data" | jq -r '.id//empty')
    vm_status=$(echo "$vm_data" | jq -r '.status//empty')
    [[ -z "$vm_name" || "$vm_name" == "null" || -z "$vm_id" || "$vm_id" == "null" ]] && continue
    [[ "$vm_status" == "ERROR" || "$vm_status" == "DELETED" ]] && log "Skipping $vm_name ($vm_status)" && continue
    is_vm_backup_supported "$vm_status" || log "Skipping $vm_name (unsupported status: $vm_status)" && continue
    [[ -n "$VM_NAME_FILTER" ]] && case "$vm_name" in $VM_NAME_FILTER) ;; *) continue ;; esac
    [[ -n "$VM_TAG_FILTER" ]] && check_vm_tags "$vm_id" || continue
    validate_openstack_vm "$vm_id" || log "Skipping $vm_name (invalid OpenStack VM structure)" && continue
    echo "$vm_name|$vm_id"
  done < <(echo "$response" | jq -c '.servers[]?')
  
  next_link=$(echo "$response" | jq -r '.servers_links[]?|select(.rel=="next")|.href//empty')
  while [[ -n "$next_link" && "$next_link" != "null" ]]; do
    log "Fetching next page..."
    response=$(curl -s -H "X-Auth-Token: $TOKEN" "$next_link")
    while IFS= read -r vm_data; do
      vm_name=$(echo "$vm_data" | jq -r '.name//empty')
      vm_id=$(echo "$vm_data" | jq -r '.id//empty')
      vm_status=$(echo "$vm_data" | jq -r '.status//empty')
      [[ -z "$vm_name" || "$vm_name" == "null" || -z "$vm_id" || "$vm_id" == "null" ]] && continue
      [[ "$vm_status" == "ERROR" || "$vm_status" == "DELETED" ]] && continue
      is_vm_backup_supported "$vm_status" || continue
      [[ -n "$VM_NAME_FILTER" ]] && case "$vm_name" in $VM_NAME_FILTER) ;; *) continue ;; esac
      [[ -n "$VM_TAG_FILTER" ]] && check_vm_tags "$vm_id" || continue
      validate_openstack_vm "$vm_id" || continue
      echo "$vm_name|$vm_id"
    done < <(echo "$response" | jq -c '.servers[]?')
    next_link=$(echo "$response" | jq -r '.servers_links[]?|select(.rel=="next")|.href//empty')
  done
}

backup_vm_config() {
  local vm_id=$1 backup_dir=$2 response vm_tags vm_metadata
  log "Backing up VM configuration"
  response=$(curl -s -H "X-Auth-Token: $TOKEN" "$NOVA_URL/servers/$vm_id")
  echo "$response" | jq '.' > "$backup_dir/vm-config.json" || log "Warning: Failed to save VM config"
  
  log "Backing up VM tags"
  vm_tags=$(curl -s -f -H "X-Auth-Token: $TOKEN" "$NOVA_URL/servers/$vm_id/tags" 2>/dev/null)
  if [[ -n "$vm_tags" ]]; then
    echo "$vm_tags" | jq '.' > "$backup_dir/vm-tags.json" 2>/dev/null || log "Warning: Failed to save VM tags"
    log "VM tags saved to vm-tags.json"
  else
    echo '{"tags":[]}' > "$backup_dir/vm-tags.json"
    log "No tags found, saved empty tags file"
  fi
  
  log "Backing up VM metadata"
  vm_metadata=$(curl -s -f -H "X-Auth-Token: $TOKEN" "$NOVA_URL/servers/$vm_id/metadata" 2>/dev/null)
  if [[ -n "$vm_metadata" ]]; then
    echo "$vm_metadata" | jq '.' > "$backup_dir/vm-metadata.json" 2>/dev/null || log "Warning: Failed to save VM metadata"
    log "VM metadata saved to vm-metadata.json"
  else
    echo '{"metadata":{}}' > "$backup_dir/vm-metadata.json"
    log "No metadata found, saved empty metadata file"
  fi
  
  log "VM configuration, tags, and metadata saved"
}

backup_volume() {
  local vol_id=$1 backup_dir=$2 timestamp snap_id tmp_vol_id img_id vol_size response cleanup_list=()
  timestamp=$(date +%F_%H%M)
  log "Backing up volume $vol_id"
  
  log "Creating snapshot"
  response=$(api_call POST "$CINDER_URL/snapshots" -d "{\"snapshot\":{\"volume_id\":\"$vol_id\",\"name\":\"snap-${vol_id}-${timestamp}\",\"force\":true}}")
  snap_id=$(echo "$response" | jq -r ".snapshot.id//empty")
  [[ -z "$snap_id" || "$snap_id" == "null" ]] && error "Failed to create snapshot"
  cleanup_list+=("snapshot:$snap_id")
  wait_status snapshot "$snap_id" "available"
  
  log "Getting volume size"
  vol_size=$(curl -s -H "X-Auth-Token: $TOKEN" "$CINDER_URL/volumes/$vol_id" | jq -r ".volume.size//empty")
  [[ -z "$vol_size" || "$vol_size" == "null" ]] && error "Failed to get volume size"
  
  log "Creating temp volume (${vol_size}GB)"
  response=$(api_call POST "$CINDER_URL/volumes" -d "{\"volume\":{\"snapshot_id\":\"$snap_id\",\"size\":$vol_size,\"name\":\"tmp-${vol_id}-${timestamp}\"}}")
  tmp_vol_id=$(echo "$response" | jq -r ".volume.id//empty")
  [[ -z "$tmp_vol_id" || "$tmp_vol_id" == "null" ]] && error "Failed to create temp volume"
  cleanup_list+=("volume:$tmp_vol_id")
  wait_status volume "$tmp_vol_id" "available"
  
  log "Creating image ($DISK_FORMAT format)"
  response=$(api_call POST "$GLANCE_URL" -d "{\"name\":\"img-${vol_id}-${timestamp}\",\"disk_format\":\"$DISK_FORMAT\",\"container_format\":\"bare\",\"volume_id\":\"$tmp_vol_id\"}")
  img_id=$(echo "$response" | jq -r ".id//empty")
  [[ -z "$img_id" || "$img_id" == "null" ]] && error "Failed to create image"
  cleanup_list+=("image:$img_id")
  wait_status image "$img_id" "active"
  
  log "Downloading $DISK_FORMAT to $backup_dir/${vol_id}.${DISK_FORMAT}"
  curl -s -f -L -o "$backup_dir/${vol_id}.${DISK_FORMAT}" -H "X-Auth-Token: $TOKEN" "$GLANCE_URL/$img_id/file" || error "Download failed"
  [[ ! -f "$backup_dir/${vol_id}.${DISK_FORMAT}" || ! -s "$backup_dir/${vol_id}.${DISK_FORMAT}" ]] && error "File missing or empty"
  log "Downloaded $(du -h "$backup_dir/${vol_id}.${DISK_FORMAT}" | cut -f1)"
  
  log "Cleaning up"
  for resource in "${cleanup_list[@]}"; do
    cleanup_resource "${resource%%:*}" "${resource##*:}"
  done
  log "Volume $vol_id backed up"
}

# Main
check_dependencies
parse_arguments "$@"
mkdir -p "$BACKUP_DIR"
log "Starting backup - Keystone: $KEYSTONE_URL, Project: $PROJECT, User: $USER, Dir: $BACKUP_DIR"

catalog_file=$(mktemp)
TOKEN=$(get_token_and_catalog "$catalog_file")
catalog_json=$(cat "$catalog_file")
rm -f "$catalog_file"
[[ -z "$TOKEN" ]] && error "Failed to obtain token"
log "Token obtained"

discover_service_endpoints "$catalog_json"
[[ -z "$CINDER_URL" || -z "$NOVA_URL" || -z "$GLANCE_URL" ]] && error "Service URLs not configured"
log "Endpoints - Cinder: $CINDER_URL, Nova: $NOVA_URL, Glance: $GLANCE_URL"

if [[ "$DISCOVER_ALL_VMS" == "true" ]]; then
  [[ -n "$VM_NAME_FILTER" ]] && log "VM name filter: $VM_NAME_FILTER"
  [[ -n "$VM_TAG_FILTER" ]] && log "VM tag filter: $VM_TAG_FILTER"
  VM_DATA=$(discover_all_vms)
  [[ -z "$VM_DATA" ]] && log "No VMs found" && exit 0
  log "Discovered $(echo "$VM_DATA" | wc -l | tr -d ' ') VM(s)"
  
  while IFS='|' read -r VM VM_ID; do
    [[ -z "$VM" || -z "$VM_ID" ]] && continue
    validate_openstack_vm "$VM_ID" || log "Skipping $VM (invalid OpenStack VM)" && continue
    log "==== VM: $VM (ID: $VM_ID) ===="
    VM_DIR="$BACKUP_DIR/$VM/$(date +%F_%H-%M)"
    mkdir -p "$VM_DIR"
    backup_vm_config "$VM_ID" "$VM_DIR"
    VOL_IDS=$(get_attached_volumes "$VM_ID")
    [[ -z "$VOL_IDS" ]] && log "No volumes for $VM" && continue
    for VOL_ID in $VOL_IDS; do
      backup_volume "$VOL_ID" "$VM_DIR" || log "Failed: $VOL_ID"
    done
    log "Completed: $VM"
  done <<< "$VM_DATA"
else
  log "Using manual VM list"
  for VM in "${VM_LIST[@]}"; do
    log "==== VM: $VM ===="
    VM_ID=$(get_vm_id "$VM" || true)
    [[ -z "$VM_ID" ]] && log "VM '$VM' not found or invalid" && continue
    validate_openstack_vm "$VM_ID" || log "VM '$VM' is not a valid OpenStack server" && continue
    VM_DIR="$BACKUP_DIR/$VM/$(date +%F_%H-%M)"
    mkdir -p "$VM_DIR"
    backup_vm_config "$VM_ID" "$VM_DIR"
    VOL_IDS=$(get_attached_volumes "$VM_ID")
    [[ -z "$VOL_IDS" ]] && log "No volumes for $VM" && continue
    for VOL_ID in $VOL_IDS; do
      backup_volume "$VOL_ID" "$VM_DIR" || log "Failed: $VOL_ID"
    done
    log "Completed: $VM"
  done
fi

log "=== ALL BACKUPS COMPLETED ==="
