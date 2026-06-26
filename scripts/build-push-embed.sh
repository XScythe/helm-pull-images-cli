#!/usr/bin/env bash
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

GOOS_VALUE="${GOOS:-$(go env GOOS)}"
GOARCH_VALUE="${GOARCH:-$(go env GOARCH)}"
VERSION_VALUE="${VERSION:-dev}"
OUTPUT_PATH="${REPO_ROOT}/internal/pushbin/push_images_${GOOS_VALUE}_${GOARCH_VALUE}.bin"

echo "building embedded push binary for ${GOOS_VALUE}/${GOARCH_VALUE} -> ${OUTPUT_PATH}"

CGO_ENABLED=0 GOOS="${GOOS_VALUE}" GOARCH="${GOARCH_VALUE}" \
  go build -trimpath \
  -ldflags="-s -w -X helm-deep-pack/cmd/pushimages.version=${VERSION_VALUE}" \
  -o "${OUTPUT_PATH}" \
  ./cmd/pushimages

