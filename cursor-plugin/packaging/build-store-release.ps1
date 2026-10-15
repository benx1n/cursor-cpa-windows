#requires -Version 7.0
[CmdletBinding()]
param(
    [Parameter(Position = 0)]
    [ValidatePattern('^[0-9][0-9A-Za-z.+-]*$')]
    [string]$Version = '0.6.6-windows.2',
    [Parameter(Position = 1)]
    [string]$OutputDir
)

$ErrorActionPreference = 'Stop'
$scriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$projectDir = Split-Path -Parent $scriptDir
if ([string]::IsNullOrWhiteSpace($OutputDir)) { $OutputDir = Join-Path $projectDir 'release' }
$OutputDir = [IO.Path]::GetFullPath($OutputDir)
$binary = if (-not [string]::IsNullOrWhiteSpace($env:CURSOR_PLUGIN_BINARY)) { $env:CURSOR_PLUGIN_BINARY } else { Join-Path $projectDir "cursor-v$Version.dll" }
if (-not (Test-Path -LiteralPath $binary -PathType Leaf)) {
    throw "Windows amd64 plugin binary not found: $binary. Build cursor-v$Version.dll first or set CURSOR_PLUGIN_BINARY."
}

New-Item -ItemType Directory -Force -Path $OutputDir | Out-Null
$assetName = "cursor_${Version}_windows_amd64.zip"
$tempRoot = [IO.Path]::GetFullPath([IO.Path]::GetTempPath())
$staging = Join-Path $tempRoot "cursor-store-$([guid]::NewGuid().ToString('N'))"
New-Item -ItemType Directory -Force -Path $staging | Out-Null
try {
    $storeBinary = Join-Path $staging "cursor-v$Version.dll"
    Copy-Item -LiteralPath $binary -Destination $storeBinary
    $archive = Join-Path $OutputDir $assetName
    if (Test-Path -LiteralPath $archive) { Remove-Item -LiteralPath $archive -Force }
    Compress-Archive -LiteralPath $storeBinary -DestinationPath $archive -CompressionLevel Optimal
    $manualAsset = Join-Path $OutputDir "cursor-plugin-$Version-windows-amd64.zip"
    $assets = @($archive)
    if (Test-Path -LiteralPath $manualAsset) { $assets += $manualAsset }
    $lines = foreach ($asset in $assets) {
        $hash = (Get-FileHash -LiteralPath $asset -Algorithm SHA256).Hash.ToLowerInvariant()
        "$hash  $([IO.Path]::GetFileName($asset))"
    }
    $lines | Set-Content -LiteralPath (Join-Path $OutputDir 'checksums.txt') -Encoding ascii
    Write-Output $archive
    Write-Output (Join-Path $OutputDir 'checksums.txt')
} finally {
    if (([IO.Path]::GetDirectoryName([IO.Path]::GetFullPath($staging))).TrimEnd('\','/') -ne $tempRoot.TrimEnd('\','/')) { throw 'Staging directory is outside the temporary directory.' }
    if (Test-Path -LiteralPath $staging) { Remove-Item -LiteralPath $staging -Recurse -Force }
}
