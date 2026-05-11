#!/bin/bash

# Build script for sp-caddy-manager binaries
# Creates binaries for both AMD64 and ARM64 architectures

set -e

echo "Building sp-caddy-manager binaries..."

# Create bin directory
mkdir -p bin

# Get version from VERSION file or use default
VERSION=$(cat VERSION 2>/dev/null || echo "unknown")
echo "Version: $VERSION"

# Build for AMD64 (without CGO for cross-compilation compatibility)
echo "Building for Linux AMD64..."
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags "-X main.version=$VERSION" -o bin/sp-caddy-manager_linux_amd64 .

# Build for ARM64 (without CGO for cross-compilation)
echo "Building for Linux ARM64..."
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -ldflags "-X main.version=$VERSION" -o bin/sp-caddy-manager_linux_arm64 .

# Create checksums
echo "Creating checksums..."
cd bin
sha256sum sp-caddy-manager_linux_amd64 > checksums.txt
sha256sum sp-caddy-manager_linux_arm64 >> checksums.txt

echo "Build complete!"
echo "Binaries created in bin/:"
ls -la sp-caddy-manager_*
echo ""
echo "Checksums:"
cat checksums.txt
