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
```

You can also set the backend host Caddy should dial (default `0.0.0.0`):

```bash
BACKEND_HOST=0.0.0.0
```

## Run

```bash
go run .
```

Open:

```text
http://localhost:1011/
```

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

### Build from Source

```bash
./build-binaries.sh
```

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
  -d '{"action":"add","domain":"app.example.com","port":8080}'
```

Override the backend host for a single request by including `backend_host` in the JSON payload:

```bash
curl -X POST http://localhost:1011/manage-domain \
  -H "Content-Type: application/json" \
  -d '{"action":"add","domain":"app.example.com","port":8080,"backend_host":"10.0.0.140"}'
```

Delete a domain:

```bash
curl -X POST http://localhost:1011/manage-domain \
  -H "Content-Type: application/json" \
  -d '{"action":"delete","domain":"app.example.com"}'

curl -X POST http://localhost:1011/manage-domain \
  -H "Content-Type: application/json" \
  -d '{"action":"reset-and-import-config-to-db"}'

```

Add raw Caddyfile content:

```bash
curl -X POST http://localhost:1011/manage-domain \
  -H "Content-Type: application/json" \
  -d '{"action":"add-caddyfile","content":"app.example.com {\n  reverse_proxy localhost:8080\n}"}'
```

Import a `.caddy` file directly (multipart form upload). Filename should be like `sp-api.ns77.domain.com.caddy` — the domain is inferred from the filename:

```bash
curl -X POST http://localhost:1011/manage-domain \
  -F action=import-caddyfile \
  -F file=@sp-api.ns77.domain.com.caddy
```

List entries:

```bash
curl -X POST http://localhost:1011/manage-domain \
  -H "Content-Type: application/json" \
  -d '{"action":"list-db"}'

curl -X POST http://localhost:1011/manage-domain \
  -H "Content-Type: application/json" \
  -d '{"action":"list-db-with-content"}'

curl -X POST http://localhost:1011/manage-domain \
  -H "Content-Type: application/json" \
  -d '{"action":"list-caddy"}'
```

## Test

```bash
go test ./...
```
