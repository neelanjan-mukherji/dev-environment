# Managed by chezmoi from ~/dev-environment. Edit the source, not this file.
# Loaded by both Windows PowerShell and PowerShell 7: their $PROFILE files dot-source
# it (one line, added by setup). Machine-specific additions: local.ps1 next to it.
# Kept light on purpose; real work happens in WSL.

# Configs shared with Linux and macOS live in ~\.config (setup also sets this for
# the user; repeated here for shells started before that took effect).
if (-not $env:XDG_CONFIG_HOME) { $env:XDG_CONFIG_HOME = Join-Path $HOME '.config' }

if (Get-Command starship -ErrorAction SilentlyContinue) {
    Invoke-Expression (& starship init powershell)
}
if (Get-Command zoxide -ErrorAction SilentlyContinue) {
    Invoke-Expression (& zoxide init powershell | Out-String)
}

if (Get-Module -ListAvailable PSReadLine) {
    Set-PSReadLineOption -EditMode Emacs -BellStyle None
    Set-PSReadLineKeyHandler -Key Tab -Function MenuComplete
}

# The same short names as the Linux shells.
Set-Alias -Name vim -Value nvim
Set-Alias -Name lg -Value lazygit
function gs { git status -sb @args }

$local = Join-Path $PSScriptRoot 'local.ps1'
if (Test-Path $local) { . $local }

# Says when the repo has updates (never updates by itself), like the Linux shells.
# After local.ps1, so $env:DEVENV_NO_UPDATE_CHECK = '1' there turns it off.
if (-not $env:DEVENV_NO_UPDATE_CHECK -and (Get-Command dev -ErrorAction SilentlyContinue)) {
    $check = Join-Path $HOME '.local/state/dev/update-check'
    if (Test-Path $check) {
        $behind = 0
        if ([int]::TryParse((Get-Content $check -TotalCount 1), [ref]$behind) -and $behind -gt 0) {
            Write-Host "dev-environment: $behind new commit(s) upstream; run ``dev update`` when convenient."
        }
    }
    if (-not (Test-Path $check) -or ((Get-Date) - (Get-Item $check).LastWriteTime).TotalDays -ge 1) {
        Start-Process dev -ArgumentList 'update', '--check' -WindowStyle Hidden -ErrorAction SilentlyContinue
    }
}
