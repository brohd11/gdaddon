# gdaddon installer (Windows).
#
# Run this from inside the unpacked release zip — it installs the gdaddon.exe
# sitting next to this script. It prompts for one of three destinations:
#
#   1) system   %ProgramFiles%\gdaddon          (Machine PATH, requires Administrator)
#   2) user     %LOCALAPPDATA%\Programs\gdaddon  (User PATH)
#   3) gdaddon  %USERPROFILE%\.gdaddon\bin       (not on PATH; for the Godot plugin)
#
# Usage: powershell -ExecutionPolicy Bypass -File install.ps1

$ErrorActionPreference = 'Stop'

$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$Bin = Join-Path $ScriptDir 'gdaddon.exe'

if (-not (Test-Path $Bin)) {
    Write-Error "gdaddon.exe not found next to this script. Run it from the unpacked release zip."
    exit 1
}

function Add-ToPath([string]$Dir, [string]$Scope) {
    # Idempotently append $Dir to the $Scope ('User' or 'Machine') PATH.
    $current = [Environment]::GetEnvironmentVariable('Path', $Scope)
    $parts = @()
    if ($current) { $parts = $current -split ';' }
    if ($parts -contains $Dir) { return }
    $updated = if ($current) { "$current;$Dir" } else { $Dir }
    [Environment]::SetEnvironmentVariable('Path', $updated, $Scope)
}

Write-Host @"
Where should gdaddon be installed?

  1) system   $env:ProgramFiles\gdaddon          Machine PATH, requires Administrator
  2) user     $env:LOCALAPPDATA\Programs\gdaddon  User PATH
  3) gdaddon  $env:USERPROFILE\.gdaddon\bin       not on PATH (launched by the Godot plugin)

"@

$choice = Read-Host "Choose [1/2/3]"

switch ($choice) {
    '1' {
        $isAdmin = ([Security.Principal.WindowsPrincipal] `
            [Security.Principal.WindowsIdentity]::GetCurrent() `
        ).IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)
        if (-not $isAdmin) {
            Write-Error "System install needs an elevated shell. Re-run PowerShell as Administrator."
            exit 1
        }
        $dest = Join-Path $env:ProgramFiles 'gdaddon'
        New-Item -ItemType Directory -Force -Path $dest | Out-Null
        Copy-Item $Bin (Join-Path $dest 'gdaddon.exe') -Force
        Add-ToPath $dest 'Machine'
        Write-Host "installed to $dest\gdaddon.exe"
        Write-Host "open a new terminal for the PATH change to take effect, then run: gdaddon"
    }
    '2' {
        $dest = Join-Path $env:LOCALAPPDATA 'Programs\gdaddon'
        New-Item -ItemType Directory -Force -Path $dest | Out-Null
        Copy-Item $Bin (Join-Path $dest 'gdaddon.exe') -Force
        Add-ToPath $dest 'User'
        Write-Host "installed to $dest\gdaddon.exe"
        Write-Host "open a new terminal for the PATH change to take effect, then run: gdaddon"
    }
    '3' {
        $dest = Join-Path $env:USERPROFILE '.gdaddon\bin'
        New-Item -ItemType Directory -Force -Path $dest | Out-Null
        Copy-Item $Bin (Join-Path $dest 'gdaddon.exe') -Force
        Write-Host "installed to $dest\gdaddon.exe"
        Write-Host "this location is intentionally not on PATH — the Godot plugin launches it"
        Write-Host "directly, or run it with the full path: $dest\gdaddon.exe"
    }
    default {
        Write-Error "invalid choice '$choice'"
        exit 1
    }
}
