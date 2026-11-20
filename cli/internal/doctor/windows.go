package doctor

import (
	"fmt"
	"strings"
)

// The Windows host: winget tools, the shared configs through XDG_CONFIG_HOME,
// the PowerShell profile, the Windows Terminal theme and font, and WSL.

// windowsTools are what packages.yaml's winget list puts on PATH.
var windowsTools = []string{"git", "gh", "nvim", "starship", "zoxide", "lazygit", "rg", "fd", "fzf", "zig", "chezmoi", "pwsh"}

// freshPath makes a check see what installers added to PATH since this
// console started (a process keeps the PATH it started with).
const freshPath = `$env:Path = [Environment]::GetEnvironmentVariable('Path','Machine') + ';' + [Environment]::GetEnvironmentVariable('Path','User'); `

// ps is a check in Windows PowerShell; a script fails with `exit 1` or an error.
func ps(name, script string) Check {
	return Check{Name: name, Shell: powerShell, Script: freshPath + "$ErrorActionPreference = 'Stop'; " + script}
}

const wtFragment = `$env:LOCALAPPDATA\Microsoft\Windows Terminal\Fragments\dev-environment\theme.json`

func windowsChecks(m Machine) []Check {
	var cs []Check
	for _, t := range windowsTools {
		cs = append(cs, ps(t+" on PATH", "Get-Command "+t+" | Out-Null"))
	}
	cs = append(cs,
		ps("neovim is pinned to 0.11", `if (-not ((nvim --version | Select-Object -First 1) -match 'v0\.11')) { exit 1 }`),
		ps(`XDG_CONFIG_HOME is ~\.config (shared nvim, lazygit, starship configs)`,
			`if ([Environment]::GetEnvironmentVariable('XDG_CONFIG_HOME','User') -ne "$HOME\.config") { exit 1 }`),
		// Both editions load the shared profile, which starts starship.
		ps("Windows PowerShell profile starts starship", `powershell -NoLogo -Command 'if (-not $env:STARSHIP_SHELL) { exit 1 }'; exit $LASTEXITCODE`),
		ps("PowerShell 7 profile starts starship", `pwsh -NoLogo -Command 'if (-not $env:STARSHIP_SHELL) { exit 1 }'; exit $LASTEXITCODE`),
		ps("Windows Terminal theme fragment", fmt.Sprintf(`Get-Content -Raw "%s" | ConvertFrom-Json | Out-Null`, wtFragment)),
		ps("JetBrainsMono Nerd Font installed",
			`if (-not (Get-ChildItem "$env:WINDIR\Fonts", "$env:LOCALAPPDATA\Microsoft\Windows\Fonts" -Filter 'JetBrainsMonoNerdFont*' -ErrorAction SilentlyContinue)) { exit 1 }`),
		ps("neovim plugins restored", `if (-not (Test-Path "$env:LOCALAPPDATA\nvim-data\lazy\telescope.nvim")) { exit 1 }`),
	)
	if m.Email != "" {
		cs = append(cs, ps("git identity from .gitconfig.local", `if ((git config user.email) -ne `+psQuote(m.Email)+`) { exit 1 }`))
	}
	wsl := ps("WSL has a Linux distro", `$env:WSL_UTF8 = '1'; $d = wsl.exe --list --quiet; if ($LASTEXITCODE -ne 0 -or -not ($d | Where-Object { $_ -and $_ -notlike 'docker-desktop*' })) { exit 1 }`)
	wsl.Skipped = skippedStep(m.State, "wsl.skipped", "wsl")
	if m.on("desktop") {
		cs = append(cs, desktopChecks(m)...)
	}
	cs = append(cs, wsl,
		ps("dotfiles in sync", `$out = chezmoi status --exclude externals,scripts; if ($LASTEXITCODE -ne 0) { exit 1 }; if ($out) { $out; chezmoi diff --exclude externals,scripts --no-pager | Select-Object -First 40; exit 1 }`),
		ps("no step failed", `$f = "$HOME\.local\state\dev\failed.log"; if ((Test-Path $f) -and (Get-Item $f).Length) { Get-Content $f; exit 1 }`),
	)
	return cs
}

// psQuote single-quotes s for PowerShell, where a quote inside is written twice.
func psQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", "''") + "'" }
