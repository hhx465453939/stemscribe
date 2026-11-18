<#
.SYNOPSIS
    Stages one whisper.cpp build into a self-contained archive and measures it.

.DESCRIPTION
    Called by .github/workflows/runtime.yml. Kept as a script rather than inline
    YAML so the staging rules can be read and tested on their own.

    What it guarantees:
      * whisper-cli.exe plus every DLL it needs sits in one directory
      * PROVENANCE.txt records the upstream tag, backend and our commit
      * the archive is measured (size + sha256) and that measurement is written to
        a small JSON file, which is the ONLY sanctioned source for a registry
        checksum (AGENTS.md iron law 2: never invent one)
      * --help is executed as a smoke check, so a binary that cannot start never
        gets published
#>
[CmdletBinding()]
param(
    [Parameter(Mandatory = $true)][ValidateSet('cpu', 'vulkan', 'cuda12')][string]$Backend,
    [Parameter(Mandatory = $true)][string]$Version
)

$ErrorActionPreference = 'Stop'

$ver = $Version.TrimStart('v')
$stage = "stage/whispercpp-$Backend-win-x64"

if (Test-Path $stage) { Remove-Item $stage -Recurse -Force }
New-Item -ItemType Directory -Force -Path $stage | Out-Null
New-Item -ItemType Directory -Force -Path dist | Out-Null

# Locate the produced binary rather than assuming a layout: this is the one part
# that differs between generators and between CMake versions.
$exe = Get-ChildItem -Path src/build -Recurse -Filter whisper-cli.exe -ErrorAction SilentlyContinue |
       Select-Object -First 1
if (-not $exe) { throw "whisper-cli.exe was not produced for backend $Backend" }
Write-Host "found $($exe.FullName)"

Copy-Item $exe.FullName $stage -Force
Get-ChildItem -Path $exe.Directory -Filter *.dll -ErrorAction SilentlyContinue |
    ForEach-Object { Copy-Item $_.FullName $stage -Force }

if ($Backend -eq 'cuda12') {
    # CUDA runtime belongs next to the exe; cublas is large, which is exactly why
    # the CUDA archive is a separate download and not bundled with the others.
    Get-ChildItem -Path src/build -Recurse -Include cudart64*.dll,cublas*.dll -ErrorAction SilentlyContinue |
        ForEach-Object { Copy-Item $_.FullName $stage -Force }
}

Copy-Item src/LICENSE (Join-Path $stage 'LICENSE-whisper.cpp.txt') -ErrorAction SilentlyContinue

@"
source: https://github.com/ggml-org/whisper.cpp
tag: $Version
backend: $Backend
built_by: .github/workflows/runtime.yml
commit: $env:GITHUB_SHA
"@ | Set-Content (Join-Path $stage 'PROVENANCE.txt')

# Smoke check before packaging: an archive that cannot start is worse than none.
$stagedExe = Join-Path $stage 'whisper-cli.exe'
& $stagedExe --help | Select-Object -First 5
# whisper-cli returns 0 on --help in some builds and 1 in others; anything else
# (especially a crash) means the DLL set is incomplete.
if ($LASTEXITCODE -ne 0 -and $LASTEXITCODE -ne 1) {
    throw "whisper-cli --help exited $LASTEXITCODE; the staged DLL set is probably incomplete"
}

$zip = Join-Path 'dist' "whispercpp-$ver-$Backend-win-x64.zip"
Compress-Archive -Path $stage -DestinationPath $zip -Force

$hash = (Get-FileHash $zip -Algorithm SHA256).Hash.ToLower()
$size = (Get-Item $zip).Length
@{ file = (Split-Path $zip -Leaf); size = $size; sha256 = $hash } |
    ConvertTo-Json -Compress | Set-Content (Join-Path 'dist' "$Backend.json")

Write-Host "staged $zip"
Write-Host "size=$size"
Write-Host "sha256=$hash"
