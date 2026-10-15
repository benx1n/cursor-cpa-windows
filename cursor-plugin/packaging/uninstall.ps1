#requires -Version 7.0
[CmdletBinding(SupportsShouldProcess)]
param([string]$PluginsDir = $env:CLIPROXY_PLUGINS_DIR)

$ErrorActionPreference = 'Stop'
if (-not $IsWindows) { throw 'This uninstaller requires Windows.' }
if ([string]::IsNullOrWhiteSpace($PluginsDir)) { $PluginsDir = 'plugins' }
$PluginsDir = [IO.Path]::GetFullPath($PluginsDir)
$pattern = '^cursor(?:-v[0-9][0-9A-Za-z.+-]*)?\.dll$'
$targetDir = Join-Path $PluginsDir 'windows\amd64'
$files = @(Get-ChildItem -LiteralPath $targetDir,$PluginsDir -File -ErrorAction SilentlyContinue | Where-Object Name -Match $pattern)
if ($files.Count -eq 0) {
    Write-Output "cursor plugin is not installed at $PluginsDir"
    return
}

$removedDir = Join-Path (Join-Path $PluginsDir '.cursor-uninstalled') ((Get-Date).ToUniversalTime().ToString('yyyyMMddTHHmmssfffZ') + '-' + [guid]::NewGuid().ToString('N'))
foreach ($file in $files) {
    if (-not $PSCmdlet.ShouldProcess($file.FullName, 'Move Cursor DLL to recoverable uninstall directory')) { continue }
    $location = if ($file.DirectoryName -eq $targetDir) { 'windows-amd64' } else { 'root' }
    $removedLocation = Join-Path $removedDir $location
    New-Item -ItemType Directory -Force -Path $removedLocation | Out-Null
    $removed = Join-Path $removedLocation $file.Name
    Move-Item -LiteralPath $file.FullName -Destination $removed
    Write-Output "uninstalled cursor plugin; recoverable binary moved to $removed"
}
Write-Output 'disable plugins.configs.cursor.enabled, then restart CLIProxyAPI'
