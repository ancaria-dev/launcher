<#
.SYNOPSIS
    Prints or sets the launcher's own version.

.DESCRIPTION
    .version is the only place this number lives. It is not the same thing as
    dependencies.json, which pins the versions of protocol and coderpack this
    launcher builds against.  Bumping this repository's own version says
    nothing about those, and this script does not touch that file.

.EXAMPLE
    pwsh tools/version.ps1
    pwsh tools/version.ps1 0.99.1
#>
param(
    [string]$Version
)

$ErrorActionPreference = 'Stop'
$root = Split-Path -Parent $PSScriptRoot
$versionPath = Join-Path $root '.version'
$current = (Get-Content -Path $versionPath -Raw).Trim()

if (-not $Version) {
    Write-Host $current
    return
}

Set-Content -Path $versionPath -Value "$Version`n" -NoNewline
Write-Host "$current -> $Version"
