<#
.SYNOPSIS
    Assembles the payload and builds the launcher.

.DESCRIPTION
    The launcher ships as one file, so everything it installs has to be inside
    it before `go build` runs: the host, the agent scripts and the two jars.

    Those come from the sibling checkouts when they are there, because that is
    what a full workspace looks like and because somebody changing the host
    wants their change in the build. With no siblings, the releases pinned in
    dependencies.json are downloaded instead, so a lone clone of this
    repository builds with nothing installed but Go.

.PARAMETER Bump
    Raises the patch number in .version before building.

.PARAMETER Protocol
.PARAMETER Coderpack
    Paths to those checkouts. Default to siblings; pass 'none' to force the
    download path even when a checkout is there.

.EXAMPLE
    pwsh tools/build.ps1
    pwsh tools/build.ps1 -Bump
    pwsh tools/build.ps1 -Protocol none -Coderpack none
#>
[CmdletBinding()]
param(
    [switch]$Bump,
    [string]$Protocol,
    [string]$Coderpack,
    [string]$Mappings
)

$ErrorActionPreference = 'Stop'
$root = Split-Path -Parent $PSScriptRoot
$workspace = Split-Path -Parent $root
$payload = Join-Path $root 'install/payload'

if (-not $Protocol)  { $Protocol  = Join-Path $workspace 'protocol' }
if (-not $Coderpack) { $Coderpack = Join-Path $workspace 'coderpack' }
if (-not $Mappings)  { $Mappings  = Join-Path $workspace 'mappings' }

# --- versions -------------------------------------------------------------
$versionFile = Join-Path $root '.version'
$version = (Get-Content $versionFile -Raw).Trim()
if ($Bump) {
    $parts = $version.Split('.')
    $parts[2] = [int]$parts[2] + 1
    $version = $parts -join '.'
    Set-Content -Path $versionFile -Value "$version`n" -NoNewline
}
Write-Host "Sacred Mod Loader $version"

$pinned = @{}
Get-Content (Join-Path $root 'dependencies.json') -Raw |
    ConvertFrom-Json |
    ForEach-Object { $pinned[$_.path -replace '^ancaria-dev/', ''] = $_.version }

function Need($path, $what) {
    if (-not (Test-Path $path)) { throw "Could not find $what at $path" }
    $path
}

function Fetch($repo, $file, $into) {
    $tag = $pinned[$repo]
    if (-not $tag) { throw "No version is pinned for $repo in dependencies.json" }
    $url = "https://github.com/ancaria-dev/$repo/releases/download/v$tag/$file"
    Write-Host "      $url"
    try {
        # No token: these are public releases, and needing one to build a
        # launcher would defeat the point of the download path.
        Invoke-WebRequest -Uri $url -OutFile $into -UseBasicParsing -ErrorAction Stop
    }
    catch {
        # Without this the failure arrives as GitHub's whole 404 page pasted
        # into the console, which buries the one line that matters.
        $code = $_.Exception.Response.StatusCode.value__
        $why = if ($code -eq 404) {
            "there is no v$tag release of $repo with a $file asset yet"
        } else {
            "HTTP $code"
        }
        throw "Could not download $file from $repo v${tag}: $why.`n" +
              "Clone https://github.com/ancaria-dev/$repo.git beside this " +
              "repository or pin a released version in dependencies.json."
    }
}

function FromSource($path, $marker) {
    $path -ne 'none' -and (Test-Path (Join-Path $path $marker))
}

# --- payload --------------------------------------------------------------
Remove-Item -Recurse -Force $payload -ErrorAction SilentlyContinue
New-Item -ItemType Directory -Path $payload, "$payload/agent" | Out-Null
Set-Content -Path "$payload/.gitkeep" -Value '' -NoNewline
Set-Content -Path "$payload/VERSION" -Value "$version`n" -NoNewline

