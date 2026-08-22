# Pinned by digest: this file is the shell tool's toolchain lockfile, and a
# floating tag means the image every nik command runs in can change under a
# rebuild that touched nothing. The tag is carried alongside for readability —
# the digest is what resolves, and it is the multi-arch index, so this still
# builds on both arm64 and amd64.
#
# Bookworm ships tmux 3.3a, which loses a pane's exit status outright when the
# pane's command has run a tmux client — see getExitCode in tmux.go. nik no
# longer depends on that status, so this is safe to sit on; it is a reason to
# know what the base is, not a reason to chase the newest one.
FROM golang:1.24.13-bookworm@sha256:1a6d4452c65dea36aac2e2d606b01b4a029ec90cc1ae53890540ce6173ea77ac

ENV HOME=/root

RUN apt-get update

RUN apt-get install -y --no-install-recommends ca-certificates
RUN apt-get install -y --no-install-recommends openssl
RUN apt-get install -y --no-install-recommends curl
RUN apt-get install -y --no-install-recommends jq
RUN apt-get install -y --no-install-recommends nodejs npm
RUN apt-get install -y --no-install-recommends sqlite3
RUN apt-get install -y --no-install-recommends tmux

WORKDIR /workspace

CMD ["sleep", "infinity"]
