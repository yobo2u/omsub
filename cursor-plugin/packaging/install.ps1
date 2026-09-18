[CmdletBinding()]
param (
    [Parameter(ValueFromRemainingArguments = $true)]
    [string[]]$RemainingArgs
)

$ErrorActionPreference = "Stop"

$scriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
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
        Write-Host "usage: install.ps1 [--plugins-dir PATH]"
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

$candidates = @(
    (Join-Path $scriptDir "bin\$goos\$goarch\cursor.dll"),
    (Join-Path $scriptDir "cursor.dll"),
    (Join-Path (Join-Path $scriptDir "..") "bin\$goos\$goarch\cursor.dll"),
    (Join-Path (Join-Path $scriptDir "..") "cursor.dll")
)

$sourceFile = ""
foreach ($cand in $candidates) {
    if (Test-Path -LiteralPath $cand) {
        $sourceFile = (Resolve-Path $cand).Path
        break
    }
}

if ([string]::IsNullOrWhiteSpace($sourceFile)) {
    Write-Error "plugin binary is unavailable for $goos/$goarch (searched $($candidates -join ", "))"
    exit 1
}

$resolvedPluginsDir = if ([System.IO.Path]::IsPathRooted($pluginsDir)) {
    $pluginsDir
} else {
    Join-Path (Get-Location).Path $pluginsDir
}

$targetDir = Join-Path $resolvedPluginsDir "$goos\$goarch"
$targetFile = Join-Path $targetDir "cursor.dll"
$backupDir = Join-Path $resolvedPluginsDir ".cursor-backups"

if (-not (Test-Path -LiteralPath $targetDir)) {
    New-Item -ItemType Directory -Path $targetDir -Force | Out-Null
}
if (-not (Test-Path -LiteralPath $backupDir)) {
    New-Item -ItemType Directory -Path $backupDir -Force | Out-Null
}

if (Test-Path -LiteralPath $targetFile) {
    $timestamp = (Get-Date).ToUniversalTime().ToString("yyyyMMddTHHmmssZ")
    $backupFile = Join-Path $backupDir "cursor-$timestamp.dll"
    Copy-Item -LiteralPath $targetFile -Destination $backupFile -Force
    Write-Host "previous plugin backed up to $backupFile"
}

$temporaryFile = "$targetFile.tmp.$PID"
try {
    Copy-Item -LiteralPath $sourceFile -Destination $temporaryFile -Force
    Move-Item -LiteralPath $temporaryFile -Destination $targetFile -Force
} catch {
    if (Test-Path -LiteralPath $temporaryFile) {
        Remove-Item -LiteralPath $temporaryFile -Force
    }
    throw $_
}

Write-Host "installed cursor plugin at $targetFile"
Write-Host "enable plugins.enabled and plugins.configs.cursor.enabled, then restart CLIProxyAPI"
