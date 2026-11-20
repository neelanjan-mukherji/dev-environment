package app_test

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"
)

// The doctor's desktop checks: VS Code, its extensions and settings, and the OS
// tweaks, only when the desktop component is on.

func TestDoctorChecksVSCodeOnlyWithDesktopOn(t *testing.T) {
	for _, goos := range []string{"linux", "darwin", "windows"} {
		t.Run(goos, func(t *testing.T) {
			off := &fakeRunner{rules: []rule{machineOn(goos, "dev", devBase, "", "mocha")}}
			if out, _ := runOn(t, goos, off, nil, "doctor"); strings.Contains(out, "VS Code") {
				t.Errorf("VS Code checked without desktop\n%s", out)
			}
			on := &fakeRunner{rules: []rule{machineOn(goos, "dev", devBase+",desktop", "", "mocha")}}
			out, code := runOn(t, goos, on, nil, "doctor")
			if code != 0 || !strings.Contains(out, "ok   VS Code: code on PATH\n") {
				t.Fatalf("exit %d, want a passing code check\n%s", code, out)
			}
		})
	}
}

func TestDoctorReportsASkippedVSCodeInstallAsSkipped(t *testing.T) {
	// What each OS's install step records when it can't install VS Code.
	for goos, skip := range map[string]string{
		"linux":   "vscode: Microsoft publishes VS Code for apt and dnf only, not pacman",
		"darwin":  "Homebrew (declined): brew install --cask visual-studio-code",
		"windows": "winget packages: winget is missing",
	} {
		t.Run(goos, func(t *testing.T) {
			b := newBox(t)
			write(t, filepath.Join(b.state, "skipped.log"), "2026-11-19 10:00:00\t"+skip+"\n")
			r := &fakeRunner{rules: []rule{machineOn(goos, "minimal", "desktop", "", "mocha"), {match: "code", fail: true}}}
			out, _ := runOn(t, goos, r, nil, "doctor")
			if !strings.Contains(out, "skip VS Code: code on PATH") {
				t.Fatalf("want code reported as skipped\n%s", out)
			}
		})
	}
}

// desktopData is the desktop component's chezmoi data as JSON, with IDs and keys
// from Microsoft's and Apple's documentation.
const desktopData = `{"extensions":{"shared":["catppuccin.catppuccin-vsc","EditorConfig.EditorConfig"],"windows":["ms-vscode-remote.remote-wsl"]},` +
	`"tweaks":{"darwin":[` +
	`{"id":"show-file-extensions","reads":[{"domain":"NSGlobalDomain","key":"AppleShowAllExtensions","value":"1"}]},` +
	`{"id":"screenshots-folder","reads":[{"domain":"com.apple.screencapture","key":"location","value":"$HOME/Pictures/Screenshots"}]}],` +
	`"windows":[` +
	`{"id":"show-file-extensions","reads":[{"key":"HKCU\\Software\\Microsoft\\Windows\\CurrentVersion\\Explorer\\Advanced","name":"HideFileExt","value":"0"}],"min_build":0},` +
	`{"id":"classic-context-menu","reads":[{"key":"HKCU\\Software\\Classes\\CLSID\\{86ca1aa0-34aa-4e8b-a509-50c905bae2a2}\\InprocServer32","name":"","value":""}],"min_build":22000}]}}`

// desktopMachine is a minimal machine with desktop on; wsl says whether it is WSL.
func desktopMachine(goos, flavor string, wsl bool) rule {
	r := machineOn(goos, "minimal", "desktop", "", flavor)
	r.out += fmt.Sprintf("%v\n%s\n", wsl, desktopData)
	return r
}

func TestDoctorChecksEachPinnedExtension(t *testing.T) {
	for goos, want := range map[string][]string{
		"linux":   {"catppuccin.catppuccin-vsc", "EditorConfig.EditorConfig"},
		"darwin":  {"catppuccin.catppuccin-vsc", "EditorConfig.EditorConfig"},
		"windows": {"catppuccin.catppuccin-vsc", "EditorConfig.EditorConfig", "ms-vscode-remote.remote-wsl"},
	} {
		t.Run(goos, func(t *testing.T) {
			r := &fakeRunner{rules: []rule{desktopMachine(goos, "mocha", false)}}
			out, _ := runOn(t, goos, r, nil, "doctor")
			for _, id := range want {
				if !strings.Contains(out, "ok   VS Code extension "+id+"\n") || r.called("--list-extensions", id) < 0 {
					t.Errorf("extension %s not checked with code --list-extensions\n%s", id, out)
				}
			}
			if goos != "windows" && strings.Contains(out, "remote-wsl") {
				t.Errorf("the WSL extension is checked off the Windows host\n%s", out)
			}
		})
	}
}

