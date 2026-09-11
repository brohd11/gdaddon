# Install gdaddon into %LOCALAPPDATA%\bin.
#
#   irm https://raw.githubusercontent.com/brohd11/gdaddon/main/install.ps1 | iex
#
# Env overrides (set them before the pipeline):
#   $env:BIN_DIR = 'C:\tools'    install target   (default: %LOCALAPPDATA%\bin)
#   $env:VERSION = 'v0.1.1'      pin a release    (default: latest)
#
# `iex` cannot pass arguments, so flags need the scriptblock form:
#   & ([scriptblock]::Create((irm <url>))) -NoModifyPath
#
# Body below "end config" is shared with
# ~/dotfiles/.misc/scripts/bash/install_stamp/simple/install.template.ps1 -- edit the
# template and re-stamp with render-installers.sh rather than editing it here.

# ---- config ----
$Repo       = 'brohd11/gdaddon'
$Binary     = 'gdaddon'
$ArchiveExt = 'zip'                                # zip | tar.gz
$Supported  = 'windows-amd64'

# Printed after a successful install. Leave the body empty for nothing.
function Write-PostInstallNote {
    Write-Host "Run '$Binary' in a Godot project to manage its addons."
    Write-Host "Later updates: '$Binary update'."
    Write-Host ""
    Write-Host "To also place a copy where the Godot EditorPlugin looks for it (~/.gdaddon/bin):"
    Write-Host ('  $env:BIN_DIR = "$env:USERPROFILE\.gdaddon\bin"; irm https://raw.githubusercontent.com/' + $Repo + '/main/install.ps1 | iex')
}
# ---- end config ----

$ErrorActionPreference = 'Stop'

# Invoke-WebRequest renders a progress bar per chunk on Windows PowerShell 5.1, which
# costs roughly an order of magnitude on the download. Silencing it is not cosmetic.
$ProgressPreference = 'SilentlyContinue'

# Is this running from a file (powershell -File install.ps1) or out of `iex`? Under
# `iex` there is no script path -- and, more to the point, no separate process: a bare
# `exit` would close the user's shell. So nothing below ever calls exit; failures throw
# and are turned into an exit code at the bottom, only when there is a file to exit from.
$FromFile = [bool]$MyInvocation.MyCommand.Path

function Write-Err([string]$Message) {
    [Console]::Error.WriteLine("error: $Message")
}

