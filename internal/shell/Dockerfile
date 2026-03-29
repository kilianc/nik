FROM golang:1.24-bookworm

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
