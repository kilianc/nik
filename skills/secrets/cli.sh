#!/bin/sh
set -eu
case "${1:-}" in
  read)   nik secrets read "$2" ;;
  list)   nik secrets list ;;
  write)  printf '%s' "$3" | nik secrets write "$2" ;;
  delete) nik secrets delete "$2" ;;
  *)      echo "usage: $0 {read|list|write|delete}" >&2; exit 64 ;;
esac
