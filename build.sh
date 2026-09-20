#!/usr/bin/env bash

# Build, test and check serve-markdown.
# Usage: ./build.sh [build|test|check|clean] [<goos>/<goarch>]
#
# The platform defaults to the host's. Naming one builds for it instead, writing
# serve-markdown-<goos>-<goarch>; it applies to the build action alone, since the
# tests run on the host.

set -euo pipefail

binary="serve-markdown"
export CGO_ENABLED=0

action="${1:-build}"
platform="${2:-}"

if [ -n "$platform" ]; then
    goos="${platform%%/*}"
    goarch="${platform##*/}"
    if [ "$goos" = "$platform" ] || [ -z "$goos" ] || [ -z "$goarch" ]; then
        echo "platform must be given as <goos>/<goarch>, e.g. linux/amd64" >&2
        exit 2
    fi
    export GOOS="$goos"
    export GOARCH="$goarch"
    binary="serve-markdown-$goos-$goarch"
    if [ "$goos" = "windows" ]; then
        binary="$binary.exe"
    fi
fi

case "$action" in
    build)
        go build -o "$binary" .
        ;;
    test)
        go test ./...
        ;;
    check)
        unformatted="$(gofmt -l .)"
        if [ -n "$unformatted" ]; then
            printf 'these files are not gofmt clean:\n%s\n' "$unformatted" >&2
            exit 1
        fi
        go vet ./...
        go test ./...
        ;;
    clean)
        rm -f serve-markdown serve-markdown.exe serve-markdown-*
        ;;
    *)
        echo "usage: $0 [build|test|check|clean] [<goos>/<goarch>]" >&2
        exit 2
        ;;
esac
