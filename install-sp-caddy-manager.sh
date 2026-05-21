#!/bin/bash

# Exit on any error
set -e

# Configuration
APP_NAME="sp-caddy-manager"
GITHUB_REPO="siliconpin/sp-caddy-manager"
INSTALL_DIR="/usr/local/bin"
SERVICE_FILE="/etc/systemd/system/${APP_NAME}.service"

install_binary() {
    local download_url="$1"
    local tmp_binary="/tmp/${APP_NAME}"

    echo "Downloading from: ${download_url}"
    curl -fL "${download_url}" -o "${tmp_binary}"
    chmod +x "${tmp_binary}"

    echo "Validating downloaded binary..."
    "${tmp_binary}" --version >/dev/null

    echo "Installing binary to ${INSTALL_DIR}..."
    sudo mv "${tmp_binary}" "${INSTALL_DIR}/${APP_NAME}"
    sudo chmod +x "${INSTALL_DIR}/${APP_NAME}"
}

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
            DOWNLOAD_URL="https://github.com/${GITHUB_REPO}/releases/download/${LATEST_RELEASE}/${BINARY_NAME}"
            install_binary "${DOWNLOAD_URL}"
            echo "Update completed successfully!"
        fi
    else
        echo "Cannot determine current version. Reinstalling..."
        DOWNLOAD_URL="https://github.com/${GITHUB_REPO}/releases/download/${LATEST_RELEASE}/${BINARY_NAME}"
        install_binary "${DOWNLOAD_URL}"
    fi
else
    echo "Installing ${APP_NAME} version $LATEST_RELEASE..."
    DOWNLOAD_URL="https://github.com/${GITHUB_REPO}/releases/download/${LATEST_RELEASE}/${BINARY_NAME}"
    install_binary "${DOWNLOAD_URL}"
fi

echo "Creating .env file..."
sudo bash -c "cat > ${INSTALL_DIR}/.env" <<EOF
PORT=1011
DB_PATH=${INSTALL_DIR}/domains.sqlite
CADDY_CONFIG_DIR=/etc/caddy/conf.d
CADDY_API_URL=http://localhost:2019/config/apps/http/servers/srv0/routes
CADDYFILE_PATH=/etc/caddy/Caddyfile
BACKEND_HOST=0.0.0.0
API_KEY_DIR=${INSTALL_DIR}/keys
EOF

echo "Setting permissions for .env file..."
sudo chmod 644 ${INSTALL_DIR}/.env
sudo chown root:root ${INSTALL_DIR}/.env

echo "Ensuring Caddy snippet directory and placeholder..."
sudo mkdir -p /etc/caddy/conf.d
sudo bash -c "cat > /etc/caddy/conf.d/empty.caddy" <<EOF
# sp-caddy-manager placeholder
EOF
sudo chmod 755 /etc/caddy /etc/caddy/conf.d
sudo chmod 644 /etc/caddy/conf.d/empty.caddy

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
