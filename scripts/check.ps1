$ErrorActionPreference = 'Stop'
$repoRoot = Split-Path -Parent $PSScriptRoot

Push-Location (Join-Path $repoRoot 'services/api_go')
try {
    $unformatted = gofmt -l .
    if ($unformatted) { throw "Go files require gofmt: $unformatted" }
    go vet ./...
    go test ./...
} finally {
    Pop-Location
}

Push-Location (Join-Path $repoRoot 'apps/client_flutter')
try {
    dart format --output=none --set-exit-if-changed lib test
    flutter analyze
    flutter test
} finally {
    Pop-Location
}