function Show-Help {
    # Not derived from the script path: under `irm | iex` there is no file to name.
    Write-Host @"
install $Binary into `$env:BIN_DIR (default: %LOCALAPPDATA%\bin)

  `$env:BIN_DIR = '<dir>'    install target
  `$env:VERSION = '<tag>'    pin a release (default: latest)

Flags (need the scriptblock form -- iex cannot take arguments):
  & ([scriptblock]::Create((irm <url>))) -NoModifyPath

  -ModifyPath      update the user PATH without prompting
  -NoModifyPath    never touch the user PATH
"@
}

# Parse into a hashtable rather than reading $args directly: $args inside a function is
# that function's own, so the caller has to hand it over explicitly.
function Read-Options([string[]]$Arguments) {
    # auto   = prompt when there is an interactive host, otherwise just print the line
    # never  = -NoModifyPath, never touch the registry
    # always = -ModifyPath, write without prompting even with no host to prompt (for env
    #          setup scripts, and for self-update, which has no terminal to answer with)
    $opts = @{ PathMode = 'auto'; Help = $false }

    foreach ($arg in $Arguments) {
        # Both spellings: --kebab matches install.sh, so callers driving either script
        # (goutil/selfupdate does) pass one flag; -Pascal is what a PowerShell user types.
        # No break/continue in here on purpose: inside a loop those are ambiguous about
        # which construct they leave. The patterns are mutually exclusive, and `default`
        # fires only when nothing else matched, so falling through costs nothing.
        switch -Regex ($arg) {
            '^(--no-modify-path|-+NoModifyPath)$' { $opts.PathMode = 'never'  }
            '^(--modify-path|-+ModifyPath)$'      { $opts.PathMode = 'always' }
            '^(-h|-+help|/\?)$'                   { $opts.Help = $true        }
            default { throw "unknown option: $arg" }
        }
    }
    return $opts
}

# --- platform ----------------------------------------------------------------

function Get-Target {
    # PROCESSOR_ARCHITECTURE reports the architecture of the *current process*, so a
    # 32-bit PowerShell on 64-bit Windows says x86. PROCESSOR_ARCHITEW6432 is set only in
    # that case and carries the real machine architecture, so it wins when present.
    $arch = if ($env:PROCESSOR_ARCHITEW6432) { $env:PROCESSOR_ARCHITEW6432 } else { $env:PROCESSOR_ARCHITECTURE }

    switch ($arch) {
        'AMD64' { $goarch = 'amd64' }
        'ARM64' { $goarch = 'arm64' }
        default { throw "unsupported architecture: $arch" }
    }

    $target = "windows-$goarch"
    $supported = $Supported -split '\s+' | Where-Object { $_ }

    if ($supported -contains $target) { return $target }

    # ARM64 Windows runs x64 binaries under emulation, so a missing arm64 build is a
    # note rather than a dead end. Anything else fails here with the list, instead of
    # surfacing as a 404 on the download.
    if ($goarch -eq 'arm64' -and $supported -contains 'windows-amd64') {
        Write-Host "no windows-arm64 build is published; installing the amd64 one (runs under emulation)"
        return 'windows-amd64'
    }

    throw "no $target build is published for $Binary
  supported: $Supported
  build from source: https://github.com/$Repo"
}

function Get-AssetUrl([string]$Target, [string]$Version) {
    $asset = "$Binary-$Target.$ArchiveExt"
    # Asset names are deliberately version-less so the /latest/download redirect works
    # and we never touch the GitHub API (no JSON parsing, no rate limit).
    if ($Version -eq 'latest') {
        return "https://github.com/$Repo/releases/latest/download/$asset"
    }
    return "https://github.com/$Repo/releases/download/$Version/$asset"
}

# --- PATH --------------------------------------------------------------------

# Broadcast that the environment changed. [Environment]::SetEnvironmentVariable does this
# for you, but it cannot write REG_EXPAND_SZ (see Add-ToUserPath), so we write the
# registry directly and send the message ourselves. Without it, new terminals keep
# inheriting Explorer's stale environment block until the user signs out.
function Send-EnvironmentChange {
    if (-not ('InstallScripts.Native' -as [type])) {
        # Add-Type is per-session and throws if the type already exists, which it does
        # when a second installer runs in the same shell -- hence the guard above.
        Add-Type -Namespace InstallScripts -Name Native -MemberDefinition @'
[DllImport("user32.dll", SetLastError = true, CharSet = CharSet.Auto)]
public static extern IntPtr SendMessageTimeout(
    IntPtr hWnd, uint Msg, UIntPtr wParam, string lParam,
    uint fuFlags, uint uTimeout, out UIntPtr lpdwResult);
'@
    }
    $HWND_BROADCAST   = [IntPtr]0xffff
    $WM_SETTINGCHANGE = 0x1a
    $SMTO_ABORTIFHUNG = 0x2
    $result = [UIntPtr]::Zero
    # A hung top-level window must not hang the installer, hence the timeout and
    # ABORTIFHUNG. The result is advisory; nothing here depends on it.
    [void][InstallScripts.Native]::SendMessageTimeout(
        $HWND_BROADCAST, $WM_SETTINGCHANGE, [UIntPtr]::Zero, 'Environment',
        $SMTO_ABORTIFHUNG, 5000, [ref]$result)
}

# Can we actually ask the user something? UserInteractive alone is not enough: it reports
# whether the process has a window station, not whether anything is listening on stdin, so
# it is true for a script launched with its input redirected -- self-update, CI, a service.
# Checking the redirect too is this script's equivalent of install.sh probing /dev/tty.
function Test-CanPrompt {
    try {
        return [Environment]::UserInteractive -and -not [Console]::IsInputRedirected
    } catch {
        return $false
    }
}

# Does the user PATH already point at $Dir? Entries are compared expanded, so an existing
# %LOCALAPPDATA%\bin and a literal C:\Users\me\AppData\Local\bin count as the same place.
function Test-OnPathValue([string]$PathValue, [string]$Dir) {
    if (-not $PathValue) { return $false }
    $want = ([Environment]::ExpandEnvironmentVariables($Dir)).TrimEnd('\', '/')
    foreach ($entry in $PathValue -split ';') {
        if (-not $entry) { continue }
        $have = ([Environment]::ExpandEnvironmentVariables($entry)).TrimEnd('\', '/')
        if ($have -ieq $want) { return $true }
    }
    return $false
}

# Append $DirRaw to the *user* PATH -- never the machine one, which needs admin.
#
# Deliberately not [Environment]::{Get,Set}EnvironmentVariable:
#   - Get expands %VAR% references on the way out, so a read-append-write would flatten
#     every %USERPROFILE%-style entry the user already had into a hardcoded path.
#   - Set gives no guarantee of REG_EXPAND_SZ, which is what makes the %LOCALAPPDATA%\bin
#     we add expand at environment-build time instead of sitting there as a literal.
# Reading with DoNotExpandEnvironmentNames and writing ExpandString gets both right.
function Add-ToUserPath([string]$DirRaw) {
    $key = [Microsoft.Win32.Registry]::CurrentUser.OpenSubKey('Environment', $true)
    if (-not $key) { throw "cannot open HKCU:\Environment" }
    try {
        $current = [string]$key.GetValue('Path', '', [Microsoft.Win32.RegistryValueOptions]::DoNotExpandEnvironmentNames)

        if (Test-OnPathValue $current $DirRaw) {
            Write-Host "your user PATH already references $DirRaw -- leaving it alone"
            return $false
        }

        $updated = if ($current.TrimEnd(';')) { $current.TrimEnd(';') + ';' + $DirRaw } else { $DirRaw }
        $key.SetValue('Path', $updated, [Microsoft.Win32.RegistryValueKind]::ExpandString)
    } finally {
        $key.Close()
    }

    Send-EnvironmentChange
    Write-Host "added $DirRaw to your user PATH"
    return $true
}

# --- install -----------------------------------------------------------------

function Invoke-Install([hashtable]$Options, [string]$BinDirRaw, [string]$BinDir, [string]$Version) {
    $target = Get-Target
    $url    = Get-AssetUrl $target $Version

    try {
        New-Item -ItemType Directory -Path $BinDir -Force | Out-Null
    } catch {
        throw "cannot create $BinDir (set `$env:BIN_DIR to somewhere you own)"
    }

    Write-Host "downloading $Binary ($target, $Version)"

    # Stage in a temp dir so a failed download can't leave a half-written binary in
    # place of a working one.
    $tmp = Join-Path ([IO.Path]::GetTempPath()) ("install-$Binary-" + [Guid]::NewGuid().ToString('N'))
    New-Item -ItemType Directory -Path $tmp -Force | Out-Null
    try {
        $archive = Join-Path $tmp "$Binary-$target.$ArchiveExt"

        # Windows PowerShell 5.1 defaults to a protocol set that github.com no longer
        # accepts, and the failure looks like a connection reset rather than anything
        # about TLS. Newer hosts negotiate this themselves, so failure here is ignorable.
        try {
            [Net.ServicePointManager]::SecurityProtocol =
                [Net.ServicePointManager]::SecurityProtocol -bor [Net.SecurityProtocolType]::Tls12
        } catch { }

        try {
            Invoke-WebRequest -Uri $url -OutFile $archive -UseBasicParsing
        } catch {
            throw "download failed: $url
  (check https://github.com/$Repo/releases for available versions)"
        }

        switch ($ArchiveExt) {
            'zip'    { Expand-Archive -LiteralPath $archive -DestinationPath $tmp -Force }
            'tar.gz' { & tar.exe -xzf $archive -C $tmp; if ($LASTEXITCODE -ne 0) { throw "could not extract $archive" } }
            default  { throw "bad ArchiveExt in this script: $ArchiveExt" }
        }

        $staged = Join-Path $tmp "$Binary.exe"
        if (-not (Test-Path -LiteralPath $staged)) { throw "archive did not contain $Binary.exe" }

        # A zip fetched over HTTP carries the Mark of the Web, which the extracted .exe
        # inherits -- SmartScreen then blocks it on first run. This is the Windows analog
        # of clearing com.apple.quarantine on macOS.
        Unblock-File -LiteralPath $staged -ErrorAction SilentlyContinue

        $dest = Join-Path $BinDir "$Binary.exe"
        $old  = "$dest.old"

        # Windows locks a running .exe against deletion and overwrite, but renaming one is
        # allowed -- so displace the old binary instead of replacing it. That is what lets
        # `$Binary update` overwrite itself while running. The stale .old is cleared by the
        # next run, once nothing holds it open.
        Remove-Item -LiteralPath $old -Force -ErrorAction SilentlyContinue
        if (Test-Path -LiteralPath $dest) {
            Move-Item -LiteralPath $dest -Destination $old -Force
        }
        Move-Item -LiteralPath $staged -Destination $dest -Force

        Write-Host "installed -> $dest"
    } finally {
        Remove-Item -LiteralPath $tmp -Recurse -Force -ErrorAction SilentlyContinue
    }

    Set-PathEntry $Options $BinDirRaw $BinDir
    Write-Host ""
    Write-PostInstallNote
}

