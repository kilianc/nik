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

# Root is reached with sudo only when we are not already root. A container is
# root with no sudo installed at all — an operator installing nik into a container does exactly
# that way — and there `sudo mv` is not a permission error but `sudo: not
# found`, exit 127, which under `set -eu` ends the install on the first binary.
SUDO=""
[ "$(id -u)" -eq 0 ] || SUDO=sudo

# nikd and nikctl install together. A host with one and not the other has no
# working nik: nikctl writes a service file pointing at its sibling, and nikd
# mounts its sibling into the shell sandbox.
fetch() {
  echo "Downloading $1 from ${BASE}/$1..."
  curl -fsSL "${BASE}/$1" -o "/tmp/$1"
  chmod +x "/tmp/$1"
  $SUDO mv "/tmp/$1" "${INSTALL_DIR}/$2"
}

fetch "nikd-${OS}-${ARCH}" nikd
fetch "nikctl-${OS}-${ARCH}" nikctl

# `nik` is how nikctl is spelled in the house — the README, the docs and
# everyone's muscle memory say it, and `nikctl chat` reads like infrastructure.
$SUDO ln -sf "${INSTALL_DIR}/nikctl" "${INSTALL_DIR}/nik"

# On macOS the sandbox cannot run the native client, so the linux build of
# nikctl rides along for the container mount. nikd is never mounted there.
if [ "$OS" = "darwin" ]; then
  fetch "nikctl-linux-${ARCH}" "nikctl-linux-${ARCH}"
fi

mkdir -p "$NIK_HOME"

# The service starts FIRST, and the account is linked into the running daemon.
#
# This used to be the other way around, because a daemon with no gateway died
# on arrival and the service manager would restart it forever. nikd now serves
# its API before it has a config and waits for exactly this, so the token goes
# to the process that will use it — and a token that arrives an hour later
# works as well as one that arrives now.
echo "Setting up daemon service..."
nikctl install --home "$NIK_HOME"

if [ -n "$NIK_TOKEN" ]; then
  echo "Linking this nik to your account..."
  if [ -n "$NIK_GATEWAY_URL" ]; then
    nikctl connect --home "$NIK_HOME" --url "$NIK_GATEWAY_URL" "$NIK_TOKEN"
  else
    nikctl connect --home "$NIK_HOME" "$NIK_TOKEN"
  fi
fi

echo ""
if [ -n "$NIK_TOKEN" ]; then
  echo "nik is running and linked to your account. Open a new terminal and run 'nik' to finish setup."
else
  echo "nik is installed. Open a new terminal and run 'nik' to get started."
fi
