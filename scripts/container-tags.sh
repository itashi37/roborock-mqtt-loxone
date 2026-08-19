#!/bin/sh
set -eu

ref=${1:?Git ref is required}
image=${2:-ghcr.io/itashi37/roborock-mqtt-loxone}

case "$ref" in
  refs/heads/main)
    printf '%s:edge\n' "$image"
    ;;
  refs/tags/v*)
    version=${ref#refs/tags/v}
    case "$version" in
      *[!0-9.]*|*.*.*.*|.*|*.)
        printf 'Invalid SemVer release tag: v%s\n' "$version" >&2
        exit 1
        ;;
    esac
    major=${version%%.*}
    remainder=${version#*.}
    minor=${remainder%%.*}
    patch=${remainder#*.}
    if [ -z "$major" ] || [ -z "$minor" ] || [ -z "$patch" ] || [ "$patch" = "$remainder" ]; then
      printf 'Invalid SemVer release tag: v%s\n' "$version" >&2
      exit 1
    fi
    printf '%s:v%s\n%s:%s.%s\n%s:%s\n%s:latest\n' "$image" "$version" "$image" "$major" "$minor" "$image" "$major" "$image"
    ;;
  *)
    printf 'Unsupported publication ref: %s\n' "$ref" >&2
    exit 1
    ;;
esac
