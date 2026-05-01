#!/usr/bin/env bash
# Check that vendorHashes in default.nix are equal to vendor/.
set -euo pipefail

if ! command -v nix &>/dev/null; then
  echo "check-vendor-hash: 'nix' not found, verification skipped" >&2
  exit 0
fi

echo "check-vendor-hash: updating vendor/..."
go mod vendor

ACTUAL=$(nix hash path --sri --type sha256 vendor/)

# Extracts any line containing vendorHash et makes the check
FAILED=0
while IFS= read -r line; do
  EXPECTED=$(echo "$line" | sed 's/.*vendorHash = "\(.*\)".*/\1/')
  if [ "$ACTUAL" != "$EXPECTED" ]; then
    echo "check-vendor-hash: obsolete vendorHash in default.nix" >&2
    echo "  Expected:  $EXPECTED" >&2
    echo "  Generated: $ACTUAL" >&2
    FAILED=1
  fi
done < <(grep 'vendorHash' default.nix)

if [ "$FAILED" -eq 1 ]; then
  echo "" >&2
  echo "Please update default.nix with:" >&2
  echo "  vendorHash = \"$ACTUAL\";" >&2
  exit 1
fi

echo "check-vendor-hash: OK ($ACTUAL)"
