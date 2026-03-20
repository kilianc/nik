FROM golang:1.24-bookworm
RUN apt-get update \
  && apt-get install -y --no-install-recommends \
    curl \
    jq \
    nodejs \
    npm \
    sqlite3 \
    tmux \
  && rm -rf /var/lib/apt/lists/*
WORKDIR /workspace
CMD ["sleep", "infinity"]
