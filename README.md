# SP Caddy Manager

Small Go service for managing Caddy reverse proxy domain entries backed by SQLite.

It can:

- Add a domain and port to SQLite.
- Create a matching `.caddy` snippet.
- Push routes to the Caddy JSON API.
- Delete domain entries from SQLite, disk, and Caddy.
- Show DB entries and live Caddy entries separately in the web UI.

## Requirements

- Go
- SQLite
- Caddy with admin API enabled
- Caddyfile importing snippets, for example:

```caddyfile
import /etc/caddy/conf.d/*.caddy
```

## Configuration

The app reads `.env`, but real environment variables take priority.

```bash
PORT=1011
DB_PATH=./domains.sqlite
CADDY_CONFIG_DIR=/etc/caddy/conf.d
CADDY_API_URL=http://localhost:2019/config/apps/http/servers/srv0/routes
CADDYFILE_PATH=/etc/caddy/Caddyfile
API_KEY_DIR=/usr/local/bin/keys
```

You can also set the backend host Caddy should dial (default `0.0.0.0`):

```bash
BACKEND_HOST=0.0.0.0
```

## Run

```bash
go run .
```

Run in development UI mode:

```bash
go run . dev
```

Open in development UI mode:

```text
http://localhost:1011/
```

## Version And API Keys

Show the installed version:

```bash
/usr/local/bin/sp-caddy-manager -v
```

Create, list, and delete API keys:

```bash
/usr/local/bin/sp-caddy-manager key add key1
/usr/local/bin/sp-caddy-manager key list
/usr/local/bin/sp-caddy-manager key delete key1
```

`key add <label>` prints the generated key once and creates a text file in `API_KEY_DIR` containing a SHA-256 hash of that key. If `API_KEY_DIR` is not set, keys are stored in a `keys` directory next to the `sp-caddy-manager` binary.

The web UI at `/` asks for the key label and key value before showing management controls.

## Installation

### Download Pre-built Binaries

You can download pre-compiled binaries from the releases page:

**GitHub Releases:**
- https://github.com/siliconpin/sp-caddy-manager/releases

**GitLab Releases:**
- https://git.siliconpin.com/kar/sp-caddy-manager/releases

Available binaries:
- `sp-caddy-manager_linux_amd64` - For Intel/AMD 64-bit systems
- `sp-caddy-manager_linux_arm64` - For ARM 64-bit systems (Raspberry Pi, etc.)

Download the appropriate binary for your system, make it executable, and move it to your PATH:

```bash
# Download for AMD64
wget https://github.com/siliconpin/sp-caddy-manager/releases/latest/download/sp-caddy-manager_linux_amd64
chmod +x sp-caddy-manager_linux_amd64
sudo mv sp-caddy-manager_linux_amd64 /usr/local/bin/sp-caddy-manager

# Or for ARM64
wget https://github.com/siliconpin/sp-caddy-manager/releases/latest/download/sp-caddy-manager_linux_arm64
chmod +x sp-caddy-manager_linux_arm64
sudo mv sp-caddy-manager_linux_arm64 /usr/local/bin/sp-caddy-manager
```

### Quick Install Script

For automated installation, use the installer script:

```bash
curl -fsSL https://raw.githubusercontent.com/siliconpin/sp-caddy-manager/master/install-sp-caddy-manager.sh | sudo bash
```

### Build from Source

```bash
./build.sh
```

The script builds a Linux binary for the current machine architecture. Set `BUILD_OUTPUT` if you want a different output path.

Or directly:

```bash
go build -o sp-caddy-manager
```

## API

All API calls use `POST /manage-domain` with an `action`.

Domains must be valid hostnames, and ports must be in `1-65535`.

```bash
curl -X POST http://localhost:1011/manage-domain \
  -H "Content-Type: application/json" \
  -H "X-API-Key: <key>" \
  -d '{"action":"add","domain":"app.example.com","port":8080}'
```

Override the backend host for a single request by including `backend_host` in the JSON payload:

```bash
curl -X POST http://localhost:1011/manage-domain \
  -H "Content-Type: application/json" \
  -H "X-API-Key: <key>" \
  -d '{"action":"add","domain":"app.example.com","port":8080,"backend_host":"10.0.0.140"}'
```

Delete a domain:

```bash
curl -X POST http://localhost:1011/manage-domain \
  -H "Content-Type: application/json" \
  -H "X-API-Key: <key>" \
  -d '{"action":"delete","domain":"app.example.com"}'

curl -X POST http://localhost:1011/manage-domain \
  -H "Content-Type: application/json" \
  -H "X-API-Key: <key>" \
  -d '{"action":"reset-and-import-config-to-db"}'

```

Add raw Caddyfile content:

```bash
curl -X POST http://localhost:1011/manage-domain \
  -H "Content-Type: application/json" \
  -H "X-API-Key: <key>" \
  -d '{"action":"add-caddyfile","content":"app.example.com {\n  reverse_proxy localhost:8080\n}"}'
```

Import a `.caddy` file directly (multipart form upload). Filename should be like `sub.domain.com.caddy` — the domain is inferred from the filename:

```bash
curl -X POST http://localhost:1011/manage-domain \
  -H "X-API-Key: <key>" \
  -F action=import-caddyfile \
  -F file=@sub.domain.com.caddy
```

List entries:

```bash
curl -X POST http://localhost:1011/manage-domain \
  -H "Content-Type: application/json" \
  -H "X-API-Key: <key>" \
  -d '{"action":"list-db"}'

curl -X POST http://localhost:1011/manage-domain \
  -H "Content-Type: application/json" \
  -H "X-API-Key: <key>" \
  -d '{"action":"list-db-with-content"}'

curl -X POST http://localhost:1011/manage-domain \
  -H "Content-Type: application/json" \
  -H "X-API-Key: <key>" \
  -d '{"action":"list-caddy"}'
```

## Test

```bash
go test ./...
```
