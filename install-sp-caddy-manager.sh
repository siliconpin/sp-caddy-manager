#!/bin/bash

# Exit on any error
set -e

# Configuration
APP_NAME="sp-caddy-manager"
GITHUB_REPO="siliconpin/sp-caddy-manager"
INSTALL_DIR="/usr/local/bin"
SERVICE_FILE="/etc/systemd/system/${APP_NAME}.service"

echo "Checking system architecture..."
ARCH=$(uname -m)

case "$ARCH" in
    x86_64)
        BINARY_NAME="${APP_NAME}_linux_amd64"
        ;;
    aarch64|arm64)
        BINARY_NAME="${APP_NAME}_linux_arm64"
        ;;
    *)
        echo "Error: Unsupported architecture $ARCH"
        exit 1
        ;;
esac

# Get latest release version
LATEST_RELEASE=$(curl -s "https://api.github.com/repos/${GITHUB_REPO}/releases/latest" | grep '"tag_name":' | sed -E 's/.*"([^"]+)".*/\1/')

# Try to download from releases first, fallback to GitHub blob if releases fail
echo "Attempting to download from GitHub releases..."
DOWNLOAD_URL="https://github.com/${GITHUB_REPO}/releases/download/${LATEST_RELEASE}/${BINARY_NAME}"

# Test if release file exists
if ! curl -s --head "${DOWNLOAD_URL}" | grep -q "200 OK"; then
    echo "Release file not found. Falling back to GitHub blob..."
    DOWNLOAD_URL="https://raw.githubusercontent.com/${GITHUB_REPO}/master/install-sp-caddy-manager.sh"
    echo "Downloading installer from: ${DOWNLOAD_URL}"
    curl -L "${DOWNLOAD_URL}" -o "/tmp/installer.sh"
    
    # Execute the downloaded installer
    chmod +x "/tmp/installer.sh"
    exec "/tmp/installer.sh"
fi

if command -v "${APP_NAME}" &> /dev/null; then
    echo "Binary ${APP_NAME} found. Checking for updates..."
    
    # Try to get current version (this is a simple check - you may want to improve this)
    CURRENT_VERSION=$(${APP_NAME} --version 2>/dev/null | head -1 || echo "unknown")
    
    if [ "$CURRENT_VERSION" = "$LATEST_RELEASE" ] || [ "$CURRENT_VERSION" != "unknown" ]; then
        echo "Current version: $CURRENT_VERSION"
        echo "Latest version: $LATEST_RELEASE"
        
        if [ "$CURRENT_VERSION" = "$LATEST_RELEASE" ]; then
            echo "Already up to date. Skipping download."
        else
            echo "Updating to version $LATEST_RELEASE..."
            
            echo "Downloading from: ${DOWNLOAD_URL}"
            curl -L "${DOWNLOAD_URL}" -o "/tmp/${APP_NAME}"
            echo "Updating binary to ${INSTALL_DIR}..."
            sudo mv "/tmp/${APP_NAME}" "${INSTALL_DIR}/${APP_NAME}"
            sudo chmod +x "${INSTALL_DIR}/${APP_NAME}"
            echo "Update completed successfully!"
        fi
    else
        echo "Cannot determine current version. Reinstalling..."
        
        echo "Downloading from: ${DOWNLOAD_URL}"
        curl -L "${DOWNLOAD_URL}" -o "/tmp/${APP_NAME}"
        echo "Installing binary to ${INSTALL_DIR}..."
        sudo mv "/tmp/${APP_NAME}" "${INSTALL_DIR}/${APP_NAME}"
        sudo chmod +x "${INSTALL_DIR}/${APP_NAME}"
    fi
else
    echo "Installing ${APP_NAME} version $LATEST_RELEASE..."
    
    echo "Downloading from: ${DOWNLOAD_URL}"
    curl -L "${DOWNLOAD_URL}" -o "/tmp/${APP_NAME}"
    echo "Installing binary to ${INSTALL_DIR}..."
    sudo mv "/tmp/${APP_NAME}" "${INSTALL_DIR}/${APP_NAME}"
    sudo chmod +x "${INSTALL_DIR}/${APP_NAME}"
fi

echo "Creating .env file..."
sudo bash -c "cat > ${INSTALL_DIR}/.env" <<EOF
PORT=1011
DB_PATH=${INSTALL_DIR}/domains.sqlite
CADDY_CONFIG_DIR=/etc/caddy/conf.d
CADDY_API_URL=http://localhost:2019/config/apps/http/servers/srv0/routes
CADDYFILE_PATH=/etc/caddy/Caddyfile
BACKEND_HOST=0.0.0.0
EOF

echo "Setting permissions for .env file..."
sudo chmod 644 ${INSTALL_DIR}/.env
sudo chown root:root ${INSTALL_DIR}/.env

echo "Creating systemd service..."
sudo bash -c "cat > ${SERVICE_FILE}" <<EOF
[Unit]
Description=Caddy Manager API for SiliconPin
After=network.target caddy.service

[Service]
Type=simple
User=root
WorkingDirectory=${INSTALL_DIR}
ExecStart=${INSTALL_DIR}/${APP_NAME}
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
EOF

echo "Reloading systemd and starting service..."
sudo systemctl daemon-reload
sudo systemctl enable "${APP_NAME}"
sudo systemctl restart "${APP_NAME}"

echo "Done! ${APP_NAME} is now running."
