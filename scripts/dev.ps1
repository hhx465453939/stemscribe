<#
.SYNOPSIS
    Development loop: Next.js dev server + Go CLI in watch mode.

.DESCRIPTION
    Two terminals are unavoidable until the Wails shell lands (CHG-0001 remaining
    item): the UI runs under `next dev` and the backend is exercised through the
    CLI. This script opens both in one window using background jobs.
#>
[CmdletBinding()]
param(
    [string]$Workdir = (Join-Path $env:USERPROFILE 'Stemscribe')
)

$ErrorActionPreference = 'Stop'
$repo = Split-Path -Parent $PSScriptRoot

Write-Host '== frontend: next dev (http://localhost:3000) ==' -ForegroundColor Cyan
$web = Start-Job -ScriptBlock {
    param($root)
    Set-Location (Join-Path $root 'frontend')
    pnpm run dev
} -ArgumentList $repo

Write-Host '== backend: go build (CLI is the current entry point) ==' -ForegroundColor Cyan
Push-Location $repo
try {
    go build -o (Join-Path $repo 'dist\stemscribe.exe') ./cmd/stemscribe
    Write-Host ''
    Write-Host 'Try it:' -ForegroundColor Green
    Write-Host ("  .\dist\stemscribe.exe probe --workdir `"{0}`"" -f $Workdir)
    Write-Host ("  .\dist\stemscribe.exe run --input <file> --workdir `"{0}`" --duration-ms 30000" -f $Workdir)
    Write-Host ''
    Write-Host 'Press Ctrl+C to stop the dev server.' -ForegroundColor DarkGray
    Receive-Job -Job $web -Wait -AutoRemoveJob
} finally {
    Pop-Location
    Stop-Job -Job $web -ErrorAction SilentlyContinue
    Remove-Job -Job $web -Force -ErrorAction SilentlyContinue
}
