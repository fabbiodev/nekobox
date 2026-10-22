param([string]$NsisPath = 'makensis.exe')
$ErrorActionPreference = 'Stop'
$repo = Split-Path (Split-Path $PSScriptRoot -Parent) -Parent
$fixtureRoot = Join-Path ([IO.Path]::GetTempPath()) ('nekobox-installer-test-' + [guid]::NewGuid().ToString('N'))
$payload = Join-Path $fixtureRoot 'payload'
[void][IO.Directory]::CreateDirectory((Join-Path $payload 'platforms'))
[IO.File]::WriteAllText((Join-Path $payload 'nekobox.exe'), 'new-gui')
[IO.File]::WriteAllText((Join-Path $payload 'platforms/test.dll'), 'plugin')
& go build -o (Join-Path $payload 'nekobox_core.exe') (Join-Path $PSScriptRoot 'installer-stub/main.go')
if ($LASTEXITCODE -ne 0) { throw 'Failed to build installer helper fixture' }
$installerBase = Join-Path $fixtureRoot 'installer'
Push-Location $repo
try {
    & $NsisPath '-V2' '-DSOFTWARE_VERSION=5.11.28.6' '-DSOFTWARE_NAME=NekoBox' ('-DDIRECTORY=' + $payload) ('-DOUTFILE=' + $installerBase) '-DNEKOBOX_INSTALLER_TEST' '-NOCD' 'script/windows_installer.nsi'
    if ($LASTEXITCODE -ne 0) { throw 'NSIS regression installer did not compile' }
} finally { Pop-Location }

function Invoke-FixtureInstaller([string]$target, [bool]$failHelper, [bool]$unpackOnly = $true) {
    $previous = [Environment]::GetEnvironmentVariable('NEKOBOX_TEST_INSTALLER_FAIL', 'Process')
    [Environment]::SetEnvironmentVariable('NEKOBOX_TEST_INSTALLER_FAIL', $(if ($failHelper) { '1' } else { '' }), 'Process')
    try {
        $start = [Diagnostics.ProcessStartInfo]::new()
        $start.FileName = $installerBase + '.exe'
        # NSIS consumes the unquoted /D value as the complete command-line tail.
        $start.Arguments = '/S /UNPACK=' + [int]$unpackOnly + ' /D=' + $target
        $start.UseShellExecute = $false
        $start.CreateNoWindow = $true
        $process = [Diagnostics.Process]::Start($start)
        try {
            if (!$process.WaitForExit(30000)) { $process.Kill(); throw 'Installer fixture hung' }
            return $process.ExitCode
        } finally { $process.Dispose() }
    } finally { [Environment]::SetEnvironmentVariable('NEKOBOX_TEST_INSTALLER_FAIL', $previous, 'Process') }
}

function New-InstalledFixture([string]$name) {
    $target = Join-Path $fixtureRoot $name
    [void][IO.Directory]::CreateDirectory((Join-Path $target 'settings'))
    [IO.File]::WriteAllText((Join-Path $target 'nekobox.exe'), 'old-gui')
    [IO.File]::WriteAllText((Join-Path $target 'nekobox_core.exe'), 'old-core')
    [IO.File]::WriteAllText((Join-Path $target 'settings/routing.json'), 'preserve-my-routes')
    return $target
}
function Assert-PreservedSettings([string]$target) {
    if ([IO.File]::ReadAllText((Join-Path $target 'settings/routing.json')) -ne 'preserve-my-routes') { throw 'Installer changed user routing' }
}

$target = New-InstalledFixture 'Program Files (test) & spaces'
if ((Invoke-FixtureInstaller $target $false) -ne 0) { throw 'Upgrade with spaces failed' }
if ([IO.File]::ReadAllText((Join-Path $target 'nekobox.exe')) -ne 'new-gui') { throw 'GUI was not upgraded' }
if (!(Test-Path -LiteralPath (Join-Path $target 'platforms/test.dll'))) { throw 'Nested payload missing' }
Assert-PreservedSettings $target
if ((Invoke-FixtureInstaller $target $false) -ne 0) { throw 'Reinstall failed' }
Assert-PreservedSettings $target
if ((Invoke-FixtureInstaller $target $false $false) -ne 0) { throw 'Full reinstall with an existing settings directory failed' }
Assert-PreservedSettings $target

$target = New-InstalledFixture 'helper-failure'
if ((Invoke-FixtureInstaller $target $true) -eq 0) { throw 'Installer ignored helper failure' }
if ([IO.File]::ReadAllText((Join-Path $target 'nekobox.exe')) -ne 'old-gui' -or [IO.File]::ReadAllText((Join-Path $target 'nekobox_core.exe')) -ne 'old-core') { throw 'Helper failure damaged previous executables' }
Assert-PreservedSettings $target

$target = New-InstalledFixture 'locked-gui'
$locked = [IO.File]::Open((Join-Path $target 'nekobox.exe'), [IO.FileMode]::Open, [IO.FileAccess]::Read, [IO.FileShare]::Read)
try {
    if ((Invoke-FixtureInstaller $target $false) -eq 0) { throw 'Installer reported success despite a locked GUI' }
} finally { $locked.Dispose() }
if ([IO.File]::ReadAllText((Join-Path $target 'nekobox.exe')) -ne 'old-gui' -or [IO.File]::ReadAllText((Join-Path $target 'nekobox_core.exe')) -ne 'old-core') { throw 'Rollback did not restore previous executable pair' }
Assert-PreservedSettings $target
Write-Host "Installer upgrade, reinstall, nested payload, failure and rollback checks passed: $fixtureRoot"
