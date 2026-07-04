#!/bin/sh
# CrossTalk server entrypoint. Configuration is entirely env-driven (see
# server/cmd/ct-server/main.go). This script only fills in sensible defaults.
set -e

: "${CT_ADDR:=:8080}"
: "${CT_DATABASE_URL:=postgres://postgres:postgres@postgres:5432/crosstalk?sslmode=disable}"
: "${CT_JWT_SECRET:=change-me-in-production}"

export CT_ADDR CT_DATABASE_URL CT_JWT_SECRET

echo "Starting ct-server addr=${CT_ADDR} db=${CT_DATABASE_URL%%\?*}"
exec ct-server
