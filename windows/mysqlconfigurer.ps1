# Compatibility entry point. Keep parameter aliases and switch values intact.
[CmdletBinding()]
param(
    [Alias('a')]
    [switch]$Apply,
    [Alias('Automatic')]
    [switch]$NonInteractive,
    [switch]$NoRestart,
    [switch]$ForceRestart,
    [ValidateSet('dynamic', 'full')]
    [string]$ApplyMode = 'full',
    [switch]$QueueApply,
    [Alias('r')]
    [switch]$Rollback,
    [Alias('p')]
    [switch]$Configure,
    [Alias('u')]
    [switch]$Update,
    [Alias('k')]
    [string]$ApiKey
)

$ErrorActionPreference = 'Stop'
$implementation = Join-Path $PSScriptRoot 'releem.ps1'
if (-not (Test-Path -LiteralPath $implementation -PathType Leaf)) {
    Write-Error 'releem.ps1 is missing. Reinstall or update Releem Agent.' -ErrorAction Continue
    exit 1
}
& $implementation @PSBoundParameters
exit $LASTEXITCODE