function Set-PathEntry([hashtable]$Options, [string]$BinDirRaw, [string]$BinDir) {
    # $env:Path is the merged machine+user+process value -- the right thing to test
    # against, since it is exactly what decides whether the command runs by name now.
    if (Test-OnPathValue $env:Path $BinDir) { return }

    Write-Host ""
    Write-Host "$BinDir is not on your PATH, so '$Binary' won't be runnable by name."

    $session_line = "`$env:Path += ';$BinDir'"
    $added = $false

    if ($Options.PathMode -eq 'never') {
        Write-Host "add it for this session with:"
        Write-Host "  $session_line"
        # Deliberately not suggesting setx: it writes the value it is given to the *user*
        # PATH, so the natural `setx PATH "$env:Path;..."` copies every machine entry into
        # the user's, and it silently truncates at 1024 characters. -ModifyPath appends one
        # entry to the user PATH and touches nothing else.
        Write-Host "to make it permanent, re-run with -ModifyPath, or add $BinDirRaw under"
        Write-Host "Settings > System > About > Advanced system settings > Environment Variables."
        return
    }

    if ($Options.PathMode -eq 'always') {
        $added = Add-ToUserPath $BinDirRaw
    } elseif (Test-CanPrompt) {
        # Unlike `curl | sh`, there is no /dev/tty problem here: `iex` runs in-process, so
        # stdin is the user's console and Read-Host just works.
        $reply = Read-Host "Add it to your user PATH? [y/N]"
        if ($reply -imatch '^y(es)?$') {
            $added = Add-ToUserPath $BinDirRaw
        } else {
            Write-Host "skipped."
        }
    } else {
        # No interactive host (CI, a service, self-update) and not explicitly asked via
        # -ModifyPath: never edit the user's environment unasked.
        Write-Host "run this installer with -ModifyPath to add it, or add it yourself."
    }

    if ($added) {
        # The registry write only reaches processes started after the broadcast, so the
        # shell that ran this still needs its own copy.
        Write-Host "open a new terminal, or run this in the current one:"
        Write-Host "  $session_line"
    } else {
        Write-Host "to use it in this session:"
        Write-Host "  $session_line"
    }
}

