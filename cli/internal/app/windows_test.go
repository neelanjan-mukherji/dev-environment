package app_test

import (
	"path/filepath"
	"strings"
	"testing"
)

// The Windows host: winget installs the tools, chezmoi applies the shared
// configs, and WSL is offered for the Linux side.

func TestSetupOnWindowsInstallsGitAndChezmoiWithWinget(t *testing.T) {
	b := newBox(t)
	r := &fakeRunner{rules: []rule{{match: "where.exe git", fail: true}, {match: "where.exe chezmoi", fail: true}}}
	out, _ := runOn(t, "windows", r, nil, "setup", "--yes", "--source", b.repo)
	for _, id := range []string{"Git.Git", "twpayne.chezmoi"} {
		if r.called("winget install", "--id "+id) < 0 {
			t.Fatalf("%s not installed with winget; calls:\n%s\n%s", id, r.log(), out)
		}
	}
	if r.called("sh -c") >= 0 || r.called("mise") >= 0 || r.called("apt-get") >= 0 {
		t.Fatalf("ran a Unix step on Windows; calls:\n%s", r.log())
	}
}

func TestSetupOnWindowsWithoutWingetSaysWhatToInstall(t *testing.T) {
	b := newBox(t)
	r := &fakeRunner{rules: []rule{{match: "where.exe winget", fail: true}, {match: "where.exe git", fail: true}}}
	out, code := runOn(t, "windows", r, nil, "setup", "--yes", "--source", b.repo)
	if code == 0 || !strings.Contains(out, "App Installer") {
		t.Fatalf("exit %d, want a pointer to App Installer (winget)\n%s", code, out)
	}
}

func TestSetupOnWindowsOffersTheLinuxSideInsideTheDefaultWSLDistro(t *testing.T) {
	b := newBox(t)
	r := &fakeRunner{rules: []rule{
		{match: "wsl.exe --list --verbose", out: "  NAME      STATE    VERSION\n  Debian    Stopped  2\n* Ubuntu-24.04 Running 2\n"},
		{match: "rev-parse --abbrev-ref HEAD", out: "v2\n"},
		// The config template echoes the answers (as chezmoi does with DEVENV_*).
		{match: "execute-template --init", out: "[data]\nprofile = \"full\"\nname = \"Ada O'Neil\"\nemail = \"ada@example.com\"\n"},
	}}
	p := &fakePrompter{answers: map[string]bool{"Apply": true, "WSL": true}}
	out, _ := runOn(t, "windows", r, p, "setup", "--source", b.repo, "--skip-identity",
		"--profile", "full", "--name", "Ada O'Neil", "--email", "ada@example.com")
	i := r.called("wsl.exe -d Ubuntu-24.04", "install.sh")
	if i < 0 {
		t.Fatalf("Linux side not set up in the default distro; calls:\n%s\n%s", r.log(), out)
	}
	call := r.calls[i]
	// The same branch as the host's checkout, and the answers already given here.
	for _, want := range []string{"/v2/install.sh", "DEVENV_REF=v2", "--profile full", `'Ada O'\''Neil'`, "--email ada@example.com"} {
		if !strings.Contains(call, want) {
			t.Errorf("WSL setup lacks %q:\n%s", want, call)
		}
	}
}

func TestSetupOnWindowsOffersToInstallWSLWhenThereIsNone(t *testing.T) {
	b := newBox(t)
	r := &fakeRunner{rules: []rule{{match: "wsl.exe --list --verbose", fail: true}}}
	p := &fakePrompter{answers: map[string]bool{"Apply": true, "WSL": true}}
	out, _ := runOn(t, "windows", r, p, "setup", "--source", b.repo, "--skip-identity")
	if r.called("wsl.exe --install -d Ubuntu") < 0 {
		t.Fatalf("WSL not offered; calls:\n%s\n%s", r.log(), out)
	}
}

