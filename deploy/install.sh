#!/usr/bin/env bash
#
# one-time bootstrap for nik on a fresh Raspberry Pi.
# usage: sudo ./install.sh [--workspace <path>] [--repo <git-ssh-url>] [--branch <branch>]
#
set -euo pipefail

REPO="${NIK_REPO:-git@github.com:kciuffolo/nik.git}"
BRANCH="${NIK_BRANCH:-main}"
INSTALL_DIR=""
NIK_USER="nik"
WORKSPACE_SRC=""

while [[ $# -gt 0 ]]; do
  case "$1" in
    --repo)      REPO="$2";          shift 2 ;;
    --branch)    BRANCH="$2";        shift 2 ;;
    --workspace) WORKSPACE_SRC="$2"; shift 2 ;;
    *)           echo "unknown flag: $1"; exit 1 ;;
  esac
done

if [[ $EUID -ne 0 ]]; then
  echo "run as root: sudo $0"
  exit 1
fi

SUDO_HOME=$(eval echo "~${SUDO_USER:-$USER}")

echo "==> installing system dependencies"
apt-get update -qq
apt-get install -y -qq git gcc make tmux curl

echo "==> creating $NIK_USER user"
if ! id "$NIK_USER" &>/dev/null; then
  useradd --system --create-home --shell /bin/bash "$NIK_USER"
  echo "  created user $NIK_USER"
fi

NIK_HOME=$(eval echo "~$NIK_USER")
INSTALL_DIR="$NIK_HOME/git"

echo "==> setting up SSH key for $NIK_USER"
NIK_SSH="$NIK_HOME/.ssh"
if [[ ! -f "$NIK_SSH/id_ed25519" ]]; then
  sudo -u "$NIK_USER" mkdir -p "$NIK_SSH"
  chmod 700 "$NIK_SSH"
  sudo -u "$NIK_USER" ssh-keygen -t ed25519 -N "" -f "$NIK_SSH/id_ed25519" -C "nik@$(hostname)"
  sudo -u "$NIK_USER" bash -c "echo 'Host github.com
  StrictHostKeyChecking accept-new' > $NIK_SSH/config"
  chmod 600 "$NIK_SSH/config"
  echo ""
  echo "  ============================================"
  echo "  add this deploy key to your GitHub repo:"
  echo "  https://github.com/kciuffolo/nik/settings/keys"
  echo "  ============================================"
  echo ""
  cat "$NIK_SSH/id_ed25519.pub"
  echo ""
  echo "  press Enter once you've added the key..."
  read -r
else
  echo "  SSH key already exists"
fi

echo "==> detecting architecture"
ARCH=$(dpkg --print-architecture)
case "$ARCH" in
  arm64|aarch64) GO_ARCH="arm64" ;;
  armhf|armv7l)  GO_ARCH="armv6l" ;;
  amd64)         GO_ARCH="amd64" ;;
  *)             echo "unsupported arch: $ARCH"; exit 1 ;;
esac
echo "  arch: $ARCH -> go linux/$GO_ARCH"

GO_VERSION="1.25.4"

INSTALLED_GO=""
if command -v /usr/local/go/bin/go &>/dev/null; then
  INSTALLED_GO=$(/usr/local/go/bin/go version | awk '{print $3}' | sed 's/go//')
fi

if [[ "$INSTALLED_GO" != "$GO_VERSION" ]]; then
  echo "==> installing Go $GO_VERSION for $GO_ARCH"
  GO_TAR="go${GO_VERSION}.linux-${GO_ARCH}.tar.gz"
  curl -fsSL "https://go.dev/dl/$GO_TAR" -o "/tmp/$GO_TAR"
  rm -rf /usr/local/go
  tar -C /usr/local -xzf "/tmp/$GO_TAR"
  rm "/tmp/$GO_TAR"
else
  echo "==> Go $GO_VERSION already installed"
fi

export PATH="/usr/local/go/bin:$PATH"
go version

echo "==> cloning repo to $INSTALL_DIR"
if [[ -d "$INSTALL_DIR/.git" ]]; then
  echo "  repo already exists, pulling"
  sudo -u "$NIK_USER" git -C "$INSTALL_DIR" fetch origin "$BRANCH"
  sudo -u "$NIK_USER" git -C "$INSTALL_DIR" checkout "$BRANCH"
  sudo -u "$NIK_USER" git -C "$INSTALL_DIR" merge --ff-only "origin/$BRANCH"
