#Requires -Version 5.1
<#
AC-02: every spec default must equal what the real binary reports.

The point is to stop the parameter table from being self-certifying. params.go can
claim "-bs defaults to 5" forever; only whisper-cli --help can settle it. This
parses the bracketed default out of the engine's own help output and diffs it
against the generated spec table.

Usage:  scripts/param-defaults-check.ps1 [-Engine path\to\whisper-cli.exe]
#>
[CmdletBinding()]
param(
    [string]$Engine = ""
)

$ErrorActionPreference = "Stop"
$repo = Split-Path -Parent $PSScriptRoot

if (-not $Engine) {
    $cand = Get-ChildItem -Path (Join-Path $repo "dist\workdir\runtimes") -Filter "whisper-cli.exe" -Recurse -ErrorAction SilentlyContinue |
        Select-Object -First 1
    if ($cand) { $Engine = $cand.FullName }
}
if (-not $Engine -or -not (Test-Path $Engine)) {
    Write-Host "skip: no whisper-cli.exe found (install a runtime first)" -ForegroundColor Yellow
    exit 0
}

Write-Host "engine: $Engine"
# whisper-cli prints its help to stderr and exits non-zero, so a plain invocation is
# expected to fail; only the text matters. "Stop" would turn that expected noise
# into a terminating error, so relax it just for this call.
$ErrorActionPreference = "Continue"
$help = (& $Engine --help 2>&1 | Out-String)
$ErrorActionPreference = "Stop"
if (-not $help) {
    Write-Host "FAIL: engine produced no help output" -ForegroundColor Red
    exit 1
}

# Lines look like:  -bs N,        [5]       beam size for beam search
$binary = @{}
foreach ($line in ($help -split "`r?`n")) {
    $m = [regex]::Match($line, '^\s*(--?[A-Za-z0-9][A-Za-z0-9_-]*)[^\[]*\[([^\]]*)\]')
    if ($m.Success) {
        $flag = $m.Groups[1].Value
        if (-not $binary.ContainsKey($flag)) {
            $binary[$flag] = $m.Groups[2].Value.Trim()
        }
    }
}
Write-Host "parsed $($binary.Count) flags with defaults from --help"

$specPath = Join-Path $repo "frontend\lib\params.generated.json"
if (-not (Test-Path $specPath)) {
    Write-Host "FAIL: $specPath missing; run: go run ./cmd/genparams" -ForegroundColor Red
    exit 1
}
$specs = (Get-Content -Raw -Encoding UTF8 $specPath | ConvertFrom-Json).specs

function Normalize([string]$v) {
    if ($null -eq $v) { return "" }
    $v = $v.Trim().TrimEnd('.')
    if ($v -eq "") { return "" }
    # Booleans: whisper.cpp prints "true"/"false" for -fa and -sns.
    if ($v -match '^(true|false)$') { return $v.ToLower() }
    $n = 0.0
    if ([double]::TryParse($v, [ref]$n)) {
        if ($n -eq [math]::Floor($n)) { return ([int]$n).ToString() }
        return $n.ToString([System.Globalization.CultureInfo]::InvariantCulture)
    }
    return $v
}

$checked = 0
$mismatch = 0
$missing = 0
foreach ($s in $specs) {
    if (-not $binary.ContainsKey($s.flag)) { $missing++; continue }
    $checked++
    $want = Normalize ([string]$s.def)
    $got = Normalize ([string]$binary[$s.flag])
    if ($want -ne $got) {
        $mismatch++
        Write-Host ("MISMATCH {0,-28} spec={1,-10} binary={2}" -f $s.flag, $want, $got) -ForegroundColor Red
    }
}

Write-Host ""
Write-Host "compared $checked specs; $missing flags not documented in --help; $mismatch mismatches"
if ($mismatch -gt 0) {
    Write-Host "AC-02 FAILED: spec defaults disagree with the binary" -ForegroundColor Red
    exit 1
}
Write-Host "AC-02 passed: every documented default matches params.go" -ForegroundColor Green
