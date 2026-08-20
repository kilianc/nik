#!/bin/sh
set -eu

NIK_HOME="${NIK_HOME:-$HOME/.nik}"
VERSION="${NIK_VERSION:-latest}"
INSTALL_DIR="${NIK_INSTALL_DIR:-/usr/local/bin}"
# From the dashboard's one-liner: the agent token, and the gateway this
# account lives on. With them, the install ends connected; without, the
# first-run setup asks.
NIK_TOKEN="${NIK_TOKEN:-}"
NIK_GATEWAY_URL="${NIK_GATEWAY_URL:-}"

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
  BASE="https://github.com/kilianc/nik/releases/latest/download"
else
  BASE="https://github.com/kilianc/nik/releases/download/${VERSION}"
fi

# nikd and nikctl install together. A host with one and not the other has no
# working nik: nikctl writes a service file pointing at its sibling, and nikd
# mounts its sibling into the shell sandbox.
fetch() {
  echo "Downloading $1 from ${BASE}/$1..."
  curl -fsSL "${BASE}/$1" -o "/tmp/$1"
  chmod +x "/tmp/$1"
  sudo mv "/tmp/$1" "${INSTALL_DIR}/$2"
}

fetch "nikd-${OS}-${ARCH}" nikd
fetch "nikctl-${OS}-${ARCH}" nikctl

# `nik` is how nikctl is spelled in the house — the README, the docs and
# everyone's muscle memory say it, and `nikctl chat` reads like infrastructure.
sudo ln -sf "${INSTALL_DIR}/nikctl" "${INSTALL_DIR}/nik"

# On macOS the sandbox cannot run the native client, so the linux build of
# nikctl rides along for the container mount. nikd is never mounted there.
if [ "$OS" = "darwin" ]; then
  fetch "nikctl-linux-${ARCH}" "nikctl-linux-${ARCH}"
fi

mkdir -p "$NIK_HOME"

# The daemon refuses to boot without a gateway, so link the account BEFORE
# the service starts — otherwise launchd/systemd would restart a nik that
# dies on arrival until setup gets around to it.
if [ -n "$NIK_TOKEN" ]; then
  echo "Linking this nik to your account..."
  if [ -n "$NIK_GATEWAY_URL" ]; then
    nikctl connect --home "$NIK_HOME" --url "$NIK_GATEWAY_URL" "$NIK_TOKEN"
  else
    nikctl connect --home "$NIK_HOME" "$NIK_TOKEN"
  fi
fi

echo "Setting up daemon service..."
nikctl install --home "$NIK_HOME"

echo ""
if [ -n "$NIK_TOKEN" ]; then
  echo "nik is running and linked to your account. Open a new terminal and run 'nik' to finish setup."
else
  echo "nik is installed. Open a new terminal and run 'nik' to get started."
fi
