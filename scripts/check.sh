#!/usr/bin/env sh
set -eu
repo_root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)

cd "$repo_root/services/api_go"
test -z "$(gofmt -l .)"
go vet ./...
go test ./...

cd "$repo_root/apps/client_flutter"
dart format --output=none --set-exit-if-changed lib test
flutter analyze
flutter test

