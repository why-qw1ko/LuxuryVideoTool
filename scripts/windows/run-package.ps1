$ErrorActionPreference = 'Stop'
$packageRoot = $PSScriptRoot
$environmentPath = Join-Path $packageRoot '.env'
if (-not (Test-Path -LiteralPath $environmentPath)) { Copy-Item -LiteralPath (Join-Path $packageRoot '.env.example') -Destination $environmentPath }
$dataPath = Join-Path $packageRoot 'data\app.db'
if (-not (Test-Path -LiteralPath $dataPath)) { throw 'Run initialize-admin.ps1 before starting the service.' }
foreach ($line in Get-Content -LiteralPath $environmentPath -Encoding utf8) {
    $trimmed = $line.Trim(); if ($trimmed.Length -eq 0 -or $trimmed.StartsWith('#')) { continue }
    $separator = $trimmed.IndexOf('='); if ($separator -le 0) { throw "Invalid configuration line: $line" }
    [Environment]::SetEnvironmentVariable($trimmed.Substring(0, $separator).Trim(), $trimmed.Substring($separator + 1).Trim(), 'Process')
}
$env:HTTP_ADDR = '127.0.0.1:8080'; $env:DATABASE_PATH = $dataPath; $env:DATA_DIR = Join-Path $packageRoot 'data'; $env:JWT_SIGNING_KEY_FILE = Join-Path $packageRoot 'secrets\jwt.key'
$tools = Join-Path $packageRoot 'tools'; if (Test-Path -LiteralPath (Join-Path $tools 'ffmpeg.exe')) { $env:FFMPEG_PATH = Join-Path $tools 'ffmpeg.exe'; $env:FFPROBE_PATH = Join-Path $tools 'ffprobe.exe' }
if (-not (Test-Path -LiteralPath $env:JWT_SIGNING_KEY_FILE)) { New-Item -ItemType Directory -Path (Split-Path -Parent $env:JWT_SIGNING_KEY_FILE) -Force | Out-Null; $key=New-Object byte[] 32; $random=[Security.Cryptography.RandomNumberGenerator]::Create(); try{$random.GetBytes($key)}finally{$random.Dispose()}; [IO.File]::WriteAllBytes($env:JWT_SIGNING_KEY_FILE,$key) }
$serverPath = Join-Path $packageRoot 'douyin-capture-server.exe'
$server = Start-Process -FilePath $serverPath -WorkingDirectory $packageRoot -WindowStyle Hidden -PassThru
try {
    $ready = $false
    for ($attempt = 0; $attempt -lt 40; $attempt++) {
        try { $response = Invoke-WebRequest -UseBasicParsing -Uri 'http://127.0.0.1:8080/health/live' -TimeoutSec 1; if ($response.StatusCode -eq 200) { $ready = $true; break } } catch { Start-Sleep -Milliseconds 250 }
    }
    if (-not $ready) { throw 'Service startup failed. Run douyin-capture-server.exe in PowerShell to view the error.' }
    Start-Process 'http://127.0.0.1:8080'
    Write-Host 'Service is running. Closing this window stops the service.'
    Wait-Process -Id $server.Id
} finally {
    if (-not $server.HasExited) { Stop-Process -Id $server.Id }
}
