<#
.SYNOPSIS
    Builds the launcher and drops it into the game folder.

.DESCRIPTION
    Reads the game path from .local.settings, which is not committed: everybody
    has Sacred Gold somewhere else, and a path in a tracked file is a merge
    conflict waiting to happen.

        sacred=D:\SteamLibrary\steamapps\common\Sacred Gold

.EXAMPLE
    pwsh tools/install.ps1
#>
[CmdletBinding()]
param([switch]$Bump)

$ErrorActionPreference = 'Stop'
$root = Split-Path -Parent $PSScriptRoot
$settings = Join-Path $root '.local.settings'

if (-not (Test-Path $settings)) {
    throw "No .local.settings file found. Create it with a line like:`n" +
          "    sacred=D:\SteamLibrary\steamapps\common\Sacred Gold"
}
$sacred = (Get-Content $settings |
    Where-Object { $_ -match '^\s*sacred\s*=' } |
    Select-Object -First 1) -replace '^\s*sacred\s*=\s*', ''
if (-not $sacred) { throw "No `sacred=` line found in .local.settings" }
$sacred = $sacred.Trim().Trim('"')
# The same three names game/exe.go looks for, in the same order. PowerShell
# cannot read that list, so this is the one place it is written twice; keep
# the two in step.
$executables = 'pureHD.exe', 'Sacred.exe', 'Game.exe'
# -Path is case-insensitive on Windows, which is the point: an install that
# spells it sacred.exe is the same install.
$game = $executables | Where-Object { Test-Path (Join-Path $sacred $_) } | Select-Object -First 1
if (-not $game) {
    throw "No game found in $sacred. Checked for $($executables -join ', '). Is this the game folder?"
}

& (Join-Path $PSScriptRoot 'build.ps1') -Bump:$Bump

Copy-Item (Join-Path $root 'dist/Sacred Mod Loader.exe') $sacred -Force
Write-Host "Installed in $sacred"
