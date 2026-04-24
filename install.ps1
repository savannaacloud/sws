<#
.SYNOPSIS
  Savannaa Cloud CLI (sws) installer for Windows.
.EXAMPLE
  irm https://raw.githubusercontent.com/savannaacloud/sws/main/install.ps1 | iex
.NOTES
  Environment overrides:
    $env:SWS_VERSION      pin a specific version (default: latest v* release)
    $env:SWS_INSTALL_DIR  target directory (default: $env:LOCALAPPDATA\sws)
#>
$ErrorActionPreference = "Stop"
$REPO = "savannaacloud/sws"
$BINARY = "sws.exe"
$InstallDir = if ($env:SWS_INSTALL_DIR) { $env:SWS_INSTALL_DIR } else { Join-Path $env:LOCALAPPDATA "sws" }

# Detect CPU arch
$archRaw = (Get-CimInstance Win32_Processor).Architecture
switch ($archRaw) {
  9  { $arch = "amd64" }   # x64
  12 { $arch = "arm64" }   # ARM64
  default { throw "Unsupported Windows CPU architecture: $archRaw" }
}

# Resolve version
if ($env:SWS_VERSION) {
  $version = $env:SWS_VERSION
} else {
  $release = Invoke-RestMethod "https://api.github.com/repos/$REPO/releases/latest"
  $version = $release.tag_name
}
if (-not $version) { throw "Could not resolve latest release. Set `$env:SWS_VERSION or tag a release first." }

$url = "https://github.com/$REPO/releases/download/$version/sws-windows-$arch.exe"
Write-Host "Downloading $url"

New-Item -ItemType Directory -Force -Path $InstallDir | Out-Null
$target = Join-Path $InstallDir $BINARY
Invoke-WebRequest -Uri $url -OutFile $target -UseBasicParsing

# Add install dir to User PATH if not already there
$userPath = [Environment]::GetEnvironmentVariable("Path", "User")
if ($userPath -notlike "*$InstallDir*") {
  if ($userPath -and -not $userPath.EndsWith(";")) { $userPath += ";" }
  [Environment]::SetEnvironmentVariable("Path", "$userPath$InstallDir", "User")
  Write-Host ""
  Write-Host "Added $InstallDir to your User PATH."
  Write-Host "Open a new PowerShell window for the PATH change to take effect."
}

Write-Host ""
Write-Host "Installed sws $version to $target"
Write-Host "Next: sws login"
