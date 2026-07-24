# sws — Savannaa Cloud CLI

Command-line client for [Savannaa Cloud](https://savannaa.com). Drive the platform from a terminal or shell script — compute, networks, storage, Kubernetes, managed databases and more — without touching the web console.

```
sws compute list
sws compute create --name web-1 --image "Ubuntu 24.04 LTS" --plan m1.small
sws ip create
sws cluster create prod-k8s --template kubernetes-default --workers 3
```

---

## Install

### Linux

\`\`\`bash
curl -fsSL https://raw.githubusercontent.com/savannaacloud/sws/main/install.sh | sh
\`\`\`

Detects your architecture (amd64 / arm64), downloads the latest release binary, and drops it at `/usr/local/bin/sws`.

### macOS

\`\`\`bash
curl -fsSL https://raw.githubusercontent.com/savannaacloud/sws/main/install.sh | sh
\`\`\`

Same installer as Linux. Works on Intel (amd64) and Apple Silicon (arm64).

### Windows

Open PowerShell and run:

\`\`\`powershell
irm https://raw.githubusercontent.com/savannaacloud/sws/main/install.ps1 | iex
\`\`\`

Detects your architecture (amd64 / arm64), downloads `sws.exe`, drops it at `%LOCALAPPDATA%\sws\sws.exe`, and adds that folder to your user PATH. Open a new PowerShell window after install for `sws` to be on PATH.

### Environment overrides (all platforms)

| Variable | Default | Notes |
|---|---|---|
| `SWS_VERSION` | latest `v*` tag | Pin a specific version in CI |
| `SWS_INSTALL_DIR` | Linux/macOS: `/usr/local/bin` · Windows: `%LOCALAPPDATA%\sws` | Where to drop the binary |

### go install

\`\`\`bash
go install github.com/savannaacloud/sws@latest
\`\`\`

### Manual download

If one-liners arent your thing, grab the binary directly from [Releases](https://github.com/savannaacloud/sws/releases/latest):

| Platform | File |
|---|---|
| Linux x86_64 | `sws-linux-amd64` |
| Linux ARM64 | `sws-linux-arm64` |
| macOS Intel | `sws-darwin-amd64` |
| macOS Apple Silicon | `sws-darwin-arm64` |
| Windows x64 | `sws-windows-amd64.exe` |
| Windows ARM64 | `sws-windows-arm64.exe` |

Verify the download against `checksums.txt` in the release:

\`\`\`bash
sha256sum -c checksums.txt --ignore-missing
\`\`\`

## Configure (recommended)

`sws configure` is an `aws configure`-style interactive setup. It prompts for
your API URL, an API token (create one under **Account → API Keys** — `ctk_…`),
and a default region, then saves them to `~/.sws/config.yaml` so you don't need
to export env vars (requires v1.0.2+):

```bash
$ sws configure
API URL [https://savannaa.com]:
API token (ctk_...): ctk_your_api_key_here
Default region (ng-abuja-1 | ng-lagos-1) [ng-lagos-1]: ng-abuja-1
✓ Saved profile default to ~/.sws/config.yaml

# Then just run commands — no env vars needed:
sws compute list
```

Use `SWS_PROFILE` to keep multiple profiles (e.g. one per region); each is a
separate entry in `~/.sws/config.yaml`:

```bash
SWS_PROFILE=abuja sws configure     # saves the "abuja" profile
SWS_PROFILE=abuja sws compute list  # uses it
```

## First login

```bash
sws login
# Email: you@example.com
# Password: ********
# API URL: https://savannaa.com
```

The password prompt uses **hidden input** — what you type is never echoed to the
screen or left in terminal scrollback (v1.1.1+). `sws login <email>` prompts for
just the password. Token is cached at `~/.config/sws/credentials.yaml` and
`~/.config/sws/token`; subsequent commands reuse it until it expires.

Non-interactive / CI — prefer a pre-issued token (below). If you must pass a
password on the command line, note it is **visible in your shell history and the
process list**:

```bash
sws login you@example.com '$PASSWORD'   # discouraged: leaks into shell history
```

Or skip login entirely by passing a pre-issued token + region:

```bash
export SWS_API_URL=https://savannaa.com
export SWS_TOKEN=ctk_...
export SWS_REGION=ng-abuja-1   # ng-abuja-1 | ng-lagos-1 (default ng-lagos-1)
sws compute list
```

> Precedence: environment variables override `~/.sws/config.yaml`. `SWS_REGION`
> is sent as the `x-region` header — without it, commands target `ng-lagos-1`,
> so a list can come back empty if your resources are in the other region.

---

## Common recipes

### Instances (VMs)

```bash
sws compute list
sws compute create --name web-1 --image "Ubuntu 24.04 LTS" --plan m1.small --network default --keypair my-key
sws compute start <id>
sws compute stop <id>
sws compute delete <id>
```

### Public IPs

```bash
sws ip list
sws ip create
sws ip attach <fip-id> <instance-id>
sws ip detach <fip-id>
sws ip delete <fip-id>
```

### Block storage

```bash
sws volume list
sws volume create --name data --size 50    # GB
sws volume attach <vol-id> <instance-id>
sws volume delete <vol-id>
```

### Networks + firewalls

```bash
sws network list
sws firewall list
```

### Managed databases

```bash
sws database list
```

### Containers (serverless)

```bash
sws container list
sws container run --name api --image python:3.12
sws container logs <container-id>
sws container delete <container-id>
```

### Kubernetes clusters

```bash
sws cluster list
sws cluster create prod-k8s --template kubernetes-default --workers 3
sws cluster kubeconfig <cluster-id> > ~/.kube/config-savannaa
sws cluster delete <cluster-id>
```

### Load balancers, secrets, images

```bash
sws lb list
sws secret list
sws image list
```

---

## Configuration

`sws` reads settings from (first wins):

1. **Environment variables** — `SWS_API_URL`, `SWS_TOKEN`
2. **`~/.config/sws/credentials.yaml`** — written by `sws login`
3. **Interactive prompt** — when missing

### Scripts / CI

Prefer env-var auth:

```bash
export SWS_API_URL=https://savannaa.com
export SWS_TOKEN="$(cat /etc/sws/token)"
sws compute list --format json | jq '.[] | .name'
```

---

## Troubleshooting

| Symptom | Cause | Fix |
|---|---|---|
| `Error: not authenticated. Run: sws login` | no token in env or config | `sws login` or set `SWS_TOKEN` |
| `error 401: ...` | token expired or wrong password | `sws login` again |
| `error 403: No project assigned` | user has no project bound | ask your admin |
| `error 404` on a subcommand | backend route not registered | check the console works; open an issue |

---

## Changelog

- **v1.1.1** — `sws login` now reads the password with **hidden input** (no
  terminal echo, nothing left in scrollback). `sws login <email>` prompts for the
  password instead of requiring it on the command line; the two-argument form
  warns that a password passed as an argument leaks into shell history. Piped /
  non-interactive stdin still works for automation.

---


## License

[TBD]

