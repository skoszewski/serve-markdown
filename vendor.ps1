#!/usr/bin/env pwsh

<#
.SYNOPSIS
Download the browser-side libraries served from inside serve-markdown.

.PARAMETER Action
What to do: fetch the libraries into assets/vendor, or list them with the URLs
they come from.

.EXAMPLE
./vendor.ps1 fetch
#>

param(
    [ValidateSet('fetch', 'list')]
    [string]$Action = 'fetch'
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

# The versions the page loads, from the binary and, with --online, from the same CDNs.
$markedVersion = '15'
$dompurifyVersion = '3'
$highlightVersion = '11.9.0'
$markdownCssVersion = '5'
$mermaidVersion = '12.0.0'

$directory = Join-Path $PSScriptRoot 'assets/vendor'

$libraries = [ordered]@{
    'marked.min.js'           = "https://cdn.jsdelivr.net/npm/marked@$markedVersion/marked.min.js"
    'purify.min.js'           = "https://cdn.jsdelivr.net/npm/dompurify@$dompurifyVersion/dist/purify.min.js"
    'highlight.min.js'        = "https://cdnjs.cloudflare.com/ajax/libs/highlight.js/$highlightVersion/highlight.min.js"
    'github.min.css'          = "https://cdnjs.cloudflare.com/ajax/libs/highlight.js/$highlightVersion/styles/github.min.css"
    'github-dark.min.css'     = "https://cdnjs.cloudflare.com/ajax/libs/highlight.js/$highlightVersion/styles/github-dark.min.css"
    'github-markdown.min.css' = "https://cdn.jsdelivr.net/npm/github-markdown-css@$markdownCssVersion/github-markdown.min.css"
    'mermaid.min.js'          = "https://cdn.jsdelivr.net/npm/mermaid@$mermaidVersion/dist/mermaid.min.js"
}

switch ($Action) {
    'fetch' {
        foreach ($name in $libraries.Keys) {
            Write-Host "fetching $name"
            Invoke-WebRequest -Uri $libraries[$name] -OutFile (Join-Path $directory $name)
        }
        Write-Host "written to $directory; update assets/vendor/LICENSES.md when a version changes"
    }
    'list' {
        foreach ($name in $libraries.Keys) {
            "{0}`t{1}" -f $name, $libraries[$name]
        }
    }
}
