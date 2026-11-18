package app_test

import (
	"strings"
	"testing"
)

// onlyPackageManager makes this fake Linux have only the given package manager.
func onlyPackageManager(pm string) []rule {
	var rules []rule
	for _, other := range []string{"apt-get", "dnf", "pacman", "apk"} {
		if other != pm {
			rules = append(rules, rule{match: "command -v " + other, fail: true})
		}
	}
	return rules
}

// freshMachine: git is missing (as on a fresh Fedora, Arch or Alpine), so the
// prerequisites step has to install them.
func freshMachine(pm string) []rule {
	return append([]rule{{match: "command -v git", fail: true}}, onlyPackageManager(pm)...)
}

func TestSetupInstallsPrerequisitesWithThisDistrosPackageManager(t *testing.T) {
	for pm, want := range map[string][]string{
		"apt-get": {"apt-get install", "git", "curl"},
		"dnf":     {"dnf install -y", "git", "curl"},
		"pacman":  {"pacman -Syu --needed --noconfirm", "git", "curl"},
		"apk":     {"apk add", "git", "curl", "bash"},
	} {
		t.Run(pm, func(t *testing.T) {
			b := newBox(t)
			r := &fakeRunner{rules: freshMachine(pm)}
			out, _ := run(t, r, nil, "setup", "--yes", "--source", b.repo)
			if r.called(want...) < 0 {
				t.Fatalf("want %q; calls:\n%s\n%s", strings.Join(want, " "), r.log(), out)
			}
		})
	}
}

func TestSetupChecksForBashToo(t *testing.T) {
	// Alpine ships without bash; every install script needs it.
	b := newBox(t)
	r := &fakeRunner{rules: append([]rule{{match: "command -v bash", fail: true}}, onlyPackageManager("apk")...)}
	run(t, r, nil, "setup", "--yes", "--source", b.repo)
	if r.called("apk add", "bash") < 0 {
		t.Fatalf("bash not installed; calls:\n%s", r.log())
	}
}

func TestUpdateSystemUsesThisDistrosPackageManager(t *testing.T) {
	for pm, want := range map[string]string{
		"dnf":    "dnf upgrade -y",
		"pacman": "pacman -Syu --noconfirm",
		"apk":    "apk upgrade",
	} {
		t.Run(pm, func(t *testing.T) {
			b := newBox(t)
			b.configured(t)
			r := &fakeRunner{rules: append([]rule{sourceAt(b.repo)}, onlyPackageManager(pm)...)}
			if out, code := run(t, r, nil, "update", "--system", "--yes"); code != 0 {
				t.Fatalf("exit %d\n%s\ncalls:\n%s", code, out, r.log())
			}
			if r.called(want) < 0 || r.called("apt-get upgrade") >= 0 {
				t.Fatalf("want %q and no apt; calls:\n%s", want, r.log())
			}
		})
	}
}

func TestNoSudoReachesTheMachineConfigAndSudoIsNeverUsed(t *testing.T) {
	b := newBox(t)
	r := &fakeRunner{}
	if out, code := run(t, r, nil, "setup", "--yes", "--no-sudo", "--source", b.repo); code != 0 {
		t.Fatalf("exit %d\n%s\ncalls:\n%s", code, out, r.log())
	}
	if r.called("DEVENV_NO_SUDO=1", "chezmoi init") < 0 {
		t.Fatalf("--no-sudo not passed to the machine config; calls:\n%s", r.log())
	}
	if r.called("sudo ") >= 0 {
		t.Fatalf("used sudo with --no-sudo; calls:\n%s", r.log())
	}
}

func TestNoSudoWithMissingPrerequisitesSaysWhatToAskFor(t *testing.T) {
	b := newBox(t)
	r := &fakeRunner{rules: freshMachine("dnf")}
	out, code := run(t, r, nil, "setup", "--yes", "--no-sudo", "--source", b.repo)
	if code == 0 || !strings.Contains(out, "git") || !strings.Contains(out, "sudo") {
		t.Fatalf("exit %d, want a no-sudo explanation naming git\n%s", code, out)
	}
	if r.called("dnf install") >= 0 {
		t.Fatalf("installed without sudo; calls:\n%s", r.log())
	}
}

func TestUpdateSystemOnANoSudoMachineDoesNotUseSudo(t *testing.T) {
	b := newBox(t)
	b.configured(t)
	r := &fakeRunner{rules: []rule{sourceAt(b.repo), {match: "nosudo", out: "true\n"}}}
	out, code := run(t, r, nil, "update", "--system", "--yes")
	if r.called("sudo ") >= 0 {
		t.Fatalf("used sudo on a machine set up with --no-sudo; calls:\n%s", r.log())
	}
	if code == 0 || !strings.Contains(out, "sudo") {
		t.Fatalf("exit %d, want the OS step reported as not possible without sudo\n%s", code, out)
	}
}
