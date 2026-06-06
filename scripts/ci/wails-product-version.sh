#!/usr/bin/env bash
set -euo pipefail

if [[ $# -ne 1 ]]; then
  echo "usage: $0 <release-tag>" >&2
  exit 2
fi

version="${1#v}"
product_version="${version%%[-+]*}"

if [[ ! "$product_version" =~ ^[0-9]+[.][0-9]+[.][0-9]+$ ]]; then
  echo "release tag $1 must resolve to a Wails productVersion in X.Y.Z format; got $product_version" >&2
  exit 1
fi

printf '%s\n' "$product_version"
