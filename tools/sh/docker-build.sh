#!/bin/bash
# Build the Docker image.
#
# Usage:
#   ./tools/sh/docker-build.sh
#
# Examples:
#   ./tools/sh/docker-build.sh                                          # native platform
#   DOCKER_PLATFORM=linux/amd64 ./tools/sh/docker-build.sh             # cross-platform
#   DOCKER_IMAGE=wile-ext-dev ./tools/sh/docker-build.sh               # custom image name
#
# Environment variables:
#   DOCKER_IMAGE      image name (default: wile-goast)
#   DOCKER_PLATFORM   target platform (e.g., linux/amd64, linux/arm64)

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
IMAGE="${DOCKER_IMAGE:-wile-goast}"

build_args=(build -f "$REPO_ROOT/docker/Dockerfile" -t "$IMAGE")
if [ -n "${DOCKER_PLATFORM:-}" ]; then
    build_args+=(--platform "$DOCKER_PLATFORM")
fi
build_args+=("$REPO_ROOT")

docker "${build_args[@]}"
