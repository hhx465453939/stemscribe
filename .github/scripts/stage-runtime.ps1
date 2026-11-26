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
    [Parameter(Mandatory = $true)][string]$Version,
    # Checkout to stage from. CI keeps it at src/; the local CUDA build uses a
    # different path so it can reuse a cached clone.
    [string]$SrcDir = 'src',
    # CUDA toolkit bin directory. Required for cuda12: the runtime DLLs are not
    # produced by the whisper.cpp build, so the archive must copy them from here
    # or the exe only works on machines with the full toolkit installed.
    [string]$CudaBinDir = ''
)

$ErrorActionPreference = 'Stop'

$ver = $Version.TrimStart('v')
$stage = "stage/whispercpp-$Backend-win-x64"

if (Test-Path $stage) { Remove-Item $stage -Recurse -Force }
New-Item -ItemType Directory -Force -Path $stage | Out-Null
New-Item -ItemType Directory -Force -Path dist | Out-Null

# Locate the produced binary rather than assuming a layout: this is the one part
# that differs between generators and between CMake versions.
$buildDir = Join-Path $SrcDir 'build'
$exe = Get-ChildItem -Path $buildDir -Recurse -Filter whisper-cli.exe -ErrorAction SilentlyContinue |
       Select-Object -First 1
if (-not $exe) { throw "whisper-cli.exe was not produced for backend $Backend under $buildDir" }
Write-Host "found $($exe.FullName)"

Copy-Item $exe.FullName $stage -Force
Get-ChildItem -Path $exe.Directory -Filter *.dll -ErrorAction SilentlyContinue |
    ForEach-Object { Copy-Item $_.FullName $stage -Force }

if ($Backend -eq 'cuda12') {
    # CUDA runtime belongs next to the exe; cublas is large, which is exactly why
    # the CUDA archive is a separate download and not bundled with the others.
    if (-not $CudaBinDir -or -not (Test-Path $CudaBinDir)) {
        throw "cuda12 staging needs -CudaBinDir pointing at the CUDA toolkit bin directory"
    }
    foreach ($pattern in @('cudart64*.dll', 'cublas64*.dll', 'cublasLt64*.dll')) {
        Get-ChildItem -Path $CudaBinDir -Filter $pattern -ErrorAction SilentlyContinue |
            ForEach-Object { Copy-Item $_.FullName $stage -Force }
    }
    # Anything the build tree happened to place next to the exe is already copied
    # above; CUDA_PATH\bin is authoritative for the toolkit DLLs.

    # Hard gate: a CUDA archive missing any of these starts fine on the build
    # machine (toolkit on PATH) and crashes on the user's. Refuse to package it.
    $required = @('cudart64_12.dll', 'cublas64_12.dll', 'cublasLt64_12.dll')
    $missing = $required | Where-Object { -not (Test-Path (Join-Path $stage $_)) }
    if ($missing) {
        throw "cuda12 archive is missing required DLLs: $($missing -join ', '). The exe would not start on a machine without the CUDA toolkit."
    }
    Write-Host "bundled CUDA DLLs: $($required -join ', ')"
}

Copy-Item (Join-Path $SrcDir 'LICENSE') (Join-Path $stage 'LICENSE-whisper.cpp.txt') -ErrorAction SilentlyContinue

$commit = $env:GITHUB_SHA
if (-not $commit) {
    $commit = (& git -C $SrcDir rev-parse HEAD 2>$null)
    if ($LASTEXITCODE -ne 0 -or -not $commit) { $commit = 'local' }
}

@"
source: https://github.com/ggml-org/whisper.cpp
tag: $Version
backend: $Backend
built_by: .github/scripts/stage-runtime.ps1
commit: $commit
cuda_bin: $CudaBinDir
"@ | Set-Content (Join-Path $stage 'PROVENANCE.txt')

# Smoke check before packaging: an archive that cannot start is worse than none.
# Two Windows PowerShell traps to avoid here:
#   * piping into Select-Object -First terminates the pipeline early and leaves
#     whisper-cli.exe holding its image file, so the following Compress-Archive
#     fails with "used by another process";
#   * `2>&1` on a native exe wraps stderr in ErrorRecord and, with
#     $ErrorActionPreference = 'Stop', throws NativeCommandError -- whisper-cli
#     prints its usage on stderr.
# Start-Process redirects both streams at the OS level and -Wait guarantees the
# process (and its file handles) are gone before we package.
$stagedExe = Join-Path $stage 'whisper-cli.exe'
$smokeDir = Join-Path 'tmp' 'smoke'
New-Item -ItemType Directory -Force -Path $smokeDir | Out-Null
$smokeOut = Join-Path $smokeDir 'whisper-help.out.txt'
$smokeErr = Join-Path $smokeDir 'whisper-help.err.txt'
$proc = Start-Process -FilePath $stagedExe -ArgumentList '--help' -NoNewWindow -Wait -PassThru `
    -RedirectStandardOutput $smokeOut -RedirectStandardError $smokeErr
$help = @()
foreach ($f in @($smokeOut, $smokeErr)) {
    if (Test-Path -LiteralPath $f) { $help += @(Get-Content -LiteralPath $f) }
}
Remove-Item -LiteralPath $smokeOut, $smokeErr -Force -ErrorAction SilentlyContinue
$help | Select-Object -First 5
# whisper-cli returns 0 on --help in some builds and 1 in others; anything else
# (especially a crash) means the DLL set is incomplete.
if ($proc.ExitCode -ne 0 -and $proc.ExitCode -ne 1) {
    throw "whisper-cli --help exited $($proc.ExitCode); the staged DLL set is probably incomplete"
}
if ($help.Count -eq 0) {
    throw "whisper-cli --help printed nothing; the binary is not usable"
}

$zip = Join-Path 'dist' "whispercpp-$ver-$Backend-win-x64.zip"
# A freshly written exe can be locked for a moment by AV scanning. Retrying is
# more honest than declaring the build broken.
for ($attempt = 1; $attempt -le 5; $attempt++) {
    try {
        Compress-Archive -Path $stage -DestinationPath $zip -Force
        break
    } catch {
        if ($attempt -eq 5) { throw }
        Write-Host "compress attempt $attempt failed ($($_.Exception.Message)); retrying"
        Start-Sleep -Milliseconds 1000
    }
}

$hash = (Get-FileHash $zip -Algorithm SHA256).Hash.ToLower()
$size = (Get-Item $zip).Length
@{ file = (Split-Path $zip -Leaf); size = $size; sha256 = $hash } |
    ConvertTo-Json -Compress | Set-Content (Join-Path 'dist' "$Backend.json")

Write-Host "staged $zip"
Write-Host "size=$size"
Write-Host "sha256=$hash"
