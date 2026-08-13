param(
    [string]$Username = 'owner',
    [string]$DisplayName = 'Owner',
    [ValidateSet('admin', 'user')][string]$Role = 'admin',
    [string]$EnvironmentFile = '.env'
)

$ErrorActionPreference = 'Stop'
$repositoryRoot = Split-Path -Parent (Split-Path -Parent $PSScriptRoot)
$apiRoot = Join-Path $repositoryRoot 'services\api_go'
$environmentPath = if ([IO.Path]::IsPathRooted($EnvironmentFile)) { $EnvironmentFile } else { Join-Path $repositoryRoot $EnvironmentFile }

if (-not (Get-Command go -ErrorAction SilentlyContinue)) { throw 'Go was not found. Install the version specified in README.' }
if (-not (Test-Path -LiteralPath $environmentPath)) { Copy-Item -LiteralPath (Join-Path $repositoryRoot '.env.example') -Destination $environmentPath }

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

if ([string]::IsNullOrWhiteSpace($env:DATABASE_PATH)) { $env:DATABASE_PATH = '.\data\app.db' }
if ([string]::IsNullOrWhiteSpace($env:JWT_SIGNING_KEY_FILE)) { $env:JWT_SIGNING_KEY_FILE = '.\secrets\jwt.key' }
$keyPath = if ([IO.Path]::IsPathRooted($env:JWT_SIGNING_KEY_FILE)) { $env:JWT_SIGNING_KEY_FILE } else { Join-Path $apiRoot $env:JWT_SIGNING_KEY_FILE }
if (-not (Test-Path -LiteralPath $keyPath)) { New-Item -ItemType Directory -Path (Split-Path -Parent $keyPath) -Force | Out-Null; $key=New-Object byte[] 32; $random=[Security.Cryptography.RandomNumberGenerator]::Create(); try{$random.GetBytes($key)}finally{$random.Dispose()}; [IO.File]::WriteAllBytes($keyPath,$key) }
$password = Read-Host 'Enter administrator password (at least 12 characters)' -AsSecureString
$credential = New-Object Management.Automation.PSCredential('local', $password)
$plainPassword = $credential.GetNetworkCredential().Password
if ($plainPassword.Length -lt 12) { throw 'Password must contain at least 12 characters.' }

$temporaryPassword = Join-Path ([IO.Path]::GetTempPath()) ("douyin-capture-password-{0}.txt" -f [Guid]::NewGuid().ToString('N'))
try {
    [IO.File]::WriteAllText($temporaryPassword, $plainPassword, [Text.UTF8Encoding]::new($false))
    Push-Location $apiRoot
    try { go run ./cmd/admin create-user --username $Username --display-name $DisplayName --role $Role --password-file $temporaryPassword }
    finally { Pop-Location }
} finally {
    $plainPassword = $null
    if (Test-Path -LiteralPath $temporaryPassword) { Remove-Item -LiteralPath $temporaryPassword -Force }
}

Write-Host 'Initialization completed. Run .\scripts\windows\start-web.ps1 to start the service.'
