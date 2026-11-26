<#
.SYNOPSIS
    Builds the whisper.cpp CUDA runtime locally and stages it for the registry.

.DESCRIPTION
    The CI cuda job needs a CUDA-capable runner that GitHub's hosted image is not.
    This script does the same build on a developer machine so the cuda12 runtime
    can exist at all, using the same staging rules as CI
    (.github/scripts/stage-runtime.ps1): bundled cudart/cublas DLLs, a --help
    smoke check, and a measured size + sha256. Nothing here writes the registry;
    the measured JSON is the only source a checksum may come from (AGENTS.md law 2).

    Requirements: Visual Studio 2019+ with the C++ toolset, CUDA 12.x toolkit with
    nvcc on PATH or under the default install root, CMake 3.24+, Git.
#>
[CmdletBinding()]
param(
    [string]$WhisperVersion = 'v1.9.5',
    [string]$SrcDir = 'tmp/build/whisper.cpp',
    [string]$CudaArch = '52;61;75;86',
    [int]$Jobs = 8,
    # Keep an existing source checkout instead of refusing to continue.
    [switch]$ReuseSource
)

$ErrorActionPreference = 'Stop'

$repoRoot = (Resolve-Path (Join-Path $PSScriptRoot '..')).Path
Set-Location $repoRoot
$ver = $WhisperVersion.TrimStart('v')

function Find-VcVars {
    $vswhere = Join-Path ${env:ProgramFiles(x86)} 'Microsoft Visual Studio\Installer\vswhere.exe'
    if (Test-Path $vswhere) {
        $root = & $vswhere -latest -products * `
            -requires Microsoft.VisualStudio.Component.VC.Tools.x86.x64 `
            -property installationPath
        if ($root) {
            $candidate = Join-Path $root 'VC\Auxiliary\Build\vcvars64.bat'
            if (Test-Path $candidate) { return $candidate }
        }
    }
    # The machine this was written on has VS2019 BuildTools but no vswhere entry.
    foreach ($candidate in @(
            'C:\Program Files (x86)\Microsoft Visual Studio\2019\BuildTools\VC\Auxiliary\Build\vcvars64.bat',
            'C:\Program Files\Microsoft Visual Studio\2022\BuildTools\VC\Auxiliary\Build\vcvars64.bat'
        )) {
        if (Test-Path $candidate) { return $candidate }
    }
    throw 'No Visual Studio C++ toolchain found. Install VS2019/2022 BuildTools with the C++ workload.'
}

function Import-VcEnv([string]$VcVars) {
    # PowerShell cannot execute a .bat and inherit its environment, so ask cmd for
    # the resulting variables and apply them here. Anything else silently loses
    # INCLUDE/LIB and nvcc fails with "cannot open source file".
    $bat = Join-Path $repoRoot 'tmp\build\vcvars-env.bat'
    New-Item -ItemType Directory -Force -Path (Split-Path $bat) | Out-Null
    @(
        '@echo off'
        "call `"$VcVars`" >nul"
        'set'
    ) | Set-Content -Encoding ascii $bat

    foreach ($line in (& cmd.exe /d /s /c "`"$bat`"")) {
        $i = $line.IndexOf('=')
        if ($i -le 0) { continue }
        $name = $line.Substring(0, $i)
        $value = $line.Substring($i + 1)
        if ($name -match '^[A-Za-z_][A-Za-z0-9_]*$') {
            Set-Item -Path "Env:$name" -Value $value
        }
    }
    Write-Host "MSVC environment imported from $VcVars"
}

function Find-Cuda {
    if ($env:CUDA_PATH -and (Test-Path (Join-Path $env:CUDA_PATH 'bin\nvcc.exe'))) {
        return $env:CUDA_PATH
    }
    $root = 'C:\Program Files\NVIDIA GPU Computing Toolkit\CUDA'
    $dir = Get-ChildItem $root -Directory -ErrorAction SilentlyContinue |
        Where-Object { $_.Name -match '^v12\.' } |
        Sort-Object Name -Descending | Select-Object -First 1
    if (-not $dir) { throw "CUDA 12 toolkit not found under $root; install it or set CUDA_PATH." }
    return $dir.FullName
}

Write-Host "== toolchain =="
$vcvars = Find-VcVars
Import-VcEnv $vcvars
$cuda = Find-Cuda
$env:CUDA_PATH = $cuda

foreach ($tool in @('cmake', 'git')) {
    $cmd = Get-Command $tool -ErrorAction SilentlyContinue
    if (-not $cmd) { throw "$tool is not on PATH" }
}
$nvccVersion = (& (Join-Path $cuda 'bin\nvcc.exe') --version | Select-Object -Last 1)
Write-Host "vcvars : $vcvars"
Write-Host "cuda   : $cuda"
Write-Host "nvcc   : $nvccVersion"
Write-Host "cmake  : $((& cmake --version | Select-Object -First 1))"

Write-Host "== source =="
if (Test-Path $SrcDir) {
    if (-not $ReuseSource) {
        throw "$SrcDir already exists. Pass -ReuseSource to build from it, or remove it deliberately."
    }
    Write-Host "reusing $SrcDir"
} else {
    & git clone --depth 1 --branch $WhisperVersion --recurse-submodules --shallow-submodules `
        https://github.com/ggml-org/whisper.cpp $SrcDir
    if ($LASTEXITCODE -ne 0) { throw "git clone failed with exit $LASTEXITCODE" }
}

Write-Host "== configure =="
$buildDir = Join-Path $SrcDir 'build'
$generator = @()
if (Get-Command ninja -ErrorAction SilentlyContinue) {
    $generator = @('-G', 'Ninja')
} else {
    Write-Host 'ninja not found; using the Visual Studio generator'
    $generator = @('-G', 'Visual Studio 16 2019', '-A', 'x64')
}

& cmake -S $SrcDir -B $buildDir @generator `
    -DCMAKE_BUILD_TYPE=Release `
    -DBUILD_SHARED_LIBS=OFF `
    -DWHISPER_BUILD_TESTS=OFF `
    -DWHISPER_BUILD_SERVER=OFF `
    -DGGML_CUDA=ON `
    -DGGML_VULKAN=OFF `
    "-DCMAKE_CUDA_ARCHITECTURES=$CudaArch" `
    "-DCMAKE_CUDA_COMPILER=$cuda\bin\nvcc.exe" `
    '-DCMAKE_CUDA_FLAGS=-allow-unsupported-compiler -Xfatbin=-compress-all'
if ($LASTEXITCODE -ne 0) { throw "cmake configure failed with exit $LASTEXITCODE" }

Write-Host "== build whisper-cli (-j $Jobs) =="
& cmake --build $buildDir --config Release --target whisper-cli -j $Jobs
if ($LASTEXITCODE -ne 0) { throw "build failed with exit $LASTEXITCODE" }

Write-Host "== stage + measure =="
& powershell -NoProfile -ExecutionPolicy Bypass -File `
    .github/scripts/stage-runtime.ps1 `
    -Backend cuda12 -Version $WhisperVersion -SrcDir $SrcDir -CudaBinDir (Join-Path $cuda 'bin')
if ($LASTEXITCODE -ne 0) { throw "staging failed with exit $LASTEXITCODE" }

$zip = Join-Path 'dist' "whispercpp-$ver-cuda12-win-x64.zip"
$json = Join-Path 'dist' 'cuda12.json'
if (-not (Test-Path $zip) -or -not (Test-Path $json)) { throw 'staging did not produce the expected zip/json' }

Write-Host "== done =="
Write-Host "archive : $zip"
Get-Content $json
