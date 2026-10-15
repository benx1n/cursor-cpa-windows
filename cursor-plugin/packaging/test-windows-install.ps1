#requires -Version 7.0
[CmdletBinding()]
param([Parameter(Mandatory)][string]$Archive)

$ErrorActionPreference = 'Stop'
$tempRoot = [IO.Path]::GetFullPath([IO.Path]::GetTempPath())
$root = Join-Path $tempRoot "cursor-install-test-$([guid]::NewGuid().ToString('N'))"
$plugins = Join-Path $root 'plugins [test]'
function Assert([bool]$Condition, [string]$Message) {
    if (-not $Condition) { throw $Message }
}
try {
    Expand-Archive -LiteralPath $Archive -DestinationPath $root
    $package = @(Get-ChildItem -LiteralPath $root -Directory)
    Assert ($package.Count -eq 1) 'Expected one package directory.'
    $installer = Join-Path $package[0].FullName 'install.ps1'
    $uninstaller = Join-Path $package[0].FullName 'uninstall.ps1'
    $binary = Get-ChildItem -LiteralPath (Join-Path $package[0].FullName 'bin\windows\amd64') -File | Select-Object -First 1
    $targetDir = Join-Path $plugins 'windows\amd64'
    $target = Join-Path $targetDir $binary.Name

    & $installer -PluginsDir $plugins -WhatIf
    Assert (-not (Test-Path -LiteralPath $plugins)) 'WhatIf created plugin files.'
    & $installer -PluginsDir $plugins
    $expected = (Get-FileHash -LiteralPath $binary.FullName).Hash
    Assert ((Get-FileHash -LiteralPath $target).Hash -eq $expected) 'Installed DLL checksum differs.'

    $unrelated = Join-Path $targetDir 'cursor-helper.dll'
    'unrelated plugin' | Set-Content -LiteralPath $unrelated
    $oldRoot = Join-Path $plugins 'cursor.dll'
    'old root plugin' | Set-Content -LiteralPath $oldRoot
    & $installer -PluginsDir $plugins
    Assert (-not (Test-Path -LiteralPath $oldRoot)) 'Old root DLL remained active.'
    Assert (Test-Path -LiteralPath $unrelated) 'Installer modified an unrelated DLL.'
    $backups = @(Get-ChildItem -LiteralPath (Join-Path $plugins '.cursor-backups') -Recurse -File)
    Assert ($backups.Count -eq 2) 'Upgrade did not back up both old DLLs.'

    # A damaged package must fail before modifying the existing installation.
    $original = [IO.File]::ReadAllBytes($binary.FullName)
    [IO.File]::WriteAllText($binary.FullName, 'damaged binary')
    $rejected = $false
    try { & $installer -PluginsDir $plugins } catch { $rejected = $_.Exception.Message -like '*checksum*' }
    Assert $rejected 'Installer accepted a damaged package.'
    Assert ((Get-FileHash -LiteralPath $target).Hash -eq $expected) 'Failed installation changed the active DLL.'
    [IO.File]::WriteAllBytes($binary.FullName, $original)

    & $uninstaller -PluginsDir $plugins -WhatIf
    Assert (Test-Path -LiteralPath $target) 'Uninstall WhatIf removed the DLL.'
    & $uninstaller -PluginsDir $plugins
    Assert (-not (Test-Path -LiteralPath $target)) 'Uninstall left the active DLL.'
    Assert (Test-Path -LiteralPath $unrelated) 'Uninstaller modified an unrelated DLL.'
    $removed = @(Get-ChildItem -LiteralPath (Join-Path $plugins '.cursor-uninstalled') -Recurse -File)
    Assert ($removed.Count -eq 1) 'Uninstalled DLL is not recoverable.'
    Assert ((Get-FileHash -LiteralPath $removed[0].FullName).Hash -eq $expected) 'Recoverable DLL checksum differs.'
    Write-Output 'Windows install, upgrade, checksum rejection and uninstall passed.'
} finally {
    if (([IO.Path]::GetDirectoryName([IO.Path]::GetFullPath($root))).TrimEnd('\','/') -ne $tempRoot.TrimEnd('\','/')) { throw 'Test directory is outside the temporary directory.' }
    if (Test-Path -LiteralPath $root) { Remove-Item -LiteralPath $root -Recurse -Force }
}
