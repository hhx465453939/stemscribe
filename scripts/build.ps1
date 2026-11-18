<#
.SYNOPSIS
    Builds the static frontend and the Windows binaries into dist/.

.DESCRIPTION
    Produces the artifacts a release would ship:
      frontend/out/         static UI bundle
      dist/Stemscribe.exe   the desktop client (native WebView2 window)
      dist/stemscribe.exe   the CLI for automation

    The desktop client is the default output because that is what a user
    double-clicks. It needs cgo (gcc) plus the WebView2 runtime; pass
    -SkipDesktop to build only the CLI on a machine without a C toolchain.
#>
[CmdletBinding()]
param(
    [string]$Version = '0.0.1-dev',
    [switch]$SkipFrontend,
    [switch]$SkipDesktop
)

$ErrorActionPreference = 'Stop'
$repo = Split-Path -Parent $PSScriptRoot
$dist = Join-Path $repo 'dist'
New-Item -ItemType Directory -Force -Path $dist | Out-Null

if (-not $SkipFrontend) {
    Write-Host '== frontend: static export ==' -ForegroundColor Cyan
    Push-Location (Join-Path $repo 'frontend')
    try {
        pnpm install --frozen-lockfile
        if ($LASTEXITCODE -ne 0) { throw 'pnpm install failed' }
        pnpm run build
        if ($LASTEXITCODE -ne 0) { throw 'pnpm build failed' }
    } finally {
        Pop-Location
    }
    if (-not (Test-Path (Join-Path $repo 'frontend\out\index.html'))) {
        throw 'frontend/out/index.html missing after build'
    }
}

Write-Host '== embed UI into the binary ==' -ForegroundColor Cyan
$assets = Join-Path $repo 'internal\webui\assets'
if (Test-Path $assets) {
    Get-ChildItem -LiteralPath $assets -Recurse -File |
        Where-Object { $_.Name -ne 'placeholder.txt' } |
        Remove-Item -Force
}
New-Item -ItemType Directory -Force -Path $assets | Out-Null
if (-not $SkipFrontend) {
    Copy-Item -Path (Join-Path $repo 'frontend\out\*') -Destination $assets -Recurse -Force
}
Write-Host '== go: build CLI (pure Go) ==' -ForegroundColor Cyan
Push-Location $repo
try {
    $env:CGO_ENABLED = '0'
    go build -trimpath `
        -ldflags "-s -w -X main.version=$Version" `
        -o (Join-Path $dist 'stemscribe.exe') ./cmd/stemscribe
    if ($LASTEXITCODE -ne 0) { throw 'go build (cli) failed' }
} finally {
    Pop-Location
}

if (-not $SkipDesktop) {
    Write-Host '== go: build desktop client (cgo + WebView2) ==' -ForegroundColor Cyan
    if (-not (Get-Command gcc -ErrorAction SilentlyContinue)) {
        throw 'gcc not found: the desktop client needs a C toolchain. Install MSYS2, or pass -SkipDesktop.'
    }
    Push-Location $repo
    try {
        $env:CGO_ENABLED = '1'
        # -H windowsgui keeps double-click from flashing a console window.
        go build -trimpath -tags 'gui desktop production' `
            -ldflags "-H windowsgui -s -w -X main.version=$Version" `
            -o (Join-Path $dist 'Stemscribe.exe') ./cmd/stemscribe-gui
        if ($LASTEXITCODE -ne 0) { throw 'go build (desktop) failed' }
    } finally {
        Pop-Location
    }
}

$cli = Join-Path $dist 'stemscribe.exe'
$sizeMb = [math]::Round((Get-Item $cli).Length / 1MB, 2)
Write-Host ("`nbuilt {0} ({1} MB)" -f $cli, $sizeMb) -ForegroundColor Green

if (-not $SkipDesktop) {
    $gui = Join-Path $dist 'Stemscribe.exe'
    $guiMb = [math]::Round((Get-Item $gui).Length / 1MB, 2)
    Write-Host ("built {0} ({1} MB)  <- double-click this one" -f $gui, $guiMb) -ForegroundColor Green
}