func TestDoctorReportsAMissingExtensionAndItsSkip(t *testing.T) {
	b := newBox(t)
	r := &fakeRunner{rules: []rule{desktopMachine("linux", "mocha", false), {match: "catppuccin.catppuccin-vsc", fail: true}}}
	if out, code := doctor(t, r); code != 1 || !strings.Contains(out, "FAIL VS Code extension catppuccin.catppuccin-vsc") {
		t.Fatalf("exit %d, want the missing extension reported\n%s", code, out)
	}
	write(t, filepath.Join(b.state, "skipped.log"), "2026-11-19 10:00:00\tvscode extensions: code is not on PATH\n")
	if out, code := doctor(t, r); code != 0 || !strings.Contains(out, "skip VS Code extension catppuccin.catppuccin-vsc") {
		t.Fatalf("exit %d, want the extension reported as skipped\n%s", code, out)
	}
}

func TestDoctorLeavesExtensionsToTheWindowsHostInsideWSL(t *testing.T) {
	// Inside WSL, code is the host's; the host's own setup installs the extensions.
	r := &fakeRunner{rules: []rule{desktopMachine("linux", "mocha", true)}}
	if out, _ := doctor(t, r); strings.Contains(out, "VS Code extension") || r.called("--list-extensions") >= 0 {
		t.Fatalf("checked extensions inside WSL\n%s", out)
	}
}

func TestDoctorLeavesSettingsToTheWindowsHostInsideWSL(t *testing.T) {
	// Inside WSL, settings.json is the host's; chezmoi doesn't write one here.
	r := &fakeRunner{rules: []rule{desktopMachine("linux", "mocha", true)}}
	if out, _ := doctor(t, r); strings.Contains(out, "settings.json") || r.called("settings.json") >= 0 {
		t.Fatalf("checked settings.json inside WSL\n%s", out)
	}
}

func TestDoctorChecksVSCodeSettingsHaveTheSharedThemeAndFont(t *testing.T) {
	// VS Code's settings.json path per OS (VS Code docs), and Catppuccin's theme name per flavor.
	for goos, path := range map[string]string{
		"linux":   ".config/Code/User/settings.json",
		"darwin":  "Library/Application Support/Code/User/settings.json",
		"windows": `$env:APPDATA\Code\User\settings.json`,
	} {
		t.Run(goos, func(t *testing.T) {
			r := &fakeRunner{rules: []rule{desktopMachine(goos, "frappe", false)}}
			out, _ := runOn(t, goos, r, nil, "doctor")
			if !strings.Contains(out, "ok   VS Code settings.json has the shared theme and font\n") ||
				r.called(path, "Catppuccin Frappé", "JetBrainsMono Nerd Font") < 0 {
				t.Fatalf("settings.json not checked for the flavor's theme and the font\n%s\ncalls:\n%s", out, r.log())
			}
		})
	}
}

func TestDoctorSaysWhySettingsWithCommentsAreNotManaged(t *testing.T) {
	// VS Code allows comments (JSONC); the settings template then leaves the file alone.
	r := &fakeRunner{rules: []rule{desktopMachine("linux", "mocha", false), {match: "Catppuccin Mocha", fail: true}}}
	out, code := doctor(t, r)
	if code != 1 || !strings.Contains(out, "FAIL VS Code settings.json") || !strings.Contains(out, "comments") {
		t.Fatalf("exit %d, want the failure to explain comments stop the shared keys\n%s", code, out)
	}
}

func TestDoctorReadsEachMacOSTweakBack(t *testing.T) {
	newBox(t)
	r := &fakeRunner{rules: []rule{desktopMachine("darwin", "mocha", false), {match: "AppleShowAllExtensions", fail: true}}}
	out, code := runOn(t, "darwin", r, nil, "doctor")
	if code != 1 || !strings.Contains(out, "FAIL macOS tweak show-file-extensions\n") || r.called("defaults read", "NSGlobalDomain", "AppleShowAllExtensions") < 0 {
		t.Fatalf("exit %d, want the unapplied tweak reported from defaults read\n%s\ncalls:\n%s", code, out, r.log())
	}
	if !strings.Contains(out, "ok   macOS tweak screenshots-folder\n") {
		t.Errorf("applied tweak not reported\n%s", out)
	}
	for _, goos := range []string{"linux", "windows"} {
		if out, _ := runOn(t, goos, &fakeRunner{rules: []rule{desktopMachine(goos, "mocha", false)}}, nil, "doctor"); strings.Contains(out, "macOS tweak") {
			t.Errorf("%s: macOS tweaks checked\n%s", goos, out)
		}
	}
}

