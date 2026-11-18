# Run after an install on Windows (the CI job, or by hand): `dev doctor` asserts
# the host matches its setup; this adds what only makes sense right after an
# install: a second install succeeds and is a no-op (no install step runs again).
# Usage: ci\verify.ps1
$ErrorActionPreference = 'Continue'
$repo = Split-Path -Parent $PSScriptRoot
$env:Path = [Environment]::GetEnvironmentVariable('Path', 'Machine') + ';' + [Environment]::GetEnvironmentVariable('Path', 'User')
$fail = 0
function Check([string]$Name, [bool]$Ok, [string]$Detail = '') {
    if ($Ok) { Write-Host "ok   $Name" } else { Write-Host "FAIL $Name"; if ($Detail) { $Detail -split "`n" | ForEach-Object { "    $_" } }; $script:fail++ }
}

dev doctor
Check 'dev doctor' ($LASTEXITCODE -eq 0)

$second = (dev setup --yes --source $repo 2>&1 | Out-String)
Check 'second install succeeds' ($LASTEXITCODE -eq 0) $second
# Step headers of the install scripts; dev's own steps (WSL etc.) print every run.
$plain = $second -replace ([string][char]27 + '\[[0-9;]*m'), ''
$reran = $plain -split "`n" | Where-Object { $_ -match '==> (winget packages|XDG_CONFIG_HOME|PowerShell profile|Neovim plugins|Windows Terminal)' }
Check 'second install is a no-op' (-not $reran) ($reran -join "`n")

Write-Host "---- $fail failed"
exit [int]($fail -gt 0)
