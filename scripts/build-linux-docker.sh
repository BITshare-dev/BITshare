#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
IMAGE_NAME="openshare-linux-builder"
DOCKERFILE_PATH="$ROOT_DIR/docker/build-linux.Dockerfile"

require_command() {
  local command_name="$1"
  if ! command -v "$command_name" >/dev/null 2>&1; then
    echo "missing required command: $command_name" >&2
    exit 1
  fi
}

build_image() {
  echo "building docker image ..."
  docker build -t "$IMAGE_NAME" -f "$DOCKERFILE_PATH" "$ROOT_DIR"
}

run_build() {
  echo "running linux build in docker ..."
  docker run --rm \
    --user "$(id -u):$(id -g)" \
    -v "$ROOT_DIR:/workspace" \
    -w /workspace \
    "$IMAGE_NAME" \
    bash ./scripts/build-linux.sh
}

main() {
  require_command docker
  build_image
  run_build
}

main "$@"
