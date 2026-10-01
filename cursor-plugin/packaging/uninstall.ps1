[CmdletBinding()]
param (
    [Parameter(ValueFromRemainingArguments = $true)]
    [string[]]$RemainingArgs
)

$ErrorActionPreference = "Stop"

$pluginsDir = if ($env:CLIPROXY_PLUGINS_DIR) { $env:CLIPROXY_PLUGINS_DIR } else { "plugins" }

$i = 0
while ($i -lt $RemainingArgs.Count) {
    $arg = $RemainingArgs[$i]
    if ($arg -in @("--plugins-dir", "-plugins-dir", "-PluginsDir")) {
        $i++
        if ($i -ge $RemainingArgs.Count) {
            Write-Error "--plugins-dir requires a path"
            exit 2
        }
        $pluginsDir = $RemainingArgs[$i]
    }
    elseif ($arg -in @("--help", "-h", "-help")) {
        Write-Host "usage: uninstall.ps1 [--plugins-dir PATH]"
        exit 0
    }
    else {
        Write-Error "unknown argument: $arg"
        exit 2
    }
    $i++
}

$goos = "windows"
$goarch = "amd64"

$resolvedPluginsDir = if ([System.IO.Path]::IsPathRooted($pluginsDir)) {
    $pluginsDir
} else {
    Join-Path (Get-Location).Path $pluginsDir
}

$targetFile = Join-Path $resolvedPluginsDir "$goos\$goarch\cursor.dll"
$fallbackTarget = Join-Path $resolvedPluginsDir "cursor.dll"

$fileToRemove = $null
if (Test-Path -LiteralPath $targetFile) {
    $fileToRemove = $targetFile
} elseif (Test-Path -LiteralPath $fallbackTarget) {
    $fileToRemove = $fallbackTarget
} else {
    Write-Host "cursor plugin is not installed at $targetFile"
    exit 0
}

$removedDir = Join-Path $resolvedPluginsDir ".cursor-uninstalled"
if (-not (Test-Path -LiteralPath $removedDir)) {
    New-Item -ItemType Directory -Path $removedDir -Force | Out-Null
}

$timestamp = (Get-Date).ToUniversalTime().ToString("yyyyMMddTHHmmssZ")
$removedFile = Join-Path $removedDir "cursor-$timestamp.dll"
Move-Item -LiteralPath $fileToRemove -Destination $removedFile -Force

Write-Host "uninstalled cursor plugin; recoverable binary moved to $removedFile"
Write-Host "disable plugins.configs.cursor.enabled, then restart CLIProxyAPI"
