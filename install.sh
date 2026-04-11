#!/bin/sh
set -eu

NIK_HOME="${NIK_HOME:-$HOME/.nik}"
VERSION="${NIK_VERSION:-latest}"
INSTALL_DIR="${NIK_INSTALL_DIR:-/usr/local/bin}"

OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
ARCH="$(uname -m)"
case "$ARCH" in
  x86_64)  ARCH="amd64" ;;
  aarch64) ARCH="arm64" ;;
esac

echo "Downloading nik (${OS}/${ARCH})..."
curl -fsSL "https://github.com/kciuffolo/nik/releases/${VERSION}/download/nik-${OS}-${ARCH}" \
  -o /tmp/nik
chmod +x /tmp/nik
sudo mv /tmp/nik "${INSTALL_DIR}/nik"

echo "Setting up daemon service..."
mkdir -p "$NIK_HOME"
nik install --home "$NIK_HOME"

echo ""
echo "nik is running. Open a new terminal and run 'nik' to get started."