else
  mkdir -p "$INSTALL_DIR"
  chown "$NIK_USER:$NIK_USER" "$INSTALL_DIR"
  sudo -u "$NIK_USER" git clone --branch "$BRANCH" "$REPO" "$INSTALL_DIR"
fi

echo "==> checking Go version from repo"
if [[ -f "$INSTALL_DIR/go.mod" ]]; then
  REPO_GO=$(grep '^go ' "$INSTALL_DIR/go.mod" | awk '{print $2}')
  if [[ "$REPO_GO" != "$GO_VERSION" ]]; then
    echo "  repo needs go $REPO_GO, reinstalling"
    GO_TAR="go${REPO_GO}.linux-${GO_ARCH}.tar.gz"
    curl -fsSL "https://go.dev/dl/$GO_TAR" -o "/tmp/$GO_TAR"
    rm -rf /usr/local/go
    tar -C /usr/local -xzf "/tmp/$GO_TAR"
    rm "/tmp/$GO_TAR"
    go version
  fi
fi

echo "==> building nik binary"
cd "$INSTALL_DIR"
sudo -u "$NIK_USER" CGO_ENABLED=1 /usr/local/go/bin/go build -o nik ./cmd/nik/

echo "==> setting up workspace"
WORKSPACE="$NIK_HOME/workspace"

if [[ ! -d "$WORKSPACE" || ! -f "$WORKSPACE/config.yaml" ]]; then
  FOUND_WS=""
  for candidate in "$WORKSPACE_SRC" "./workspace" "$SUDO_HOME/workspace"; do
    if [[ -n "$candidate" && -d "$candidate" && -f "$candidate/config.yaml" ]]; then
      FOUND_WS="$candidate"
      break
    fi
  done

  if [[ -n "$FOUND_WS" ]]; then
    echo "  copying workspace from $FOUND_WS"
    mkdir -p "$WORKSPACE"
    cp -a "$FOUND_WS"/. "$WORKSPACE"/
    chown -R "$NIK_USER:$NIK_USER" "$WORKSPACE"
    chmod 600 "$WORKSPACE/config.yaml"
  else
    echo ""
    echo "  WARNING: no workspace found."
    echo "  re-run with --workspace <path>, or place it next to install.sh."
    echo "  the workspace must contain at least config.yaml."
    echo ""
  fi
fi

sudo -u "$NIK_USER" mkdir -p "$WORKSPACE"/{media,skills,debug,journal,dreams,soul,briefings,backups}

echo "==> patching config.yaml paths for deploy layout"
if [[ -f "$WORKSPACE/config.yaml" ]]; then
  sed -i 's|prompts_dir:.*|prompts_dir: ../git/prompts|' "$WORKSPACE/config.yaml"
  sed -i 's|skills_dir:.*|skills_dir: ../git/skills|' "$WORKSPACE/config.yaml"
fi

echo "==> installing systemd units"
cp "$INSTALL_DIR/deploy/nik.service" /etc/systemd/system/
cp "$INSTALL_DIR/deploy/nik-update.service" /etc/systemd/system/
cp "$INSTALL_DIR/deploy/nik-update.timer" /etc/systemd/system/
chmod +x "$INSTALL_DIR/deploy/update.sh"
systemctl daemon-reload
systemctl enable nik.service
systemctl enable nik-update.timer

echo "==> WhatsApp pairing"
echo "  nik will start interactively for QR code pairing."
echo "  scan the QR code with WhatsApp, then press Ctrl+C."
echo ""
echo "  press Enter to continue (or Ctrl+C to skip and pair later)..."
read -r

sudo -u "$NIK_USER" "$INSTALL_DIR/nik" --home "$WORKSPACE" -force-wapp-link || true

echo "==> starting services"
systemctl start nik.service
systemctl start nik-update.timer

echo ""
echo "done. nik is running on $(hostname)."
echo ""
echo "useful commands:"
echo "  journalctl -u nik -f              # tail logs"
echo "  systemctl status nik              # check status"
echo "  systemctl status nik-update # check update timer"
echo "  journalctl -u nik-update          # update history"
