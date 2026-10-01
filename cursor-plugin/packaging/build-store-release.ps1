[CmdletBinding()]
param (
    [string]$Version = "0.6.1",
    [string]$OutputDir = "",
    [string]$Binary = ""
)

$ErrorActionPreference = "Stop"

Add-Type -AssemblyName System.IO.Compression.FileSystem

$scriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$projectDir = (Resolve-Path (Join-Path $scriptDir "..")).Path

if ([string]::IsNullOrWhiteSpace($OutputDir)) {
    $OutputDir = Join-Path $projectDir "release"
} else {
    $OutputDir = [System.IO.Path]::GetFullPath($OutputDir)
}

if ([string]::IsNullOrWhiteSpace($Binary)) {
    $candidate1 = Join-Path $projectDir "bin\windows\amd64\cursor.dll"
    $candidate2 = Join-Path $projectDir "cursor.dll"
    if (Test-Path -LiteralPath $candidate1) {
        $Binary = $candidate1
    } elseif (Test-Path -LiteralPath $candidate2) {
        $Binary = $candidate2
    } else {
        Write-Host "Binary not found, running build.ps1..."
        & (Join-Path $scriptDir "build.ps1") -Output $candidate1
        $Binary = $candidate1
    }
} else {
    $Binary = [System.IO.Path]::GetFullPath($Binary)
}

if (-not (Test-Path -LiteralPath $Binary)) {
    throw "Cursor plugin binary not found at $Binary"
}

$assetName = "cursor_${Version}_windows_amd64.zip"
$manualAsset = "cursor-plugin-$Version-windows-amd64.zip"
$stagingBase = Join-Path ([System.IO.Path]::GetTempPath()) ([System.Guid]::NewGuid().ToString("N"))

try {
    New-Item -ItemType Directory -Path $stagingBase -Force | Out-Null
    if (-not (Test-Path -LiteralPath $OutputDir)) {
        New-Item -ItemType Directory -Path $OutputDir -Force | Out-Null
    }

    Copy-Item -LiteralPath $Binary -Destination (Join-Path $stagingBase "cursor.dll") -Force

    $zipFile = Join-Path $OutputDir $assetName
    if (Test-Path -LiteralPath $zipFile) {
        Remove-Item -LiteralPath $zipFile -Force
    }

    [System.IO.Compression.ZipFile]::CreateFromDirectory($stagingBase, $zipFile)

    $checksumEntries = @()
    $storeHash = (Get-FileHash -LiteralPath $zipFile -Algorithm SHA256).Hash.ToLowerInvariant()
    $checksumEntries += "$storeHash  $assetName"

    $manualZip = Join-Path $OutputDir $manualAsset
    if (Test-Path -LiteralPath $manualZip) {
        $manualHash = (Get-FileHash -LiteralPath $manualZip -Algorithm SHA256).Hash.ToLowerInvariant()
        $checksumEntries += "$manualHash  $manualAsset"
    }

    $checksumFile = Join-Path $OutputDir "checksums.txt"
    $checksumText = ($checksumEntries -join "`r`n") + "`r`n"
    [System.IO.File]::WriteAllText($checksumFile, $checksumText, [System.Text.UTF8Encoding]::new($false))

    Write-Host "Store release package created: $zipFile"
    Write-Host "Checksum file created: $checksumFile"
    return $zipFile
}
finally {
    if (Test-Path -LiteralPath $stagingBase) {
        Remove-Item -LiteralPath $stagingBase -Recurse -Force
    }
}
