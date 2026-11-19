package doctor

import (
	"fmt"
	"strings"
)

// The desktop component: VS Code, its pinned extensions and shared settings,
// and the reviewed OS tweaks.

// Desktop is the desktop component's chezmoi data (home/.chezmoidata/desktop.yaml).
type Desktop struct {
	Extensions struct {
		Shared  []string `json:"shared"`
		Windows []string `json:"windows"`
	} `json:"extensions"`
	Tweaks struct {
		Darwin  []Tweak `json:"darwin"`
		Windows []Tweak `json:"windows"`
	} `json:"tweaks"`
}

// Tweak is one reviewed OS preference; it is applied when every read matches.
type Tweak struct {
	ID       string `json:"id"`
	MinBuild int    `json:"min_build"` // Windows: lowest OS build it applies to
	Reads    []struct {
		Domain string `json:"domain"` // macOS: `defaults read <domain> <key>`
		Key    string `json:"key"`    // Windows: the registry key
		Name   string `json:"name"`   // Windows: the value's name ("" = the key's default)
		Value  string `json:"value"`
	} `json:"reads"`
}

// vscodeInstallSkipped reports whether the step that installs VS Code was
// skipped: by the Homebrew step on macOS, the winget step on Windows when winget
// itself is missing, and the Linux step itself ("vscode: …", "vscode (…)", not
// "vscode extensions: …").
func vscodeInstallSkipped(m Machine) func() bool {
	switch m.OS {
	case "darwin":
		return skippedStep(m.State, "", "homebrew")
	case "windows":
		return skippedStep(m.State, "", "winget packages: winget is missing")
	}
	return skippedStep(m.State, "", "vscode:", "vscode (")
}

func desktopChecks(m Machine) []Check {
	installSkipped := vscodeInstallSkipped(m)
	code := zsh("VS Code: code on PATH", "command -v code") // the user's PATH (Homebrew's bin on macOS)
	if m.OS == "windows" {
		code = ps(code.Name, "Get-Command code | Out-Null")
	}
	code.Skipped = installSkipped
	cs := []Check{code}

	// Inside WSL, code is the Windows host's, and so are its extensions and settings.
	if !m.WSL {
		extensions := m.Desktop.Extensions.Shared
		if m.OS == "windows" {
			extensions = append(extensions[:len(extensions):len(extensions)], m.Desktop.Extensions.Windows...)
		}
		extSkipped := skippedStep(m.State, "", "vscode extensions")
		for _, id := range extensions {
			// IDs are case-insensitive; the list prints the Marketplace's casing.
			c := zsh("VS Code extension "+id, "code --list-extensions | grep -qixF "+shellQuote(id))
			if m.OS == "windows" {
				// -notcontains compares case-insensitively.
				c = ps(c.Name, "$l = @(code --list-extensions); if ($LASTEXITCODE -ne 0 -or $l -notcontains "+psQuote(id)+") { exit 1 }")
			}
			c.Skipped = func() bool { return installSkipped() || extSkipped() }
			cs = append(cs, c)
		}
		cs = append(cs, settingsCheck(m))
	}

	switch m.OS {
	case "darwin":
		for _, t := range m.Desktop.Tweaks.Darwin {
			cs = append(cs, tweakCheck(m, "macOS", t, bash("", macOSApplied(t))))
		}
	case "windows":
		for _, t := range m.Desktop.Tweaks.Windows {
			cs = append(cs, tweakCheck(m, "Windows", t, ps("", windowsApplied(t))))
		}
	}
	return cs
}

// vscodeThemes are Catppuccin's VS Code theme names per flavor.
var vscodeThemes = map[string]string{"latte": "Catppuccin Latte", "frappe": "Catppuccin Frappé", "macchiato": "Catppuccin Macchiato", "mocha": "Catppuccin Mocha"}

const vscodeFont = "JetBrainsMono Nerd Font"

