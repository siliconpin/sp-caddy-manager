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

if command -v "${APP_NAME}" &> /dev/null; then
    echo "Binary ${APP_NAME} found. Skipping download..."
    BINARY_PATH=$(command -v "${APP_NAME}")
    sudo cp "${BINARY_PATH}" "${INSTALL_DIR}/${APP_NAME}"
    sudo chmod +x "${INSTALL_DIR}/${APP_NAME}"
else
    echo "Downloading ${BINARY_NAME} from GitHub releases..."
    LATEST_RELEASE=$(curl -s "https://api.github.com/repos/${GITHUB_REPO}/releases/latest" | grep '"tag_name":' | sed -E 's/.*"([^"]+)".*/\1/')
    DOWNLOAD_URL="https://github.com/${GITHUB_REPO}/releases/download/${LATEST_RELEASE}/${BINARY_NAME}"
    
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
