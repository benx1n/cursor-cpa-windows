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

$packageName = "cursor-plugin-$Version-windows-amd64"
$tempRoot = [IO.Path]::GetFullPath([IO.Path]::GetTempPath())
$staging = Join-Path $tempRoot "cursor-plugin-$([guid]::NewGuid().ToString('N'))"
$packageDir = Join-Path $staging $packageName
$targetDir = Join-Path $packageDir 'bin\windows\amd64'
New-Item -ItemType Directory -Force -Path $targetDir,$OutputDir | Out-Null
try {
    Copy-Item -LiteralPath $binary -Destination (Join-Path $targetDir "cursor-v$Version.dll")
    Copy-Item -LiteralPath (Join-Path $scriptDir 'install.ps1') -Destination $packageDir
    Copy-Item -LiteralPath (Join-Path $scriptDir 'uninstall.ps1') -Destination $packageDir
    foreach ($file in 'README.md','DESIGN.md','DISCLAIMER.md','LICENSE','THIRD_PARTY_NOTICES.md') {
        Copy-Item -LiteralPath (Join-Path $projectDir $file) -Destination $packageDir
    }
    Copy-Item -LiteralPath (Join-Path $projectDir 'docs') -Destination $packageDir -Recurse
    $hash = (Get-FileHash -LiteralPath (Join-Path $targetDir "cursor-v$Version.dll") -Algorithm SHA256).Hash.ToLowerInvariant()
    "$hash *bin/windows/amd64/cursor-v$Version.dll" | Set-Content -LiteralPath (Join-Path $packageDir 'SHA256SUMS') -Encoding ascii
    $archive = Join-Path $OutputDir "$packageName.zip"
    if (Test-Path -LiteralPath $archive) { Remove-Item -LiteralPath $archive -Force }
    Compress-Archive -LiteralPath $packageDir -DestinationPath $archive -CompressionLevel Optimal
    Write-Output $archive
} finally {
    if (([IO.Path]::GetDirectoryName([IO.Path]::GetFullPath($staging))).TrimEnd('\','/') -ne $tempRoot.TrimEnd('\','/')) { throw 'Staging directory is outside the temporary directory.' }
    if (Test-Path -LiteralPath $staging) { Remove-Item -LiteralPath $staging -Recurse -Force }
}
