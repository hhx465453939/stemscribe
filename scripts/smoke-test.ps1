<#
.SYNOPSIS
    End-to-end smoke test: build, run the pipeline on a synthetic clip, assert the
    bundle contract (AC-15 preconditions).

.DESCRIPTION
    Uses the synthetic engine, so this verifies orchestration, checkpointing and
    artifact shape — not transcription quality. That boundary is deliberate: a
    green run here must never be read as "inference works" (see PIPELINE.md §8).
#>
[CmdletBinding()]
param(
    [string]$Workdir,
    [int]$DurationMs = 30000
)

$ErrorActionPreference = 'Stop'
$repo = Split-Path -Parent $PSScriptRoot

# Windows PowerShell 5.1 decodes files as ANSI by default, which mangles the
# UTF-8 transcripts this test asserts on (ConvertFrom-Json then throws). Force
# UTF-8 for both the console and file reads.
try {
    [Console]::OutputEncoding = [System.Text.Encoding]::UTF8
    $OutputEncoding = [System.Text.Encoding]::UTF8
} catch { }

if (-not $Workdir) {
    $Workdir = Join-Path ([System.IO.Path]::GetTempPath()) ("stemscribe-smoke-" + [guid]::NewGuid().ToString('N').Substring(0, 8))
}
Write-Host "workdir: $Workdir" -ForegroundColor DarkGray

$failures = New-Object System.Collections.Generic.List[string]
function Assert-True([bool]$cond, [string]$message) {
    if ($cond) {
        Write-Host "  ok   $message" -ForegroundColor Green
    } else {
        Write-Host "  FAIL $message" -ForegroundColor Red
        $script:failures.Add($message)
    }
}

Push-Location $repo
try {
    Write-Host '== 1. go build ==' -ForegroundColor Cyan
    $exe = Join-Path $repo 'dist\stemscribe.exe'
    New-Item -ItemType Directory -Force -Path (Join-Path $repo 'dist') | Out-Null
    $env:CGO_ENABLED = '0'
    go build -o $exe ./cmd/stemscribe
    Assert-True ($LASTEXITCODE -eq 0 -and (Test-Path $exe)) 'binary built'

    Write-Host '== 2. go test ==' -ForegroundColor Cyan
    go test ./... 2>&1 | Out-Null
    Assert-True ($LASTEXITCODE -eq 0) 'go test ./... green'

    Write-Host '== 3. probe ==' -ForegroundColor Cyan
    $probe = & $exe probe --workdir $Workdir 2>&1
    Assert-True ($LASTEXITCODE -eq 0) 'probe exits 0'
    $probe | ForEach-Object { Write-Host "    $_" -ForegroundColor DarkGray }
    Assert-True (($probe -join "`n") -match 'backend\s*:') 'probe reports a backend'

    Write-Host '== 4. run (synthetic engine) ==' -ForegroundColor Cyan
    $input = Join-Path $Workdir 'input.bin'
    New-Item -ItemType Directory -Force -Path $Workdir | Out-Null
    [System.IO.File]::WriteAllBytes($input, [byte[]](1..64))
    $out = & $exe run --input $input --workdir $Workdir --duration-ms $DurationMs 2>&1
    $out | ForEach-Object { Write-Host "    $_" -ForegroundColor DarkGray }
    Assert-True ($LASTEXITCODE -eq 0) 'run exits 0'

    Write-Host '== 5. bundle contract ==' -ForegroundColor Cyan
    $lib = Join-Path $Workdir 'libraries'
    $bundles = @(Get-ChildItem -LiteralPath $lib -Directory -ErrorAction SilentlyContinue)
    Assert-True ($bundles.Count -eq 1) 'exactly one bundle created'

    if ($bundles.Count -ge 1) {
        $b = $bundles[0].FullName
        foreach ($rel in @('meta.json', 'text\transcript.json', 'text\transcript.srt', 'text\transcript.vtt', 'text\transcript.md')) {
            Assert-True (Test-Path (Join-Path $b $rel)) "bundle has $rel"
        }
        Assert-True ((Get-ChildItem (Join-Path $b 'runs') -Filter '*.jsonl' -ErrorAction SilentlyContinue).Count -ge 1) 'run log written'

        $segs = Get-Content (Join-Path $b 'text\transcript.json') -Raw -Encoding UTF8 | ConvertFrom-Json
        Assert-True ($segs.Count -gt 0) 'transcript has segments'
        $ordered = $true
        for ($i = 1; $i -lt $segs.Count; $i++) {
            if ([int]$segs[$i].start_ms -le [int]$segs[$i - 1].start_ms) { $ordered = $false }
        }
        Assert-True $ordered 'segment start times strictly increase'

        $srt = Get-Content (Join-Path $b 'text\transcript.srt') -Raw -Encoding UTF8
        Assert-True ($srt -match '(?m)^\d{2}:\d{2}:\d{2},\d{3} --> \d{2}:\d{2}:\d{2},\d{3}$') 'srt has valid timestamps'

        $meta = Get-Content (Join-Path $b 'meta.json') -Raw -Encoding UTF8 | ConvertFrom-Json
        Assert-True ($meta.status -eq 'done') 'meta.status == done'
        Assert-True ($null -ne $meta.provenance.engines) 'provenance present'
    }
} finally {
    Pop-Location
}

Write-Host ''
if ($failures.Count -gt 0) {
    Write-Host ("SMOKE TEST FAILED ({0} assertion(s))" -f $failures.Count) -ForegroundColor Red
    exit 1
}
Write-Host 'SMOKE TEST PASSED' -ForegroundColor Green
exit 0
