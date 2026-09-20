#!/usr/bin/env pwsh

<#
.SYNOPSIS
Build, test and check serve-markdown.

.PARAMETER Action
What to do: build the executable, run the tests, check formatting, vetting and
tests together, or remove the executables.

.PARAMETER Platform
The destination platform as <goos>/<goarch>, e.g. linux/amd64. Defaults to the
host's, and applies to the build action alone, since the tests run on the host.
Naming one writes serve-markdown-<goos>-<goarch> instead.

.EXAMPLE
./build.ps1 check

.EXAMPLE
./build.ps1 build windows/arm64
#>

param(
    [ValidateSet('build', 'test', 'check', 'clean')]
    [string]$Action = 'build',

    [ValidatePattern('^[a-z0-9]+/[a-z0-9]+$')]
    [string]$Platform
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'
# A native command exiting non-zero raises an error rather than being ignored.
$PSNativeCommandUseErrorActionPreference = $true

$env:CGO_ENABLED = '0'
$binary = if ($IsWindows) { 'serve-markdown.exe' } else { 'serve-markdown' }

if ($Platform) {
    $goos, $goarch = $Platform -split '/'
    $env:GOOS = $goos
    $env:GOARCH = $goarch
    $binary = "serve-markdown-$goos-$goarch"
    if ($goos -eq 'windows') {
        $binary += '.exe'
    }
}

switch ($Action) {
    'build' {
        go build -o $binary .
    }
    'test' {
        go test ./...
    }
    'check' {
        $unformatted = gofmt -l .
        if ($unformatted) {
            throw "these files are not gofmt clean:`n$($unformatted -join [Environment]::NewLine)"
        }
        go vet ./...
        go test ./...
    }
    'clean' {
        Remove-Item -Path 'serve-markdown', 'serve-markdown.exe', 'serve-markdown-*' -Force -ErrorAction SilentlyContinue
    }
}