func TestSetupOnWindowsWithYesLeavesWSLAloneUnlessAskedFor(t *testing.T) {
	// Installing WSL needs a restart; scripted runs (CI) only do it with --wsl.
	b := newBox(t)
	r := &fakeRunner{rules: []rule{{match: "wsl.exe --list --verbose", fail: true}}}
	runOn(t, "windows", r, nil, "setup", "--yes", "--source", b.repo)
	if r.called("wsl.exe --install") >= 0 {
		t.Fatalf("installed WSL without --wsl; calls:\n%s", r.log())
	}
	if log := read(t, filepath.Join(b.state, "skipped.log")); !strings.Contains(log, "wsl") {
		t.Fatalf("the WSL skip is not recorded (doctor would fail it): %q", log)
	}
	r = &fakeRunner{rules: []rule{{match: "wsl.exe --list --verbose", fail: true}}}
	runOn(t, "windows", r, nil, "setup", "--yes", "--wsl", "--source", b.repo)
	if r.called("wsl.exe --install -d Ubuntu") < 0 {
		t.Fatalf("--wsl did not install WSL; calls:\n%s", r.log())
	}
}

func TestUpdateOnWindowsRestoresNeovimPluginsAndUpgradesWithWinget(t *testing.T) {
	b := newBox(t)
	b.configured(t)
	r := &fakeRunner{rules: []rule{sourceAt(b.repo)}}
	if out, code := runOn(t, "windows", r, nil, "update", "--system", "--yes"); code != 0 {
		t.Fatalf("exit %d\n%s\ncalls:\n%s", code, out, r.log())
	}
	if r.called("Lazy! restore") < 0 || r.called("winget upgrade --all") < 0 {
		t.Fatalf("want Lazy! restore and winget upgrade --all; calls:\n%s", r.log())
	}
	for _, unix := range []string{"mise upgrade", "zinit", "tpm", "apt-get"} {
		if r.called(unix) >= 0 {
			t.Fatalf("ran %q on Windows; calls:\n%s", unix, r.log())
		}
	}
}

func TestDoctorOnWindowsChecksTheHostSetupInPowerShell(t *testing.T) {
	r := &fakeRunner{rules: []rule{machineOn("windows", "minimal", "", "", "mocha")}}
	doctor(t, r)
	for _, want := range []string{"powershell", "STARSHIP_SHELL", "Windows Terminal", "XDG_CONFIG_HOME", "JetBrainsMonoNerdFont", "nvim-data"} {
		if r.called(want) < 0 {
			t.Errorf("no check mentions %q; calls:\n%s", want, r.log())
		}
	}
	if r.called("zsh") >= 0 {
		t.Fatalf("zsh checks on Windows; calls:\n%s", r.log())
	}
}

func TestAddingAToolOnWindowsPointsToWSL(t *testing.T) {
	// The host has no mise; tools belong to the Linux side.
	b := newBox(t)
	b.configured(t)
	r := &fakeRunner{rules: []rule{sourceAt(b.repo)}}
	out, code := runOn(t, "windows", r, nil, "add", "terraform", "--yes")
	if code == 0 || !strings.Contains(out, "WSL") || r.called("mise") >= 0 {
		t.Fatalf("exit %d, want a pointer to WSL and no mise\n%s\ncalls:\n%s", code, out, r.log())
	}
}

func TestDecliningTheLinuxSideIsRecordedAsASkip(t *testing.T) {
	b := newBox(t)
	r := &fakeRunner{rules: []rule{{match: "wsl.exe --list --verbose", out: "  NAME STATE VERSION\n* Ubuntu Running 2\n"}}}
	p := &fakePrompter{answers: map[string]bool{"Apply": true}}
	runOn(t, "windows", r, p, "setup", "--source", b.repo, "--skip-identity")
	if log := read(t, filepath.Join(b.state, "skipped.log")); !strings.Contains(log, "wsl") {
		t.Fatalf("declining the Linux side is not recorded: %q", log)
	}
}
