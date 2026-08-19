#!/bin/sh
set -eu

script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
image=ghcr.io/example/project

edge=$($script_dir/container-tags.sh refs/heads/main "$image")
[ "$edge" = "$image:edge" ]

release=$($script_dir/container-tags.sh refs/tags/v1.2.3 "$image")
expected=$(printf '%s:v1.2.3\n%s:1.2\n%s:1\n%s:latest' "$image" "$image" "$image" "$image")
[ "$release" = "$expected" ]

if $script_dir/container-tags.sh refs/tags/v1.2 >/dev/null 2>&1; then
  echo "invalid SemVer tag was accepted" >&2
  exit 1
fi
