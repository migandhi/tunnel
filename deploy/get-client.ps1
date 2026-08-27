# Tunnel client installer for Windows
# Usage:  irm https://raw.githubusercontent.com/migandhi/tunnel/main/deploy/get-client.ps1 | iex
$ErrorActionPreference = "Stop"

$Repo = "migandhi/tunnel"
$Arch = if ($env:PROCESSOR_ARCHITECTURE -eq "ARM64") { "arm64" } else { "amd64" }
$Name = "tunnel-client-windows-$Arch.exe"
$Dir  = Join-Path $env:USERPROFILE "tunnel"
$Dest = Join-Path $Dir "tunnel-client.exe"
$Url  = "https://github.com/$Repo/releases/latest/download/$Name"

New-Item -ItemType Directory -Force -Path $Dir | Out-Null
Write-Host "Downloading $Url"
Invoke-WebRequest -Uri $Url -OutFile $Dest -UseBasicParsing
Write-Host "Installed: $Dest"

# Add install dir to the user's PATH if missing
$UserPath = [Environment]::GetEnvironmentVariable("Path", "User")
if ($UserPath -notlike "*$Dir*") {
    [Environment]::SetEnvironmentVariable("Path", "$UserPath;$Dir", "User")
    Write-Host "Added $Dir to your PATH. Open a NEW terminal to use 'tunnel-client'."
} else {
    Write-Host "PATH already contains $Dir"
}

& $Dest version
Write-Host ""
Write-Host "Usage:  tunnel-client http 8000 --server tun.example.com:7000 --token tk_..."
