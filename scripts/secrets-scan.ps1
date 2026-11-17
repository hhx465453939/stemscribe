<#
.SYNOPSIS
    Scans a work directory (and the repo) for plaintext credentials (AC-13).

.DESCRIPTION
    Secrets must live in config/secrets.bin as DPAPI ciphertext. This script looks
    for anything that still resembles a live credential on disk or in logs.
#>
[CmdletBinding()]
param(
    [string]$Workdir = (Join-Path $env:USERPROFILE 'Stemscribe'),
    [switch]$IncludeRepo
)

$ErrorActionPreference = 'Stop'
$patterns = @(
    'sk-[A-Za-z0-9]{16,}',
    'Bearer\s+[A-Za-z0-9._-]{20,}',
    'AKIA[0-9A-Z]{16}',
    'BEGIN (RSA|OPENSSH|EC|PGP) PRIVATE KEY'
)

$targets = @()
if (Test-Path -LiteralPath $Workdir) {
    $targets += @(
        (Join-Path $Workdir 'config'),
        (Join-Path $Workdir 'logs')
    ) | Where-Object { Test-Path -LiteralPath $_ }
} else {
    Write-Host "note: workdir not found, skipping: $Workdir" -ForegroundColor DarkGray
}

if ($IncludeRepo) {
    $repoRoot = Split-Path -Parent $PSScriptRoot
    $targets += $repoRoot
}

if ($targets.Count -eq 0) {
    Write-Host 'secrets scan: nothing to scan' -ForegroundColor DarkGray
    exit 0
}

$hits = 0
foreach ($target in $targets) {
    $files = Get-ChildItem -LiteralPath $target -Recurse -File -ErrorAction SilentlyContinue |
        Where-Object {
            $_.FullName -notmatch '\\(node_modules|\.git|\.next|out)\\'
        }
    foreach ($file in $files) {
        # secrets.bin is expected to be binary ciphertext; never match against it.
        if ($file.Name -eq 'secrets.bin') { continue }
        try {
            $content = Get-Content -LiteralPath $file.FullName -Raw -ErrorAction Stop
        } catch {
            continue
        }
        if ([string]::IsNullOrEmpty($content)) { continue }
        foreach ($p in $patterns) {
            if ($content -match $p) {
                Write-Host ("HIT {0} :: {1}" -f $p, $file.FullName) -ForegroundColor Red
                $hits++
            }
        }
    }
}

if ($hits -gt 0) {
    Write-Host ("secrets scan FAILED: {0} hit(s)" -f $hits) -ForegroundColor Red
    exit 1
}
Write-Host 'secrets scan passed (0 hits)' -ForegroundColor Green
exit 0