// settingsCheck checks the keys the settings.json template owns. The template
// leaves a file it can't parse (VS Code allows comments) as it is, so this is
// where that shows.
func settingsCheck(m Machine) Check {
	const name = "VS Code settings.json has the shared theme and font"
	theme := vscodeThemes[m.Flavor]
	var c Check
	if m.OS == "windows" {
		c = ps(name, fmt.Sprintf(`$s = Get-Content -Raw -Encoding UTF8 "$env:APPDATA\Code\User\settings.json" | ConvertFrom-Json; `+
			`if ($s.'workbench.colorTheme' -cne %s -or $s.'editor.fontFamily' -cne %[2]s -or $s.'terminal.integrated.fontFamily' -cne %[2]s -or $s.'terminal.integrated.fontLigatures.enabled' -ne $false) { exit 1 }`,
			psQuote(theme), psQuote(vscodeFont)))
	} else {
		path := `"$HOME/.config/Code/User/settings.json"`
		if m.OS == "darwin" {
			path = `"$HOME/Library/Application Support/Code/User/settings.json"`
		}
		// zsh: jq comes from mise.
		c = zsh(name, fmt.Sprintf(`jq -e --arg t %s --arg f %s '.["workbench.colorTheme"] == $t and .["editor.fontFamily"] == $f and .["terminal.integrated.fontFamily"] == $f and .["terminal.integrated.fontLigatures.enabled"] == false' %s >/dev/null`,
			shellQuote(theme), shellQuote(vscodeFont), path))
	}
	c.Hint = "The shared keys aren't applied while settings.json has comments or trailing commas: remove them, then run chezmoi apply."
	return c
}

// tweakCheck passes a tweak that was declined (its marker, as the tweak step
// leaves it), else reads it back with c; nobody to ask is reported as a skip.
func tweakCheck(m Machine, os string, t Tweak, c Check) Check {
	c.Name = os + " tweak " + t.ID
	if skippedStep(m.State, "tweak-"+t.ID+".declined")() {
		c.Name += " (declined)"
		c.Script = "exit 0"
		return c
	}
	prefix := strings.ToLower(os) + " tweak"
	c.Skipped = skippedStep(m.State, "", prefix+"s:", prefix+" "+t.ID+" ")
	return c
}

// macOSApplied is a bash script that succeeds when every read matches
// `defaults read` ($HOME in a value is the user's home).
func macOSApplied(t Tweak) string {
	var tests []string
	for _, r := range t.Reads {
		value := strings.ReplaceAll(shellQuote(r.Value), "$HOME", `'"$HOME"'`)
		tests = append(tests, fmt.Sprintf(`[ "$(defaults read %s %s 2>/dev/null)" = %s ]`, shellQuote(r.Domain), shellQuote(r.Key), value))
	}
	return strings.Join(tests, " && ")
}

// readRegistry reads a registry value with reg.exe, as the tweak step does: the
// value as text (a DWORD in decimal), '' for an empty string, $null when the key
// or value is missing (an unset default value too). Name '' = the key's default.
const readRegistry = `function Read-Registry([string]$Key, [string]$Name) {
    $ErrorActionPreference = 'Continue'
    if ($Name) { $which = @('/v', $Name) } else { $which = @('/ve') }
    $out = & reg.exe query $Key @which 2>$null
    if ($LASTEXITCODE -ne 0) { return $null }
    foreach ($line in $out) {
        if ("$line" -match '^ {4}.*? {4}(REG_[A-Z_]+)(?: {4}(.*))?$') {
            $data = "$($Matches[2])"
            if ($Matches[1] -eq 'REG_DWORD') { return [string][Convert]::ToInt64($data.Substring(2), 16) }
            return $data
        }
    }
    return $null
}
`

// windowsApplied is a PowerShell script that succeeds when every read matches
// the registry; a tweak for a later Windows build passes where it doesn't apply.
func windowsApplied(t Tweak) string {
	s := readRegistry
	if t.MinBuild > 0 {
		s += fmt.Sprintf("if ([int](Read-Registry 'HKLM\\SOFTWARE\\Microsoft\\Windows NT\\CurrentVersion' 'CurrentBuildNumber') -lt %d) { 'not before Windows build %[1]d'; exit 0 }\n", t.MinBuild)
	}
	for _, r := range t.Reads {
		s += fmt.Sprintf("if ((Read-Registry %s %s) -cne %s) { exit 1 }\n", psQuote(r.Key), psQuote(r.Name), psQuote(r.Value))
	}
	return s
}
