#!/usr/bin/env bash
set -e

cd ~/git

BRANCH="${NIK_BRANCH:-main}"
git fetch origin "$BRANCH"

LOCAL=$(git rev-parse HEAD)
REMOTE=$(git rev-parse "origin/$BRANCH")

if [ "$LOCAL" = "$REMOTE" ]; then
    echo "up to date"
    exit 0
fi

echo "updating $LOCAL -> $REMOTE"
git merge --ff-only "origin/$BRANCH"

CGO_ENABLED=1 go build -o nik.new ./cmd/nik/
mv nik.new nik

systemctl restart nik
echo "restarted with $(git rev-parse --short HEAD)"
