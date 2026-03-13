#!/bin/bash
# Open an interactive shell inside the Docker container.
#
# The image must be built first (make docker-build).
#
# Usage:
#   ./tools/sh/docker-shell.sh [shell]
#
# Examples:
#   ./tools/sh/docker-shell.sh              # bash (default)
#   ./tools/sh/docker-shell.sh /bin/sh      # sh
#
# Environment variables:
#   DOCKER_IMAGE   image name (default: wile-extension-example)

set -euo pipefail

IMAGE="${DOCKER_IMAGE:-wile-extension-example}"
SHELL_CMD="${1:-/bin/bash}"

docker run --rm -it "$IMAGE" "$SHELL_CMD"
