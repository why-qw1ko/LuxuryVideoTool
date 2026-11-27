param(
    [string]$FFmpegBin
)

$ErrorActionPreference = 'Stop'
$repositoryRoot = Split-Path -Parent (Split-Path -Parent $PSScriptRoot)
$apiRoot = Join-Path $repositoryRoot 'services\api_go'
$version = (Get-Content -Raw -LiteralPath (Join-Path $repositoryRoot 'VERSION')).Trim()
$outputRoot = Join-Path $repositoryRoot ("dist\douyin-capture-windows-{0}" -f $version)

if (-not (Get-Command go -ErrorAction SilentlyContinue)) { throw 'Go was not found.' }
if (Test-Path -LiteralPath $outputRoot) { throw "Output directory already exists. Move it away and retry: $outputRoot" }
New-Item -ItemType Directory -Path $outputRoot -Force | Out-Null

Push-Location $apiRoot
try {
    go build -trimpath -ldflags "-s -w -X github.com/why-qw1ko/LuxuryVideoTool/services/api_go/internal/version.Version=$version" -o (Join-Path $outputRoot 'douyin-capture-server.exe') ./cmd/server
    go build -trimpath -ldflags "-s -w -X github.com/why-qw1ko/LuxuryVideoTool/services/api_go/internal/version.Version=$version" -o (Join-Path $outputRoot 'douyin-capture-admin.exe') ./cmd/admin
} finally { Pop-Location }

Copy-Item -LiteralPath (Join-Path $repositoryRoot '.env.example') -Destination (Join-Path $outputRoot '.env.example') -Force
Copy-Item -LiteralPath (Join-Path $PSScriptRoot 'run-package.ps1') -Destination (Join-Path $outputRoot 'start-web.ps1') -Force
Copy-Item -LiteralPath (Join-Path $PSScriptRoot 'initialize-package.ps1') -Destination (Join-Path $outputRoot 'initialize-admin.ps1') -Force
Copy-Item -LiteralPath (Join-Path $repositoryRoot 'docs\operations\Windows网页版使用说明.md') -Destination (Join-Path $outputRoot 'README.md') -Force

if ($FFmpegBin) {
    $resolvedFFmpeg = (Resolve-Path -LiteralPath $FFmpegBin).Path
    foreach ($name in @('ffmpeg.exe', 'ffprobe.exe')) { if (-not (Test-Path -LiteralPath (Join-Path $resolvedFFmpeg $name))) { throw "$resolvedFFmpeg does not contain $name" } }
    $tools = Join-Path $outputRoot 'tools'; New-Item -ItemType Directory -Path $tools -Force | Out-Null
    Copy-Item -LiteralPath (Join-Path $resolvedFFmpeg 'ffmpeg.exe') -Destination $tools -Force
    Copy-Item -LiteralPath (Join-Path $resolvedFFmpeg 'ffprobe.exe') -Destination $tools -Force
}

Write-Host "Local package created: $outputRoot"
