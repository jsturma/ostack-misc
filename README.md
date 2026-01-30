# ostack-misc

Miscellaneous OpenStack-related scripts and assets.

## Project layout

| Path | Description |
|------|-------------|
| [go/](go/) | Go code |
| [scripts/bash/](scripts/bash/) | Bash scripts (e.g. [protect-ostack.sh](scripts/bash/protect-ostack.sh) — VM and volume backup) |
| [diagrams/](diagrams/) | Diagram sources and exports |
| [LICENSE](LICENSE) | MIT License |

## Scripts

- **[scripts/bash/protect-ostack.sh](scripts/bash/protect-ostack.sh)** — Back up OpenStack VMs and their Cinder volumes (bash). Full usage, options, and troubleshooting: [scripts/bash/README.md](scripts/bash/README.md).
- **[go/](go/)** — Pure Go port of the same backup tool (stdlib only). Build: `go build -o protect-ostack .` in `go/`. See [go/README.md](go/README.md).

## License

MIT — see [LICENSE](LICENSE).
