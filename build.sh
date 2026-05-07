#!/bin/bash

# Detect architecture
ARCH=$(uname -m)
case $ARCH in
    x86_64)
        ARCH_NAME="amd64"
        ;;
    aarch64)
        ARCH_NAME="arm64"
        ;;
    *)
        echo "Unsupported architecture: $ARCH"
        exit 1
        ;;
esac

OUTPUT="sp-caddy-manager-$ARCH_NAME"

echo "Building for $ARCH_NAME..."
go build -o "$OUTPUT"

if [ $? -eq 0 ]; then
    echo "Build successful: $OUTPUT"
else
    echo "Build failed"
    exit 1
fi