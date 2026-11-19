# Shared helpers for the Windows install scripts (Windows PowerShell 5.1 and
# PowerShell 7). Included with {{ "{{" }} template "lib.ps1" . {{ "}}" }}.
#
# Same conventions as lib.sh:
# - every system-changing command is echoed before it runs (Invoke-Logged);
# - risky steps ask first (Confirm-Step), and --yes (DEVENV_YES=1) answers yes;
# - something that can't be done is skipped and recorded in
#   ~/.local/state/dev/skipped.log (Add-Skip), not failed;
# - a real failure exits non-zero and is recorded in failed.log under the current
#   Step name; chezmoi retries the script on the next apply.

$ErrorActionPreference = 'Stop'
# Started from PowerShell 7, Windows PowerShell inherits its PSModulePath and then
# fails loading 7's type data on the first module it imports. Use its own default.
if ($PSVersionTable.PSEdition -eq 'Desktop') {
    $env:PSModulePath = @(
        (Join-Path ([Environment]::GetFolderPath('MyDocuments')) 'WindowsPowerShell\Modules'),
        (Join-Path $env:ProgramFiles 'WindowsPowerShell\Modules'),
        (Join-Path $PSHOME 'Modules')
    ) -join ';'
}
$DevState = Join-Path $HOME '.local\state\dev'
New-Item -ItemType Directory -Force -Path $DevState | Out-Null
$script:DevStep = 'install script'

function Step([string]$Name) {
    $script:DevStep = $Name
    Write-Host ''
    Write-Host '==> ' -ForegroundColor Magenta -NoNewline
    Write-Host $Name
}
function Info([string]$Text) { Write-Host "    $Text" }
function Warn([string]$Text) { Write-Host "    ! $Text" -ForegroundColor Yellow }

function Add-Skip([string]$What) {
    $line = '{0}{1}{2}' -f (Get-Date -Format 'yyyy-MM-dd HH:mm:ss'), "`t", $What
    Add-Content -Path (Join-Path $DevState 'skipped.log') -Value $line
    Warn "Skipped: $What"
}

# Show-Command 'command line': echo it as Invoke-Logged does, for a command kept as one string.
function Show-Command([string]$Line) { Write-Host "    `$ $Line" -ForegroundColor DarkGray }

# Invoke-Logged <command> [arguments...]: shows it, runs it, fails on a non-zero exit.
# No named parameters: a command's own flags (e.g. -Command) must pass through.
function Invoke-Logged {
    $command, $arguments = $args
    Show-Command ('{0} {1}' -f $command, ($arguments -join ' '))
    & $command @arguments
    if ($LASTEXITCODE -ne 0) { throw "$command exited with $LASTEXITCODE" }
}

# Test-Native <command> [arguments...]: true when it exits 0; its output is
# discarded. (Windows PowerShell 5.1 turns redirected stderr into errors, which
# $ErrorActionPreference = 'Stop' would throw on.)
function Test-Native {
    $command, $arguments = $args
    $ErrorActionPreference = 'Continue'
    & $command @arguments *> $null
    return $LASTEXITCODE -eq 0
}

function Test-AssumeYes { return $env:DEVENV_YES -eq '1' }
function Test-HaveTty { return -not [Console]::IsInputRedirected }

# Confirm-Step "Question?": yes under --yes; no when there is nobody to ask.
function Confirm-Step([string]$Question) {
    if (Test-AssumeYes) { Info "$Question -> yes (--yes)"; return $true }
    if (-not (Test-HaveTty)) { return $false }
    $answer = Read-Host "    ? $Question [y/N]"
    return $answer -match '^(y|yes)$'
}

# Test-NobodyToAsk <marker> <step name> <skip text>: true when there is nobody to
# ask (no terminal, no --yes); says so once (the step and a skip), then stays
# quiet while <marker> in the state dir exists. False (marker gone) when someone can answer.
function Test-NobodyToAsk([string]$Marker, [string]$StepName, [string]$Skip) {
    $path = Join-Path $DevState $Marker
    if ((Test-AssumeYes) -or (Test-HaveTty)) {
        Remove-Item -Force -ErrorAction SilentlyContinue $path
        return $false
    }
    if (-not (Test-Path $path)) {
        Step $StepName
        New-Item -ItemType File -Force -Path $path | Out-Null
        Add-Skip $Skip
    }
    return $true
}

# Update-Path: pick up what installers added to PATH (a running process keeps its own).
# Off Windows (the pwsh render tests) there is no stored PATH, and the current one stays.
function Update-Path {
    $fresh = @([Environment]::GetEnvironmentVariable('Path', 'Machine'), [Environment]::GetEnvironmentVariable('Path', 'User')) | Where-Object { $_ }
    if ($fresh) { $env:Path = $fresh -join ';' }
}

trap {
    $line = '{0}{1}{2} ({3})' -f (Get-Date -Format 'yyyy-MM-dd HH:mm:ss'), "`t", $script:DevStep, $_
    Add-Content -Path (Join-Path $DevState 'failed.log') -Value $line
    Write-Host "    ! $($script:DevStep) failed: $_" -ForegroundColor Red
    exit 1
}
