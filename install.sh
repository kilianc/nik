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

case "$OS" in
  darwin|linux) ;;
  *) echo "unsupported OS: $OS (supported: darwin, linux)" >&2; exit 1 ;;
esac

if [ "$OS" = "darwin" ] && [ "$ARCH" = "amd64" ]; then
  echo "Intel Macs aren't published as binaries. Build from source: https://github.com/kilianc/nik#from-source" >&2
  exit 1
fi

if [ "$VERSION" = "latest" ]; then
  URL="https://github.com/kilianc/nik/releases/latest/download/nik-${OS}-${ARCH}"
else
  URL="https://github.com/kilianc/nik/releases/download/${VERSION}/nik-${OS}-${ARCH}"
fi

echo "Downloading nik (${OS}/${ARCH}) from ${URL}..."
curl -fsSL "$URL" -o /tmp/nik
chmod +x /tmp/nik
sudo mv /tmp/nik "${INSTALL_DIR}/nik"

if [ "$OS" = "darwin" ]; then
  if [ "$VERSION" = "latest" ]; then
    LINUX_URL="https://github.com/kilianc/nik/releases/latest/download/nik-linux-${ARCH}"
  else
    LINUX_URL="https://github.com/kilianc/nik/releases/download/${VERSION}/nik-linux-${ARCH}"
  fi
  echo "Downloading nik (linux/${ARCH}) for shell container from ${LINUX_URL}..."
  curl -fsSL "$LINUX_URL" -o /tmp/nik-linux-${ARCH}
  chmod +x /tmp/nik-linux-${ARCH}
  sudo mv /tmp/nik-linux-${ARCH} "${INSTALL_DIR}/nik-linux-${ARCH}"
fi

echo "Setting up daemon service..."
mkdir -p "$NIK_HOME"
nik install --home "$NIK_HOME"

echo ""
echo "nik is running. Open a new terminal and run 'nik' to get started."
