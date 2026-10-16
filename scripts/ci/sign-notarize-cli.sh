#!/usr/bin/env bash
set -euo pipefail

binary="${1:?Usage: sign-notarize-cli.sh <binary>}"
: "${APPLE_PASSWORD:?APPLE_PASSWORD is required for notarization}"

# Reuse the Developer ID Application certificate imported by the release job.
identity="$(security find-identity -v -p codesigning | awk -F '"' '/Developer ID Application/ {print $2; exit}')"
if [[ -z "$identity" ]]; then
  echo "No Developer ID Application certificate found in keychain" >&2
  exit 1
fi
team_id="$(printf '%s' "$identity" | sed -nE 's/.*\(([A-Z0-9]{10})\)$/\1/p')"
if [[ -z "$team_id" ]]; then
  echo "Could not determine Apple Team ID from signing identity" >&2
  exit 1
fi

codesign --force --options runtime --timestamp \
  --identifier com.mertcikla.tld.cli --sign "$identity" "$binary"
codesign --verify --strict --verbose=2 "$binary"

tmp_dir="$(mktemp -d)"
trap 'rm -rf "$tmp_dir"' EXIT
ditto -c -k --keepParent "$binary" "$tmp_dir/tld.zip"

notary_args=(
  --apple-id mertcikla@gmail.com
  --password "$APPLE_PASSWORD"
  --team-id "$team_id"
)

# A submission alone is insufficient: fail the release unless Apple accepts it.
notary_failed=false
if ! xcrun notarytool submit "$tmp_dir/tld.zip" "${notary_args[@]}" \
  --wait --timeout 30m --output-format json > "$tmp_dir/result.json"; then
  notary_failed=true
  echo "Notarization submission failed or timed out for $binary" >&2
fi

cat "$tmp_dir/result.json"
status="$(jq -r '.status // empty' "$tmp_dir/result.json")"
if [[ "$status" != "Accepted" || "$notary_failed" == true ]]; then
  submission_id="$(jq -r '.id // empty' "$tmp_dir/result.json")"
  if [[ -n "$submission_id" ]]; then
    xcrun notarytool log "$submission_id" "${notary_args[@]}" || true
  fi
  echo "Apple did not accept notarization for $binary (status: ${status:-unknown})" >&2
  exit 1
fi

# Standalone Mach-O binaries cannot have a ticket stapled to them. Gatekeeper
# retrieves Apple's ticket online using the signed binary's code directory hash.
codesign --verify --strict --verbose=2 "$binary"
