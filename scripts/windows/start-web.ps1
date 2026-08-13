param(
    [string]$EnvironmentFile = ".env"
)

$ErrorActionPreference = 'Stop'
$repositoryRoot = Split-Path -Parent (Split-Path -Parent $PSScriptRoot)
$apiRoot = Join-Path $repositoryRoot 'services\api_go'
$environmentPath = if ([IO.Path]::IsPathRooted($EnvironmentFile)) { $EnvironmentFile } else { Join-Path $repositoryRoot $EnvironmentFile }
$openBrowser = $false

if (-not (Get-Command go -ErrorAction SilentlyContinue)) {
    throw 'Go was not found. Install the version specified in README, then reopen PowerShell.'
}
if (-not (Test-Path -LiteralPath $environmentPath)) {
    Copy-Item -LiteralPath (Join-Path $repositoryRoot '.env.example') -Destination $environmentPath
    Write-Host "Created $environmentPath. Configure ASR settings when needed."
}

foreach ($line in Get-Content -LiteralPath $environmentPath -Encoding utf8) {
    $trimmed = $line.Trim()
    if ($trimmed.Length -eq 0 -or $trimmed.StartsWith('#')) { continue }
    $separator = $trimmed.IndexOf('=')
    if ($separator -le 0) { throw "Invalid configuration line: $line" }
    $name = $trimmed.Substring(0, $separator).Trim()
    $value = $trimmed.Substring($separator + 1).Trim()
    if ($name -notmatch '^[A-Z][A-Z0-9_]*$') { throw "Invalid environment variable name: $name" }
    [Environment]::SetEnvironmentVariable($name, $value, 'Process')
}

if ([string]::IsNullOrWhiteSpace($env:JWT_SIGNING_KEY_FILE)) {
    $env:JWT_SIGNING_KEY_FILE = '.\secrets\jwt.key'
}
$keyPath = if ([IO.Path]::IsPathRooted($env:JWT_SIGNING_KEY_FILE)) { $env:JWT_SIGNING_KEY_FILE } else { Join-Path $apiRoot $env:JWT_SIGNING_KEY_FILE }
if (-not (Test-Path -LiteralPath $keyPath)) {
    $keyDirectory = Split-Path -Parent $keyPath
    New-Item -ItemType Directory -Path $keyDirectory -Force | Out-Null
    $key = New-Object byte[] 32
    $random = [Security.Cryptography.RandomNumberGenerator]::Create()
    try { $random.GetBytes($key) } finally { $random.Dispose() }
    [IO.File]::WriteAllBytes($keyPath, $key)
    Write-Host "Generated local JWT key: $keyPath"
}

if (-not [string]::IsNullOrWhiteSpace($env:ALIYUN_DASHSCOPE_API_KEY) -and [string]::IsNullOrWhiteSpace($env:PUBLIC_BASE_URL)) {
    Write-Warning 'Aliyun fallback requires a public HTTPS PUBLIC_BASE_URL. SiliconFlow remains available without it.'
}

Push-Location $apiRoot
try {
    Write-Host 'Web UI: http://127.0.0.1:8080'
    $server = Start-Process -FilePath 'go' -ArgumentList @('run', './cmd/server') -WorkingDirectory $apiRoot -WindowStyle Hidden -PassThru
    try {
        for ($attempt = 0; $attempt -lt 80; $attempt++) {
            if ($server.HasExited) { throw "Service stopped with exit code $($server.ExitCode). Run 'go run ./cmd/server' in services/api_go to view the error." }
            try { $response = Invoke-WebRequest -UseBasicParsing -Uri 'http://127.0.0.1:8080/health/live' -TimeoutSec 1; if ($response.StatusCode -eq 200) { $openBrowser = $true; break } } catch { Start-Sleep -Milliseconds 250 }
        }
        if (-not $openBrowser) { throw 'Service did not become ready in time.' }
        Start-Process 'http://127.0.0.1:8080'
        Write-Host 'Service is running. Closing this window stops the service.'
        Wait-Process -Id $server.Id
    } finally {
        if (-not $server.HasExited) { Stop-Process -Id $server.Id }
    }
} finally {
    Pop-Location
}
