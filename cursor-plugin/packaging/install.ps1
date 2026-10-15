#requires -Version 7.0
[CmdletBinding(SupportsShouldProcess)]
param([string]$PluginsDir = $env:CLIPROXY_PLUGINS_DIR)

$ErrorActionPreference = 'Stop'
if (-not $IsWindows) { throw 'This installer requires Windows.' }
if ([string]::IsNullOrWhiteSpace($PluginsDir)) { $PluginsDir = 'plugins' }
$PluginsDir = [IO.Path]::GetFullPath($PluginsDir)
$pattern = '^cursor(?:-v[0-9][0-9A-Za-z.+-]*)?\.dll$'
$sourceDir = Join-Path $PSScriptRoot 'bin\windows\amd64'
$sources = @(Get-ChildItem -LiteralPath $sourceDir -File | Where-Object Name -Match $pattern)
if ($sources.Count -ne 1) { throw "Expected one Windows amd64 Cursor DLL in $sourceDir" }
$source = $sources[0]

$manifest = Join-Path $PSScriptRoot 'SHA256SUMS'
$hashLine = @(Get-Content -LiteralPath $manifest | Where-Object { $_ -match ('^[a-fA-F0-9]{64}\s+\*?bin/windows/amd64/' + [regex]::Escape($source.Name) + '$') })
if ($hashLine.Count -ne 1) { throw 'SHA256SUMS must contain exactly one checksum for the Cursor DLL.' }
$expectedHash = ($hashLine[0] -split '\s+', 2)[0]
$sourceHash = (Get-FileHash -LiteralPath $source.FullName -Algorithm SHA256).Hash
if ($sourceHash -ne $expectedHash) { throw 'Cursor DLL checksum does not match SHA256SUMS.' }

$targetDir = Join-Path $PluginsDir 'windows\amd64'
$target = Join-Path $targetDir $source.Name
if (-not $PSCmdlet.ShouldProcess($target, 'Install Cursor DLL and back up previous versions')) { return }
New-Item -ItemType Directory -Force -Path $targetDir | Out-Null
$backupDir = Join-Path (Join-Path $PluginsDir '.cursor-backups') ((Get-Date).ToUniversalTime().ToString('yyyyMMddTHHmmssfffZ') + '-' + [guid]::NewGuid().ToString('N'))
$oldFiles = @(Get-ChildItem -LiteralPath $targetDir,$PluginsDir -File | Where-Object Name -Match $pattern)
$temp = Join-Path $targetDir ".cursor-$([guid]::NewGuid().ToString('N')).tmp"
$backups = @()
$installed = $false
try {
    Copy-Item -LiteralPath $source.FullName -Destination $temp
    if ((Get-FileHash -LiteralPath $temp -Algorithm SHA256).Hash -ne $sourceHash) { throw 'Copied Cursor DLL failed checksum verification.' }
    foreach ($old in $oldFiles) {
        $location = if ($old.DirectoryName -eq $targetDir) { 'windows-amd64' } else { 'root' }
        $backupLocation = Join-Path $backupDir $location
        New-Item -ItemType Directory -Force -Path $backupLocation | Out-Null
        $backup = Join-Path $backupLocation $old.Name
        Copy-Item -LiteralPath $old.FullName -Destination $backup
        $backups += @{ Original = $old.FullName; Backup = $backup }
    }
    Move-Item -LiteralPath $temp -Destination $target -Force
    $installed = $true
    foreach ($old in $oldFiles) {
        if ($old.FullName -ne $target) { Remove-Item -LiteralPath $old.FullName -Force }
    }
} catch {
    $installError = $_
    if ($installed) {
        Remove-Item -LiteralPath $target -Force
        foreach ($backup in $backups) { Copy-Item -LiteralPath $backup.Backup -Destination $backup.Original -Force }
    }
    throw $installError
} finally {
    if (Test-Path -LiteralPath $temp) { Remove-Item -LiteralPath $temp -Force }
}
if ($backups.Count -gt 0) { Write-Output "previous plugins backed up to $backupDir" }
Write-Output "installed cursor plugin at $target"
Write-Output 'enable plugins.enabled and plugins.configs.cursor.enabled, then restart CLIProxyAPI'
