#!/usr/bin/env bash

# Download the browser-side libraries served from inside serve-markdown.
# Usage: ./vendor.sh [fetch|list]
#
# The versions below are the ones the page loads, from the binary and, with
# --online, from the same CDNs.

set -euo pipefail

marked_version="15"
dompurify_version="3"
highlight_version="11.9.0"
markdown_css_version="5"
mermaid_version="12.0.0"

directory="$(dirname "$0")/assets/vendor"

# Each entry is "<file name> <URL>".
libraries=(
    "marked.min.js https://cdn.jsdelivr.net/npm/marked@${marked_version}/marked.min.js"
    "purify.min.js https://cdn.jsdelivr.net/npm/dompurify@${dompurify_version}/dist/purify.min.js"
    "highlight.min.js https://cdnjs.cloudflare.com/ajax/libs/highlight.js/${highlight_version}/highlight.min.js"
    "github.min.css https://cdnjs.cloudflare.com/ajax/libs/highlight.js/${highlight_version}/styles/github.min.css"
    "github-dark.min.css https://cdnjs.cloudflare.com/ajax/libs/highlight.js/${highlight_version}/styles/github-dark.min.css"
    "github-markdown.min.css https://cdn.jsdelivr.net/npm/github-markdown-css@${markdown_css_version}/github-markdown.min.css"
    "mermaid.min.js https://cdn.jsdelivr.net/npm/mermaid@${mermaid_version}/dist/mermaid.min.js"
)

action="${1:-fetch}"

case "$action" in
    fetch)
        for library in "${libraries[@]}"; do
            name="${library%% *}"
            url="${library#* }"
            printf 'fetching %s\n' "$name"
            curl -fsSL --retry 3 -o "$directory/$name" "$url"
        done
        printf 'written to %s; update assets/vendor/LICENSES.md when a version changes\n' "$directory"
        ;;
    list)
        for library in "${libraries[@]}"; do
            printf '%s\t%s\n' "${library%% *}" "${library#* }"
        done
        ;;
    *)
        echo "usage: $0 [fetch|list]" >&2
        exit 2
        ;;
esac
