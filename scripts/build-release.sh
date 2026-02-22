#!/usr/bin/env bash
set -euo pipefail

VERSION="${1:?Usage: build-release.sh <version>}"

echo "Building sarge v${VERSION}..."

# GoReleaser expects a git tag for version injection via ldflags.
# Create a lightweight tag so GoReleaser resolves {{.Version}} correctly.
# semantic-release will create the annotated tag after this step.
git tag -f "v${VERSION}"

goreleaser release --clean --skip=publish,announce

echo "Build complete. Artifacts in dist/"
