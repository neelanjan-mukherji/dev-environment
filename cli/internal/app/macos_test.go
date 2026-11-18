package app_test

import (
	"strings"
	"testing"
)

func TestSetupOnAMacWithoutCommandLineToolsStartsTheirInstaller(t *testing.T) {
	b := newBox(t)
	r := &fakeRunner{rules: []rule{{match: "git --version", fail: true}}}
	out, code := runOn(t, "darwin", r, nil, "setup", "--yes", "--source", b.repo)
	if code == 0 || !strings.Contains(out, "Command Line Tools") {
		t.Fatalf("exit %d, want a stop that explains the Command Line Tools\n%s", code, out)
	}
	if r.called("xcode-select --install") < 0 || r.called("apt-get") >= 0 {
		t.Fatalf("want xcode-select --install and no apt; calls:\n%s", r.log())
	}
}

func TestSetupRunsOnAMac(t *testing.T) {
	b := newBox(t)
	r := &fakeRunner{rules: []rule{rendered("dev", "", "")}}
	if out, code := runOn(t, "darwin", r, nil, "setup", "--yes", "--source", b.repo); code != 0 {
		t.Fatalf("exit %d\n%s", code, out)
	}
	if r.called("chezmoi apply") < 0 {
		t.Fatalf("not applied; calls:\n%s", r.log())
	}
}

func TestUpdateSystemOnAMacUsesHomebrew(t *testing.T) {
	b := newBox(t)
	b.configured(t)
	r := &fakeRunner{rules: []rule{sourceAt(b.repo)}}
	if out, code := runOn(t, "darwin", r, nil, "update", "--system", "--yes"); code != 0 {
		t.Fatalf("exit %d\n%s", code, out)
	}
	if r.called("brew upgrade") < 0 || r.called("apt-get") >= 0 {
		t.Fatalf("want brew upgrade and no apt; calls:\n%s", r.log())
	}
}

func TestDoctorOnAMacChecksTheTerminalFontAndColima(t *testing.T) {
	r := &fakeRunner{rules: []rule{machineOn("darwin", "dev", "docker,node", "", "mocha")}}
	out, code := doctor(t, r)
	if code != 0 {
		t.Fatalf("exit %d\n%s", code, out)
	}
	for _, want := range []string{"ok   WezTerm installed", "ok   JetBrainsMono Nerd Font installed", "ok   zsh has colima"} {
		if !strings.Contains(out, want) {
			t.Errorf("no %q\n%s", want, out)
		}
	}
	out, _ = doctor(t, &fakeRunner{rules: []rule{machineOn("linux", "dev", "docker", "", "mocha")}})
	if strings.Contains(out, "WezTerm") || strings.Contains(out, "colima") {
		t.Errorf("macOS checks on linux\n%s", out)
	}
}
