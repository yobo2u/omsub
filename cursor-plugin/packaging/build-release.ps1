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

$packageName = "cursor-plugin-$Version-windows-amd64"
$stagingBase = Join-Path ([System.IO.Path]::GetTempPath()) ([System.Guid]::NewGuid().ToString("N"))
$packageDir = Join-Path $stagingBase $packageName

try {
    $binDir = Join-Path $packageDir "bin\windows\amd64"
    New-Item -ItemType Directory -Path $binDir -Force | Out-Null
    if (-not (Test-Path -LiteralPath $OutputDir)) {
        New-Item -ItemType Directory -Path $OutputDir -Force | Out-Null
    }

    Copy-Item -LiteralPath $Binary -Destination (Join-Path $binDir "cursor.dll") -Force
    Copy-Item -LiteralPath (Join-Path $scriptDir "install.ps1") -Destination (Join-Path $packageDir "install.ps1") -Force
    Copy-Item -LiteralPath (Join-Path $scriptDir "uninstall.ps1") -Destination (Join-Path $packageDir "uninstall.ps1") -Force

    foreach ($doc in @("README.md", "DESIGN.md", "DISCLAIMER.md", "LICENSE", "THIRD_PARTY_NOTICES.md")) {
        $src = Join-Path $projectDir $doc
        if (Test-Path -LiteralPath $src) {
            Copy-Item -LiteralPath $src -Destination (Join-Path $packageDir $doc) -Force
        }
    }

    $docsSrc = Join-Path $projectDir "docs"
    if (Test-Path -LiteralPath $docsSrc) {
        Copy-Item -LiteralPath $docsSrc -Destination (Join-Path $packageDir "docs") -Recurse -Force
    }

    $dllRelative = "bin/windows/amd64/cursor.dll"
    $dllPath = Join-Path $packageDir "bin\windows\amd64\cursor.dll"
    $hash = (Get-FileHash -LiteralPath $dllPath -Algorithm SHA256).Hash.ToLowerInvariant()
    $shaContent = "$hash  $dllRelative`r`n"
    [System.IO.File]::WriteAllText((Join-Path $packageDir "SHA256SUMS"), $shaContent, [System.Text.UTF8Encoding]::new($false))

    $zipFile = Join-Path $OutputDir "$packageName.zip"
    if (Test-Path -LiteralPath $zipFile) {
        Remove-Item -LiteralPath $zipFile -Force
    }

    [System.IO.Compression.ZipFile]::CreateFromDirectory($packageDir, $zipFile)
    Write-Host "Release package created: $zipFile"
    return $zipFile
}
finally {
    if (Test-Path -LiteralPath $stagingBase) {
        Remove-Item -LiteralPath $stagingBase -Recurse -Force
    }
}
