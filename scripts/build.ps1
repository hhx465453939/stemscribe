<#
.SYNOPSIS
    Builds the static frontend and the Windows binary into dist/.

.DESCRIPTION
    Produces the artifacts a release would ship:
      frontend/out/        static UI bundle
      dist/stemscribe.exe  single Windows binary

    The Wails shell (which embeds frontend/out) is the remaining CHG-0001 item;
    until it lands this produces the CLI binary plus the exportable UI.
#>
[CmdletBinding()]
param(
    [string]$Version = '0.0.1-dev',
    [switch]$SkipFrontend
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
Write-Host '== go: build windows/amd64 ==' -ForegroundColor Cyan
Push-Location $repo
try {
    $env:CGO_ENABLED = '0'
    go build -trimpath `
        -ldflags "-s -w -X main.version=$Version" `
        -o (Join-Path $dist 'stemscribe.exe') ./cmd/stemscribe
    if ($LASTEXITCODE -ne 0) { throw 'go build failed' }
} finally {
    Pop-Location
}

$exe = Join-Path $dist 'stemscribe.exe'
$sizeMb = [math]::Round((Get-Item $exe).Length / 1MB, 2)
Write-Host ("`nbuilt {0} ({1} MB)" -f $exe, $sizeMb) -ForegroundColor Green