# protocol.exe
if (FromSource $Protocol 'Cargo.toml') {
    Write-Host '[1/4] protocol (cargo)'
    Push-Location $Protocol
    try { cargo build --release; if ($LASTEXITCODE) { throw 'Cargo build failed' } }
    finally { Pop-Location }
    Copy-Item (Need "$Protocol/target/release/protocol.exe" 'protocol.exe') $payload
} else {
    Write-Host "[1/4] protocol (release $($pinned['protocol']))"
    Fetch 'protocol' 'protocol.exe' "$payload/protocol.exe"
}

# api.jar, zygote.jar and the agent, address table included
if (FromSource $Coderpack 'settings.gradle.kts') {
    Write-Host '[2/4] coderpack (gradle)'
    Push-Location $Coderpack
    try {
        & ./gradlew.bat --console=plain jar
        if ($LASTEXITCODE) { throw 'Gradle build failed' }
        # The address table is generated, never checked in: a stale copy is a
        # hook that silently never fires. The path is passed only when there is
        # one -- given no argument, addr.py walks its own chain and ends up
        # downloading the registry, which is the whole point of that chain.
        if (Test-Path (Join-Path $Mappings 'mappings.json')) {
            python tools/addr.py $Mappings
        } else {
            python tools/addr.py
        }
        if ($LASTEXITCODE) { throw 'Address generation failed' }
    }
    finally { Pop-Location }
    foreach ($part in 'api', 'zygote') {
        $jar = Get-ChildItem "$Coderpack/$part/build/libs/$part-*.jar" |
               Where-Object { $_.Name -notmatch '-(sources|javadoc)\.jar$' } |
               Select-Object -First 1
        Copy-Item (Need $jar.FullName "$part.jar") "$payload/$part.jar"
    }
    Write-Host '[3/4] agent'
    Copy-Item -Recurse "$Coderpack/agent/src/*" "$payload/agent"
} else {
    Write-Host "[2/4] coderpack (release $($pinned['coderpack']))"
    Fetch 'coderpack' 'api.jar' "$payload/api.jar"
    Fetch 'coderpack' 'zygote.jar' "$payload/zygote.jar"
    Write-Host '[3/4] agent'
    $zip = Join-Path ([IO.Path]::GetTempPath()) 'coderpack-agent.zip'
    Fetch 'coderpack' 'agent.zip' $zip
    Expand-Archive -Path $zip -DestinationPath "$payload/agent" -Force
    Remove-Item $zip
}
# Either way the table has to be in there, or every hook lands on the wrong
# address and nothing says so.
Need "$payload/agent/gen/addr.js" 'the generated address table' | Out-Null

# No mods are staged, deliberately. They come from a repository the launcher
# reads at run time, so a player picks the ones they want instead of finding
# somebody else's choices already installed -- and the launcher stops growing by
# nine megabytes every time a mod packs a UI toolkit. The empty mods/ directory
# stays in the payload: unpacking it is what creates <game>/mods.
#
# For a mod you are working on, `gradlew installSacredMod -PsacredDir=...` in
# its own checkout puts the jar straight in that folder.

$size = (Get-ChildItem $payload -Recurse -File | Measure-Object Length -Sum).Sum
Write-Host ("    {0} MB staged" -f [int]($size / 1MB))

# --- launcher -------------------------------------------------------------
Write-Host '[4/4] launcher (go)'
$out = Join-Path $root 'dist/Sacred Mod Loader.exe'
New-Item -ItemType Directory -Force -Path (Split-Path -Parent $out) | Out-Null
Push-Location $root
try {
    # The icon and the version block, from the checked-in .ico and .version.
    # Generated rather than tracked for the same reason the payload is, and
    # generated by Go rather than by go-winres or rsrc so the download path
    # still needs nothing but a Go toolchain. `go build` links whatever .syso
    # is sitting in the module root.
    go run ./tools/rsrc
    if ($LASTEXITCODE) { throw 'Resource generation failed' }
    go build -ldflags '-H=windowsgui' -o $out .
    if ($LASTEXITCODE) { throw 'Go build failed' }
}
finally { Pop-Location }
Write-Host "    $out"
