[CmdletBinding()]
param (
    [string]$Output = ""
)

$ErrorActionPreference = "Stop"

$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$ProjectDir = (Resolve-Path (Join-Path $ScriptDir "..")).Path

if ([string]::IsNullOrWhiteSpace($Output)) {
    $OutputFile = Join-Path $ProjectDir "bin\windows\amd64\cursor.dll"
} else {
    $OutputFile = [System.IO.Path]::GetFullPath($Output)
}

$OutputDir = Split-Path -Parent $OutputFile
if (-not (Test-Path $OutputDir)) {
    New-Item -ItemType Directory -Path $OutputDir -Force | Out-Null
}

Write-Host "Running go vet ./... in $ProjectDir..."
Push-Location $ProjectDir
try {
    $env:CGO_ENABLED = "1"
    $env:GOOS = "windows"
    $env:GOARCH = "amd64"

    & go vet ./...
    if ($LASTEXITCODE -ne 0) {
        throw "go vet failed with exit code $LASTEXITCODE"
    }

    Write-Host "Building Windows amd64 cursor.dll -> $OutputFile..."
    & go build -trimpath -ldflags="-s -w" -buildmode=c-shared -o $OutputFile .
    if ($LASTEXITCODE -ne 0) {
        throw "go build failed with exit code $LASTEXITCODE"
    }

    $HeaderFile = [System.IO.Path]::ChangeExtension($OutputFile, ".h")
    if (Test-Path -LiteralPath $HeaderFile) {
        Remove-Item -LiteralPath $HeaderFile -Force
    }
}
finally {
    Pop-Location
}

if (-not (Test-Path -LiteralPath $OutputFile)) {
    throw "Build completed but output file does not exist: $OutputFile"
}

Write-Host "Verifying exported symbol 'cliproxy_plugin_init'..."
$symbolFound = $false

if (Get-Command objdump -ErrorAction SilentlyContinue) {
    $dumpOutput = & objdump -p $OutputFile 2>&1
    if ($dumpOutput | Select-String -Pattern "cliproxy_plugin_init" -SimpleMatch) {
        $symbolFound = $true
    }
}
elseif (Get-Command dumpbin -ErrorAction SilentlyContinue) {
    $dumpOutput = & dumpbin /exports $OutputFile 2>&1
    if ($dumpOutput | Select-String -Pattern "cliproxy_plugin_init" -SimpleMatch) {
        $symbolFound = $true
    }
}
else {
    $bytes = [System.IO.File]::ReadAllBytes($OutputFile)
    $text = [System.Text.Encoding]::ASCII.GetString($bytes)
    if ($text.Contains("cliproxy_plugin_init")) {
        $symbolFound = $true
    }
}

if (-not $symbolFound) {
    throw "Verification failed: symbol 'cliproxy_plugin_init' was not found in $OutputFile"
}

Write-Host "Build and symbol verification succeeded: $OutputFile"
