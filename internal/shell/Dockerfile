FROM golang:1.24-bookworm
RUN apt-get update && apt-get install -y --no-install-recommends tmux sqlite3 curl jq && rm -rf /var/lib/apt/lists/*
WORKDIR /workspace
CMD ["sleep", "infinity"]
