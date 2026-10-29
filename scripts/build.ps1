# Builds the production gateway binary used by PM2.
$ErrorActionPreference = "Stop"

$root = Split-Path -Parent $PSScriptRoot
Set-Location -LiteralPath $root

New-Item -ItemType Directory -Force -Path "bin" | Out-Null
go build -trimpath -o "bin/llm-api-gateway.exe" ./cmd/server
Write-Output "built $root\bin\llm-api-gateway.exe"