# --- entry -------------------------------------------------------------------

$code = 0
try {
    # $args is read here, at script scope, because inside a function it would be that
    # function's own arguments.
    $options = Read-Options $args

    if ($options.Help) {
        Show-Help
    } else {
        # Two forms of the install directory are kept side by side: the raw one, which may
        # still contain %VAR% references and is what gets written to the registry, and the
        # expanded one, which is what the filesystem needs.
        $binDirRaw = if ($env:BIN_DIR) { $env:BIN_DIR } else { '%LOCALAPPDATA%\bin' }
        $binDir = [Environment]::ExpandEnvironmentVariables($binDirRaw)

        # ExpandEnvironmentVariables leaves unknown names alone, so a leftover % means
        # LOCALAPPDATA was not set -- rare, but it would otherwise create a directory
        # literally named "%LOCALAPPDATA%".
        if ($binDir -like '*%*') {
            $binDirRaw = Join-Path $env:USERPROFILE 'AppData\Local\bin'
            $binDir = $binDirRaw
        }

        $version = if ($env:VERSION) { $env:VERSION } else { 'latest' }

        Invoke-Install $options $binDirRaw $binDir $version
    }
} catch {
    Write-Err $_.Exception.Message
    $code = 1
}

# Only a real script process has an exit code worth setting. Under `irm | iex` there is
# no separate process, and `exit` would take the user's shell down with it.
if ($FromFile) { exit $code }
