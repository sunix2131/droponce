#!/usr/bin/env sh
set -eu

cd "$(dirname "$0")/.."

(cd frontend && npm run typecheck && npm run lint && npm test && npm run build)

PKGS="$(go list ./... | grep -v '/frontend/node_modules/')"
go vet $PKGS
go test $PKGS
go test -race $PKGS

if command -v staticcheck >/dev/null 2>&1; then
  staticcheck $PKGS
fi

if command -v govulncheck >/dev/null 2>&1; then
  govulncheck ./...
fi
