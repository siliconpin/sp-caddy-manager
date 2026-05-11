# Pushing Binaries to Releases

This guide covers how to push compiled binaries to both GitHub and GitLab releases.

## Prerequisites

- Git configured with remotes for both repositories
- Access tokens for GitHub and GitLab (if needed)
- Built binaries in `bin/` directory

## Build Binaries

```bash
# Build binaries for both architectures
./build-binaries.sh
```

This creates:
- `bin/sp-caddy-manager_linux_amd64`
- `bin/sp-caddy-manager_linux_arm64`
- `bin/checksums.txt`

## Method 1: Automated GitHub Releases (Recommended)

### Step 1: Update Version

```bash
# Update version in VERSION file
echo "1.6.3" > VERSION

# Commit version change
git add VERSION
git commit -m "Bump version to 1.6.3"
```

### Step 2: Create and Push Tag

```bash
# Create tag
git tag v1.6.3

# Push to both remotes
git push origin master
git push origin v1.6.3

git push gh master
git push gh v1.6.3
```

### Step 3: GitHub Actions (Automatic)

GitHub Actions will automatically:
- Build binaries for both architectures
- Create GitHub release
- Upload binaries as release assets

## Method 2: Manual Release Creation

### For GitHub

```bash
# Install GitHub CLI if not installed
# Ubuntu/Debian: sudo apt install gh
# macOS: brew install gh

# Create release manually
gh release create v1.6.3 \
  bin/sp-caddy-manager_linux_amd64 \
  bin/sp-caddy-manager_linux_arm64 \
  bin/checksums.txt \
  --title "Release v1.6.3" \
  --notes "Release version 1.6.3"
```

### For GitLab

```bash
# Install GitLab CLI if not installed
# curl -sL https://gitlab.com/gitlab-org/cli/-/raw/main/scripts/install.sh | sudo bash

# Create release manually
glab release create v1.6.3 \
  bin/sp-caddy-manager_linux_amd64 \
  bin/sp-caddy-manager_linux_arm64 \
  bin/checksums.txt \
  --name "Release v1.6.3" \
  --description "Release version 1.6.3"
```

## Method 3: Web Interface

### GitHub Releases

1. Go to: https://github.com/siliconpin/sp-caddy-manager/releases/new
2. Enter tag version: `v1.6.3`
3. Title: `Release v1.6.3`
4. Description: Add release notes
5. Upload binaries:
   - Drag and drop `bin/sp-caddy-manager_linux_amd64`
   - Drag and drop `bin/sp-caddy-manager_linux_arm64`
   - Drag and drop `bin/checksums.txt`
6. Click "Publish release"

### GitLab Releases

1. Go to: https://git.siliconpin.com/kar/sp-caddy-manager/-/releases/new
2. Enter tag version: `v1.6.3`
3. Title: `Release v1.6.3`
4. Description: Add release notes
5. Upload binaries:
   - Click "Choose files" and select binaries from `bin/`
6. Click "Create release"

## Quick Commands Summary

```bash
# Complete workflow for new release
./build-binaries.sh
echo "1.6.3" > VERSION
git add .
git commit -m "Release v1.6.3"
git tag v1.6.3
git push origin master && git push origin v1.6.3
git push gh master && git push gh v1.6.3
```

## Verify Releases

After pushing, verify releases are available at:
- GitHub: https://github.com/siliconpin/sp-caddy-manager/releases
- GitLab: https://git.siliconpin.com/kar/sp-caddy-manager/releases

## Download Links

Users can download binaries using:
```bash
# GitHub
wget https://github.com/siliconpin/sp-caddy-manager/releases/latest/download/sp-caddy-manager_linux_amd64
wget https://github.com/siliconpin/sp-caddy-manager/releases/latest/download/sp-caddy-manager_linux_arm64

# GitLab
wget https://git.siliconpin.com/kar/sp-caddy-manager/-/releases/latest/downloads/sp-caddy-manager_linux_amd64
wget https://git.siliconpin.com/kar/sp-caddy-manager/-/releases/latest/downloads/sp-caddy-manager_linux_arm64
```

## Automate with Script

Create `push-release.sh`:
```bash
#!/bin/bash
set -e

VERSION=$1
if [ -z "$VERSION" ]; then
    echo "Usage: $0 <version>"
    exit 1
fi

echo "Building binaries..."
./build-binaries.sh

echo "Updating version to $VERSION..."
echo "$VERSION" > VERSION
git add VERSION
git commit -m "Release v$VERSION"

echo "Creating tag v$VERSION..."
git tag "v$VERSION"

echo "Pushing to origin (GitLab)..."
git push origin master
git push origin "v$VERSION"

echo "Pushing to gh (GitHub)..."
git push gh master
git push gh "v$VERSION"

echo "Release v$VERSION pushed successfully!"
```

Usage:
```bash
chmod +x push-release.sh
./push-release.sh 1.6.3
```
