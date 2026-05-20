#!/usr/bin/env bash
set -euo pipefail

APP="sp-caddy-manager"
VERSION="$(cat VERSION 2>/dev/null || echo dev)"

if [[ -z "${ARCH:-}" ]]; then
    case "$(uname -m)" in
        x86_64|amd64) ARCH="amd64" ;;
        aarch64|arm64) ARCH="arm64" ;;
        armv7l|armv6l) ARCH="arm" ;;
        *) echo "Unsupported architecture: $(uname -m)" >&2; exit 1 ;;
    esac
fi

OUT="${BUILD_OUTPUT:-dist/${APP}_linux_${ARCH}}"
mkdir -p "$(dirname "$OUT")"

echo "Building ${APP} ${VERSION} for linux/${ARCH} -> ${OUT}"

GOOS=linux \
GOARCH="$ARCH" \
CGO_ENABLED="${CGO_ENABLED:-0}" \
GOTOOLCHAIN="${GOTOOLCHAIN:-local}" \
go build -trimpath -buildvcs=false -ldflags="-s -w -X main.version=${VERSION}" -o "$OUT" .

echo "Build successful: ${OUT}"
