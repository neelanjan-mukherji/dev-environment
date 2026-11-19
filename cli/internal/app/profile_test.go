package app_test

import (
	"path/filepath"
	"strings"
	"testing"
)

// current answers "what is this machine's profile?" the way chezmoi does: the
// profile, its own components, the add and remove overrides, the effective
// components and the repo.
func current(profile, base, add, remove, effective, repo string) rule {
	return rule{match: "execute-template {{ .profile }}", out: strings.Join([]string{profile, base, add, remove, effective, repo}, "\n") + "\n"}
}

const devBase = "docker,devtools,node,python,go,rust"

func TestProfileShowsTheCurrentSelection(t *testing.T) {
	b := newBox(t)
	b.configured(t)
	r := &fakeRunner{rules: []rule{current("dev", devBase, "cloud", "go", "docker,devtools,node,python,rust,cloud", b.repo)}}
	out, code := run(t, r, nil, "profile")
	if code != 0 {
		t.Fatalf("exit %d\n%s", code, out)
	}
	for _, want := range []string{"dev", "+ cloud", "- go", "docker,devtools,node,python,rust,cloud"} {
		if !strings.Contains(out, want) {
			t.Errorf("output lacks %q\n%s", want, out)
		}
	}
	if r.called("chezmoi init") >= 0 {
		t.Errorf("showing the profile changed it; calls:\n%s", r.log())
	}
}

func TestProfileSwitchKeepsTheOverridesAndApplies(t *testing.T) {
	b := newBox(t)
	b.configured(t)
	r := &fakeRunner{rules: []rule{current("dev", devBase, "cloud", "", devBase+",cloud", b.repo), rendered("full", "Me", "")}}
	out, code := run(t, r, nil, "profile", "full", "--yes")
	if code != 0 {
		t.Fatalf("exit %d\n%s", code, out)
	}
	initAt := r.called("DEVENV_PROFILE=full", "DEVENV_ADD=cloud", "chezmoi init --source "+b.repo)
	if initAt < 0 || r.called("chezmoi apply") < initAt {
		t.Fatalf("want init with the new profile and old overrides, then apply; calls:\n%s", r.log())
	}
}

func TestProfileRejectsAnUnknownProfile(t *testing.T) {
	b := newBox(t)
	b.configured(t)
	r := &fakeRunner{rules: []rule{current("dev", devBase, "", "", devBase, b.repo)}}
	out, code := run(t, r, nil, "profile", "huge", "--yes")
	if code == 0 || !strings.Contains(out, "minimal") {
		t.Fatalf("exit %d, want an error listing the profiles\n%s", code, out)
	}
}

func TestAddAComponentMergesWithExistingOverrides(t *testing.T) {
	b := newBox(t)
	b.configured(t)
	// java is not in the dev profile: it becomes an addition; go was removed and comes back.
	r := &fakeRunner{rules: []rule{current("dev", devBase, "cloud", "go", "", b.repo), rendered("dev", "", "")}}
	if out, code := run(t, r, nil, "add", "java", "go", "--yes"); code != 0 {
		t.Fatalf("exit %d\n%s", code, out)
	}
	if r.called("DEVENV_PROFILE=dev", "DEVENV_ADD=cloud,java", "DEVENV_REMOVE= ", "chezmoi init") < 0 {
		t.Fatalf("want add=cloud,java and remove emptied; calls:\n%s", r.log())
	}
}

func TestRemoveAComponentOfTheProfile(t *testing.T) {
	b := newBox(t)
	b.configured(t)
	r := &fakeRunner{rules: []rule{current("dev", devBase, "cloud", "", "", b.repo), rendered("dev", "", "")}}
	if out, code := run(t, r, nil, "remove", "cloud", "docker", "--yes"); code != 0 {
		t.Fatalf("exit %d\n%s", code, out)
	}
	if r.called("DEVENV_ADD= ", "DEVENV_REMOVE=docker", "chezmoi init") < 0 {
		t.Fatalf("want add emptied and docker removed; calls:\n%s", r.log())
	}
}

