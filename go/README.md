# protect-ostack (Go)

Pure Go port of the [bash backup script](../scripts/bash/protect-ostack.sh). Uses only the standard library (no external dependencies).

OpenStack logic lives in the **ostack** library (`tools/ostack/`): Keystone auth and catalog, Nova (VMs, tags, metadata), Cinder (volumes, snapshots), Glance (images), and backup orchestration. `main.go` handles CLI flags and calls into the lib.

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

See [scripts/bash/README.md](../scripts/bash/README.md) for full documentation (features, options, backup layout, troubleshooting).

## Requirements

- Go 1.21+
