param(
  [string]$Binary = ".\rewind-windows-amd64.exe",
  [string]$Workspace = "$env:TEMP\rewind-windows-contract-fixture"
)

$ErrorActionPreference = "Stop"
if ($env:OS -ne "Windows_NT") {
  throw "windows acceptance: run this contract preflight on Windows"
}

New-Item -ItemType Directory -Force -Path $Workspace | Out-Null
$payload = & $Binary platform contract --platform windows --workspace $Workspace | ConvertFrom-Json

if (-not $payload.code_complete) {
  throw "windows acceptance: portable contract is not complete"
}
if (-not $payload.manual_gate_required) {
  throw "windows acceptance: expected the signed-helper/VHDX manual gate"
}

[pscustomobject]@{
  result = "WINDOWS_CONTRACT_PREFLIGHT_PASS"
  platform = $payload.platform
  backend = $payload.backend
  manual_gate_required = $payload.manual_gate_required
  reasons = ($payload.reasons -join "; ")
} | ConvertTo-Json -Compress