func TestAddAToolThatIsNotAComponentGoesToThisMachineOnly(t *testing.T) {
	b := newBox(t)
	b.configured(t)
	r := &fakeRunner{rules: []rule{current("dev", devBase, "", "", devBase, b.repo)}}
	if out, code := run(t, r, nil, "add", "terraform@1.9", "--yes"); code != 0 {
		t.Fatalf("exit %d\n%s", code, out)
	}
	local := filepath.Join(b.home, ".config/mise/conf.d/local.toml")
	if r.called("mise use --path "+local+" terraform@1.9") < 0 {
		t.Fatalf("want mise use into the machine-local config; calls:\n%s", r.log())
	}
	if r.called("chezmoi init") >= 0 {
		t.Errorf("a tool changed the machine config; calls:\n%s", r.log())
	}
}

func TestRemoveAMachineLocalTool(t *testing.T) {
	b := newBox(t)
	b.configured(t)
	r := &fakeRunner{rules: []rule{current("dev", devBase, "", "", devBase, b.repo)}}
	if out, code := run(t, r, nil, "remove", "terraform", "--yes"); code != 0 {
		t.Fatalf("exit %d\n%s", code, out)
	}
	local := filepath.Join(b.home, ".config/mise/conf.d/local.toml")
	if r.called("mise unuse --path "+local+" terraform") < 0 {
		t.Fatalf("want mise unuse from the machine-local config; calls:\n%s", r.log())
	}
}

// uninstalls are the commands that would take something off the machine.
var uninstalls = []string{"uninstall", "unuse", "brew remove", "apt-get remove", "apt-get purge", "dnf remove", "pacman -R", "apk del", "Remove-Item"}

func TestAddDesktopTurnsTheComponentOn(t *testing.T) {
	for _, goos := range []string{"linux", "darwin", "windows"} {
		t.Run(goos, func(t *testing.T) {
			b := newBox(t)
			b.configured(t)
			r := &fakeRunner{rules: []rule{current("dev", devBase, "", "", devBase, b.repo), rendered("dev", "", "")}}
			if out, code := runOn(t, goos, r, nil, "add", "desktop", "--yes"); code != 0 {
				t.Fatalf("exit %d\n%s", code, out)
			}
			if r.called("DEVENV_ADD=desktop", "chezmoi init") < 0 {
				t.Fatalf("want desktop added to the machine config; calls:\n%s", r.log())
			}
			if r.called("mise use") >= 0 {
				t.Errorf("desktop was treated as a mise tool; calls:\n%s", r.log())
			}
		})
	}
}

func TestRemoveDesktopOnlyChangesTheSelection(t *testing.T) {
	for _, goos := range []string{"linux", "darwin", "windows"} {
		t.Run(goos, func(t *testing.T) {
			b := newBox(t)
			b.configured(t)
			r := &fakeRunner{rules: []rule{current("dev", devBase, "cloud,desktop", "", devBase+",cloud,desktop", b.repo), rendered("dev", "", "")}}
			out, code := runOn(t, goos, r, nil, "remove", "desktop", "--yes")
			if code != 0 {
				t.Fatalf("exit %d\n%s", code, out)
			}
			if r.called("DEVENV_ADD=cloud", "chezmoi init") < 0 || r.called("DEVENV_ADD=cloud,desktop") >= 0 {
				t.Fatalf("want desktop dropped from the additions; calls:\n%s", r.log())
			}
			for _, u := range uninstalls {
				if i := r.called(u); i >= 0 {
					t.Errorf("removing desktop ran an uninstall: %s", r.calls[i])
				}
			}
			if !strings.Contains(out, "nothing was uninstalled") {
				t.Errorf("summary does not say nothing was uninstalled\n%s", out)
			}
		})
	}
}
