FROM golang:1.25.4

RUN \
  apt-get update && \
  apt-get install -y --no-install-recommends \
    curl \
    docker.io \
    jq \
    tmux \
  && rm -rf /var/lib/apt/lists/*

WORKDIR /app/workspace

ENTRYPOINT ["sh", "-c", "CGO_ENABLED=1 CGO_CFLAGS=-w go build -o /tmp/nik ../cmd/nik/ && exec /tmp/nik"]