func TestDoctorReadsEachWindowsTweakBackFromTheRegistry(t *testing.T) {
	newBox(t)
	r := &fakeRunner{rules: []rule{desktopMachine("windows", "mocha", false), {match: "HideFileExt", fail: true}}}
	out, code := runOn(t, "windows", r, nil, "doctor")
	if code != 1 || !strings.Contains(out, "FAIL Windows tweak show-file-extensions\n") {
		t.Fatalf("exit %d, want the unapplied tweak reported\n%s", code, out)
	}
	i := r.called("powershell", "reg.exe query", `'HKCU\Software\Microsoft\Windows\CurrentVersion\Explorer\Advanced' 'HideFileExt'`)
	if i < 0 || strings.Contains(r.calls[i], "CurrentBuildNumber") {
		t.Errorf("HideFileExt not read from the registry, or gated on the build; calls:\n%s", r.log())
	}
	// The classic context menu is Windows 11 only (build 22000).
	if r.called("{86ca1aa0-34aa-4e8b-a509-50c905bae2a2}", "CurrentBuildNumber", "22000") < 0 {
		t.Errorf("classic context menu not gated on the Windows 11 build; calls:\n%s", r.log())
	}
}

func TestDoctorPassesADeclinedTweak(t *testing.T) {
	for _, goos := range []string{"darwin", "windows"} {
		t.Run(goos, func(t *testing.T) {
			b := newBox(t)
			write(t, filepath.Join(b.state, "tweak-show-file-extensions.declined"), "")
			r := &fakeRunner{rules: []rule{desktopMachine(goos, "mocha", false), {match: "AppleShowAllExtensions", fail: true}, {match: "HideFileExt", fail: true}}}
			out, code := runOn(t, goos, r, nil, "doctor")
			if code != 0 || !strings.Contains(out, "tweak show-file-extensions (declined)\n") {
				t.Fatalf("exit %d, want the declined tweak to pass\n%s", code, out)
			}
		})
	}
}

func TestDoctorReportsTweaksNobodyWasAskedAboutAsSkipped(t *testing.T) {
	for goos, skip := range map[string]string{
		"darwin":  "macos tweaks: 2 not applied, nobody to ask (run dev setup in a terminal, or with --yes)",
		"windows": "windows tweaks: 2 not applied, nobody to ask (run dev setup in a terminal, or with --yes)",
	} {
		t.Run(goos, func(t *testing.T) {
			b := newBox(t)
			write(t, filepath.Join(b.state, "skipped.log"), "2026-11-19 10:00:00\t"+skip+"\n")
			r := &fakeRunner{rules: []rule{desktopMachine(goos, "mocha", false), {match: "AppleShowAllExtensions", fail: true}, {match: "HideFileExt", fail: true}}}
			out, code := runOn(t, goos, r, nil, "doctor")
			if code != 0 || !strings.Contains(out, "skip ") || !strings.Contains(out, "tweak show-file-extensions (the install step was skipped") {
				t.Fatalf("exit %d, want the tweak reported as skipped\n%s", code, out)
			}
		})
	}
}

func TestDoctorCountsOnlyAMissingWingetAsTheWindowsVSCodeSkip(t *testing.T) {
	// Another winget packages skip (not about winget itself) says nothing about VS Code.
	b := newBox(t)
	write(t, filepath.Join(b.state, "skipped.log"), "2026-11-19 10:00:00\twinget packages: Microsoft.WindowsTerminal not installed\n")
	r := &fakeRunner{rules: []rule{machineOn("windows", "minimal", "desktop", "", "mocha"), {match: "Get-Command code", fail: true}}}
	out, code := runOn(t, "windows", r, nil, "doctor")
	if code != 1 || !strings.Contains(out, "FAIL VS Code: code on PATH") {
		t.Fatalf("exit %d, want the missing code reported as a failure\n%s", code, out)
	}
}

func TestDoctorFailsAMissingCodeWhenOnlyTheExtensionsWereSkipped(t *testing.T) {
	// The extension step's own skip says code is missing; it is not the install skip.
	b := newBox(t)
	write(t, filepath.Join(b.state, "skipped.log"), "2026-11-19 10:00:00\tvscode extensions: code is not on PATH\n")
	r := &fakeRunner{rules: []rule{machine("minimal", "desktop", ""), {match: "command -v code", fail: true}}}
	out, code := doctor(t, r)
	if code != 1 || !strings.Contains(out, "FAIL VS Code: code on PATH") {
		t.Fatalf("exit %d, want the missing code reported as a failure\n%s", code, out)
	}
}
