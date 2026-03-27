# GoClaw Lite (Desktop) installer for Windows
# Usage:
#   irm https://raw.githubusercontent.com/nextlevelbuilder/goclaw/main/scripts/install-lite.ps1 | iex
#   .\install-lite.ps1 -Version lite-v0.1.0

param([string]$Version = "")

$ErrorActionPreference = "Stop"
$Repo = "nextlevelbuilder/goclaw"
$InstallDir = "$env:LOCALAPPDATA\GoClaw Lite"

# Resolve latest version
if (-not $Version) {
    Write-Host "-> Fetching latest desktop release..."
    $releases = Invoke-RestMethod "https://api.github.com/repos/$Repo/releases"
    $latest = $releases | Where-Object { $_.tag_name -like "lite-v*" -and -not $_.prerelease -and -not $_.draft } | Select-Object -First 1
    if (-not $latest) {
        Write-Error "No desktop release found. Check https://github.com/$Repo/releases"
        exit 1
    }
    $Version = $latest.tag_name
}

$Semver = $Version -replace "^lite-v", ""
Write-Host "-> Installing GoClaw Lite v$Semver..."

# Download
$Asset = "goclaw-lite-$Semver-windows-amd64.zip"
$Url = "https://github.com/$Repo/releases/download/$Version/$Asset"
$TmpZip = Join-Path $env:TEMP $Asset

Write-Host "-> Downloading $Url..."
Invoke-WebRequest -Uri $Url -OutFile $TmpZip -UseBasicParsing

# Extract
Write-Host "-> Installing to $InstallDir..."
New-Item -ItemType Directory -Force -Path $InstallDir | Out-Null
Expand-Archive -Path $TmpZip -DestinationPath $InstallDir -Force
Remove-Item $TmpZip -Force

# Create Start Menu shortcut
$ExePath = Join-Path $InstallDir "goclaw-lite.exe"
if (Test-Path $ExePath) {
    $ShortcutPath = Join-Path ([Environment]::GetFolderPath("StartMenu")) "Programs\GoClaw Lite.lnk"
    $Shell = New-Object -ComObject WScript.Shell
    $Shortcut = $Shell.CreateShortcut($ShortcutPath)
    $Shortcut.TargetPath = $ExePath
    $Shortcut.WorkingDirectory = $InstallDir
    $Shortcut.Save()
    Write-Host "-> Start Menu shortcut created"
}

Write-Host ""
Write-Host "GoClaw Lite v$Semver installed to $InstallDir" -ForegroundColor Green
Write-Host ""
Write-Host "To launch: Start Menu -> GoClaw Lite"
Write-Host "Or run: & '$ExePath'"
