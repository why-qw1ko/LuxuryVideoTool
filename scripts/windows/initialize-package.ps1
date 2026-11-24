$ErrorActionPreference = 'Stop'
$packageRoot = $PSScriptRoot
$environmentPath = Join-Path $packageRoot '.env'
if (-not (Test-Path -LiteralPath $environmentPath)) {
    Copy-Item -LiteralPath (Join-Path $packageRoot '.env.example') -Destination $environmentPath
}
$env:DATABASE_PATH = Join-Path $packageRoot 'data\app.db'
$keyPath = Join-Path $packageRoot 'secrets\jwt.key'
if (-not (Test-Path -LiteralPath $keyPath)) { New-Item -ItemType Directory -Path (Split-Path -Parent $keyPath) -Force | Out-Null; $key=New-Object byte[] 32; $random=[Security.Cryptography.RandomNumberGenerator]::Create(); try{$random.GetBytes($key)}finally{$random.Dispose()}; [IO.File]::WriteAllBytes($keyPath,$key) }
$password = Read-Host 'Enter administrator password (6 to 12 characters)' -AsSecureString
$credential = New-Object Management.Automation.PSCredential('local', $password)
$plainPassword = $credential.GetNetworkCredential().Password
$temporaryPassword = Join-Path ([IO.Path]::GetTempPath()) ("douyin-capture-password-{0}.txt" -f [Guid]::NewGuid().ToString('N'))
try {
    [IO.File]::WriteAllText($temporaryPassword,$plainPassword,[Text.UTF8Encoding]::new($false))
    & (Join-Path $packageRoot 'douyin-capture-admin.exe') create-user --username owner --display-name Owner --role admin --password-file $temporaryPassword
    if ($LASTEXITCODE -ne 0) { throw 'Account creation failed.' }
}
finally { $plainPassword=$null; if(Test-Path -LiteralPath $temporaryPassword){Remove-Item -LiteralPath $temporaryPassword -Force} }
