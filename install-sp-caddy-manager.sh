#!/bin/bash

# Exit on any error
set -e

# Configuration
APP_NAME="sp-caddy-manager"
BASE_URL="https://siliconpin.com"
INSTALL_DIR="/usr/local/bin"
SERVICE_FILE="/etc/systemd/system/${APP_NAME}.service"

echo "Checking system architecture..."
ARCH=$(uname -m)

case "$ARCH" in
    x86_64)
        BINARY_NAME="${APP_NAME}-amd64"
        ;;
    aarch64|arm64)
        BINARY_NAME="${APP_NAME}-arm64"
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
    echo "Downloading ${BINARY_NAME}..."
    curl -L "${BASE_URL}/${BINARY_NAME}" -o "/tmp/${APP_NAME}"
    echo "Installing binary to ${INSTALL_DIR}..."
    sudo mv "/tmp/${APP_NAME}" "${INSTALL_DIR}/${APP_NAME}"
    sudo chmod +x "${INSTALL_DIR}/${APP_NAME}"
fi

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
