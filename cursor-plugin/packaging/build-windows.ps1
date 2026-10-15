#requires -Version 7.0
[CmdletBinding()]
param(
	[ValidatePattern('^[0-9][0-9A-Za-z.+-]*$')]
	[string]$Version = '0.6.6-windows.2',
    [string]$GoCommand = 'go',
    [string]$CCompiler = $env:CC,
    [string]$OutputPath
)

$ErrorActionPreference = 'Stop'
if (-not $IsWindows) { throw 'This build script requires Windows.' }
if ([string]::IsNullOrWhiteSpace($CCompiler)) { $CCompiler = 'gcc' }
$go = (Get-Command $GoCommand -CommandType Application -ErrorAction Stop).Source
$cc = (Get-Command $CCompiler -CommandType Application -ErrorAction Stop).Source
$projectDir = Split-Path -Parent $PSScriptRoot
if ([string]::IsNullOrWhiteSpace($OutputPath)) { $OutputPath = Join-Path $projectDir "cursor-v$Version.dll" }
$OutputPath = [IO.Path]::GetFullPath($OutputPath)
New-Item -ItemType Directory -Force -Path (Split-Path -Parent $OutputPath) | Out-Null

$previous = @{}
foreach ($name in 'CGO_ENABLED','GOOS','GOARCH','CC') { $previous[$name] = [Environment]::GetEnvironmentVariable($name, 'Process') }
Push-Location $projectDir
try {
    $env:CGO_ENABLED = '1'
    $env:GOOS = 'windows'
    $env:GOARCH = 'amd64'
    $env:CC = $cc
    & $go version
    & $go build -trimpath -ldflags "-X cursorplugin/internal/plugin.Version=$Version" -buildmode=c-shared -o $OutputPath .
    if ($LASTEXITCODE -ne 0) { throw "Windows DLL build failed with exit code $LASTEXITCODE" }
    Write-Output $OutputPath
} finally {
    Pop-Location
    foreach ($name in $previous.Keys) { [Environment]::SetEnvironmentVariable($name, $previous[$name], 'Process') }
}
