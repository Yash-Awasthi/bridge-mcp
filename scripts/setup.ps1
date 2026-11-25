# Build the board, put it on PATH, and wire every detected MCP client.
# Override the destination with $env:VB_BIN_DIR; extra arguments go to `vb install`.
$ErrorActionPreference = "Stop"
$root = Split-Path -Parent $PSScriptRoot
$bin = if ($env:VB_BIN_DIR) { $env:VB_BIN_DIR } else { Join-Path $env:LOCALAPPDATA "Programs\vb" }
New-Item -ItemType Directory -Force -Path $bin | Out-Null
Push-Location $root
try {
  go build -o (Join-Path $bin "vb.exe") ./cmd/open-board
  go build -o (Join-Path $bin "mcp-bridge.exe") ./cmd/mcp-bridge
} finally {
  Pop-Location
}
Write-Host "built $bin\vb.exe"
$userPath = [Environment]::GetEnvironmentVariable("Path", "User")
if ($userPath -notlike "*$bin*") {
  Write-Host "note: add this folder to your user PATH: $bin"
  Write-Host "  Settings > System > About > Advanced system settings > Environment Variables"
}
& (Join-Path $bin "vb.exe") install @args
