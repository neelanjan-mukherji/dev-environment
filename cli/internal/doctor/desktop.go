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

// desktopOS is what the desktop checks need from one OS: which skips mean VS
// Code wasn't installed, how each check is written there, and its tweaks.
type desktopOS struct {
	installSkips []string // skipped.log prefixes of the step that installs VS Code
	code         func() Check
	extensions   func(Desktop) []string
	extension    func(id string) Check
	settings     func(name, theme string) Check
	tweakOS      string // in check names: "macOS tweak <id>"
	tweaks       func(Desktop) []Tweak
	applied      func(Tweak) Check
}

// unixDesktop is Linux; macOS differs only where it says so.
var unixDesktop = desktopOS{
	// The Linux step's own skips ("vscode: …", "vscode (…)", not "vscode extensions: …").
	installSkips: []string{"vscode:", "vscode ("},
	code:         func() Check { return zsh("", "command -v code") }, // the user's PATH (Homebrew's bin on macOS)
	extensions:   func(d Desktop) []string { return d.Extensions.Shared },
	// IDs are case-insensitive; the list prints the Marketplace's casing.
	extension: func(id string) Check { return zsh("", "code --list-extensions | grep -qixF "+shellQuote(id)) },
	settings:  jqSettings(`"$HOME/.config/Code/User/settings.json"`),
	tweaks:    func(Desktop) []Tweak { return nil },
}

func desktopFor(goos string) desktopOS {
	switch goos {
	case "darwin":
		d := unixDesktop
		d.installSkips = []string{"homebrew"} // the cask is in the Homebrew step
		d.settings = jqSettings(`"$HOME/Library/Application Support/Code/User/settings.json"`)
		d.tweakOS = "macOS"
		d.tweaks = func(d Desktop) []Tweak { return d.Tweaks.Darwin }
		d.applied = func(t Tweak) Check { return bash("", macOSApplied(t)) }
		return d
	case "windows":
		return desktopOS{
			installSkips: []string{"winget packages: winget is missing"}, // VS Code is in the winget step
			code:         func() Check { return ps("", "Get-Command code | Out-Null") },
			extensions: func(d Desktop) []string {
				return append(d.Extensions.Shared[:len(d.Extensions.Shared):len(d.Extensions.Shared)], d.Extensions.Windows...)
			},
			// -notcontains compares case-insensitively.
			extension: func(id string) Check {
				return ps("", "$l = @(code --list-extensions); if ($LASTEXITCODE -ne 0 -or $l -notcontains "+psQuote(id)+") { exit 1 }")
			},
			settings: func(name, theme string) Check {
				return ps(name, fmt.Sprintf(`$s = Get-Content -Raw -Encoding UTF8 "$env:APPDATA\Code\User\settings.json" | ConvertFrom-Json; `+
					`if ($s.'workbench.colorTheme' -cne %s -or $s.'editor.fontFamily' -cne %[2]s -or $s.'terminal.integrated.fontFamily' -cne %[2]s -or $s.'terminal.integrated.fontLigatures.enabled' -ne $false) { exit 1 }`,
					psQuote(theme), psQuote(vscodeFont)))
			},
			tweakOS: "Windows",
			tweaks:  func(d Desktop) []Tweak { return d.Tweaks.Windows },
			applied: func(t Tweak) Check { return ps("", windowsApplied(t)) },
		}
	}
	return unixDesktop
}

func desktopChecks(m Machine) []Check {
	d := desktopFor(m.OS)
	installSkipped := skippedStep(m.State, "", d.installSkips...)
	code := d.code()
	code.Name, code.Skipped = "VS Code: code on PATH", installSkipped
	cs := []Check{code}

	// Inside WSL, code is the Windows host's, and so are its extensions and settings.
	if !m.WSL {
		extSkipped := skippedStep(m.State, "", "vscode extensions")
		for _, id := range d.extensions(m.Desktop) {
			c := d.extension(id)
			c.Name, c.Skipped = "VS Code extension "+id, func() bool { return installSkipped() || extSkipped() }
			cs = append(cs, c)
		}
		// The template leaves a file it can't parse (VS Code allows comments) as it is, so this is where that shows.
		c := d.settings("VS Code settings.json has the shared theme and font", vscodeThemes[m.Flavor])
		c.Hint = "The shared keys aren't applied while settings.json has comments or trailing commas: remove them, then run chezmoi apply."
		cs = append(cs, c)
	}

	for _, t := range d.tweaks(m.Desktop) {
		cs = append(cs, tweakCheck(m, d.tweakOS, t, d.applied(t)))
	}
	return cs
}

// vscodeThemes are Catppuccin's VS Code theme names per flavor.
var vscodeThemes = map[string]string{"latte": "Catppuccin Latte", "frappe": "Catppuccin Frappé", "macchiato": "Catppuccin Macchiato", "mocha": "Catppuccin Mocha"}

const vscodeFont = "JetBrainsMono Nerd Font"

// jqSettings checks the keys the settings.json template owns at path (zsh: jq comes from mise).
func jqSettings(path string) func(name, theme string) Check {
	return func(name, theme string) Check {
		return zsh(name, fmt.Sprintf(`jq -e --arg t %s --arg f %s '.["workbench.colorTheme"] == $t and .["editor.fontFamily"] == $f and .["terminal.integrated.fontFamily"] == $f and .["terminal.integrated.fontLigatures.enabled"] == false' %s >/dev/null`,
			shellQuote(theme), shellQuote(vscodeFont), path))
	}
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

// readRegistry reads a registry value with reg.exe: the value as text (a DWORD in
// decimal), ” for an empty string, $null when the key or value is missing (an
// unset default value too). Name ” = the key's default. The Windows tweak step
// has the same Read-Registry; go:embed can't reach home/, so change both together.
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
