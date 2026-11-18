# Bootstrap for the Windows host: puts dev.exe in ~\.local\bin, then runs `dev setup`.
#
#   irm https://raw.githubusercontent.com/neelanjan-mukherji/dev-environment/main/install.ps1 | iex
#   & ([scriptblock]::Create((irm https://raw.githubusercontent.com/neelanjan-mukherji/dev-environment/main/install.ps1))) --yes
#   .\install.ps1 --from-source      # build dev from this clone (needs Go)
#
# Every other argument goes to `dev setup` (dev setup --help). Where dev comes from:
#   DEVENV_DEV_BIN=<path>      use this binary
#   DEVENV_DEV_VERSION=<tag>   this release instead of the newest one
#   DEVENV_RELEASE_URL=<url>   download from here (releases/download/<tag>)
# The download is checked against the release's checksums.txt.
# The Linux side (WSL) uses install.sh; dev setup offers it at the end.

# Everything runs in its own scope: under `irm | iex` this is the person's own
# session, so no `exit` (it would close their window) and no leftover settings.
& {
$ErrorActionPreference = 'Stop'
$ProgressPreference = 'SilentlyContinue'   # Invoke-WebRequest is slow with the progress bar
# Windows PowerShell 5.1 may not offer TLS 1.2, which GitHub requires.
[Net.ServicePointManager]::SecurityProtocol = [Net.ServicePointManager]::SecurityProtocol -bor [Net.SecurityProtocolType]::Tls12
$Repo = 'neelanjan-mukherji/dev-environment'
$BinDir = Join-Path $HOME '.local\bin'

function Say([string]$Text) { Write-Host '==> ' -ForegroundColor Blue -NoNewline; Write-Host $Text }
function Die([string]$Text) { throw "install.ps1: $Text" }

$fromSource = $false
$setupArgs = @()
foreach ($a in $args) {
    switch ($a) {
        '--from-source' { $fromSource = $true }
        { $_ -in '-h', '--help' } {
            Write-Host @'
Usage: install.ps1 [--from-source] [dev setup options]

Puts the `dev` CLI in ~\.local\bin, then runs `dev setup` with the options.
  --from-source   build dev from this clone (needs Go) instead of downloading
Where dev comes from otherwise:
  DEVENV_DEV_BIN=<path>    use this binary
  DEVENV_DEV_VERSION=<tag> this release instead of the newest one
  (the download is checked against the release's checksums.txt)
For the setup options: dev setup --help
'@
            return
        }
        default { $setupArgs += $a }
    }
}

# This clone, when run from one (irm | iex has no file, hence no clone).
$clone = $null
if ($PSScriptRoot -and (Test-Path (Join-Path $PSScriptRoot 'home\.chezmoi.toml.tmpl'))) { $clone = $PSScriptRoot }

$arch = switch ($env:PROCESSOR_ARCHITECTURE) {
    'AMD64' { 'amd64' }
    'ARM64' { 'arm64' }
    default { Die "Unsupported CPU: $($env:PROCESSOR_ARCHITECTURE)" }
}

function Get-Release {
    $base = $env:DEVENV_RELEASE_URL
    if (-not $base) {
        $tag = $env:DEVENV_DEV_VERSION
        if (-not $tag) {
            # The newest release, prereleases included (v2 is in alpha).
            $releases = Invoke-RestMethod "https://api.github.com/repos/$Repo/releases?per_page=1"
            if (-not $releases) { Die 'No release published yet; clone the repo and run .\install.ps1 --from-source.' }
            $tag = @($releases)[0].tag_name
        }
        $base = "https://github.com/$Repo/releases/download/$tag"
    }
    $asset = "dev_windows_$arch.zip"
    $tmp = Join-Path ([IO.Path]::GetTempPath()) ("dev-" + [guid]::NewGuid())
    New-Item -ItemType Directory -Path $tmp | Out-Null
    try {
        Say "Downloading dev ($asset)"
        Invoke-WebRequest -UseBasicParsing "$base/$asset" -OutFile (Join-Path $tmp $asset)
        Invoke-WebRequest -UseBasicParsing "$base/checksums.txt" -OutFile (Join-Path $tmp 'checksums.txt')
        $want = Get-Content (Join-Path $tmp 'checksums.txt') |
            Where-Object { ($_ -split '\s+')[1] -in @($asset, "*$asset") } |
            ForEach-Object { ($_ -split '\s+')[0] } | Select-Object -First 1
        if (-not $want) { Die "checksums.txt has no entry for $asset." }
        $got = (Get-FileHash -Algorithm SHA256 (Join-Path $tmp $asset)).Hash
        if ($got -ne $want) { Die "Checksum mismatch for $asset; not installing it." }
        Expand-Archive -Path (Join-Path $tmp $asset) -DestinationPath $tmp -Force
        New-Item -ItemType Directory -Force -Path $BinDir | Out-Null
        Copy-Item (Join-Path $tmp 'dev.exe') (Join-Path $BinDir 'dev.exe') -Force
    } finally {
        Remove-Item -Recurse -Force $tmp -ErrorAction SilentlyContinue
    }
}

if ($env:DEVENV_DEV_BIN) {
    $dev = $env:DEVENV_DEV_BIN
} elseif ($fromSource) {
    if (-not $clone) { Die "--from-source needs a clone: git clone https://github.com/$Repo; .\dev-environment\install.ps1 --from-source" }
    if (-not (Get-Command go -ErrorAction SilentlyContinue)) { Die '--from-source needs Go on PATH (winget install GoLang.Go).' }
    Say "Building dev from $clone\cli"
    New-Item -ItemType Directory -Force -Path $BinDir | Out-Null
    Push-Location (Join-Path $clone 'cli')
    try {
        $env:CGO_ENABLED = '0'
        go build -trimpath -o (Join-Path $BinDir 'dev.exe') .
        if ($LASTEXITCODE -ne 0) { Die 'go build failed.' }
    } finally { Pop-Location }
    $dev = Join-Path $BinDir 'dev.exe'
} else {
    Get-Release
    $dev = Join-Path $BinDir 'dev.exe'
}

# ~\.local\bin on the user's PATH, for new terminals and this one. Read and written
# unexpanded, so entries like %USERPROFILE%\bin stay as they are.
if ($IsWindows -or $env:OS -eq 'Windows_NT') {
    $key = Get-Item 'HKCU:\Environment'
    $userPath = $key.GetValue('Path', '', 'DoNotExpandEnvironmentNames')
    if (-not (($userPath -split ';') -contains $BinDir)) {
        Say "Adding $BinDir to your PATH"
        Set-ItemProperty -Path 'HKCU:\Environment' -Name Path -Type ExpandString -Value ((@($BinDir, $userPath) | Where-Object { $_ }) -join ';')
    }
}
if (-not (($env:Path -split ';') -contains $BinDir)) { $env:Path = "$BinDir;$env:Path" }

if ($clone) { $setupArgs += @('--source', $clone) }
& $dev setup @setupArgs
# $LASTEXITCODE is dev's; a CI step (or the caller) reads it.
} @args
