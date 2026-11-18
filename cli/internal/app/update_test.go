package app_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// sourceAt answers "where is the repo?" the way chezmoi does on a configured machine.
func sourceAt(dir string) rule {
	return rule{match: "workingTree", out: dir + "\n"}
}

func TestUpdatePullsAppliesAndUpgradesToolsAndPlugins(t *testing.T) {
	b := newBox(t)
	b.configured(t)
	r := &fakeRunner{rules: []rule{sourceAt(b.repo)}}
	out, code := run(t, r, nil, "update")
	if code != 0 {
		t.Fatalf("exit %d\n%s\ncalls:\n%s", code, out, r.log())
	}
	// docs/v2-design.md: chezmoi update + mise upgrade + Zinit/lazy.nvim/TPM plugins.
	order := []string{
		"git -C " + b.repo + " pull --ff-only",
		"chezmoi apply --force --keep-going --refresh-externals",
		"mise upgrade",
		"zinit update --all",
		"tpm/bin/update_plugins all",
		"Lazy! restore",
	}
	last := -1
	for _, step := range order {
		i := r.called(step)
		if i < 0 || i < last {
			t.Fatalf("want %q after the steps before it; calls:\n%s", step, r.log())
		}
		last = i
	}
	if r.called("apt-get") >= 0 {
		t.Errorf("OS packages upgraded without --system; calls:\n%s", r.log())
	}
}

func TestUpdateSystemAlsoUpgradesOSPackages(t *testing.T) {
	b := newBox(t)
	b.configured(t)
	r := &fakeRunner{rules: []rule{sourceAt(b.repo)}}
	if out, code := run(t, r, nil, "update", "--system", "--yes"); code != 0 {
		t.Fatalf("exit %d\n%s", code, out)
	}
	if r.called("apt-get", "upgrade") < 0 {
		t.Fatalf("--system did not upgrade OS packages; calls:\n%s", r.log())
	}
}

func TestUpdateCarriesOnPastAFailedStepAndReportsIt(t *testing.T) {
	b := newBox(t)
	b.configured(t)
	r := &fakeRunner{rules: []rule{sourceAt(b.repo), {match: "mise upgrade", fail: true}}}
	out, code := run(t, r, nil, "update")
	if code == 0 {
		t.Fatalf("exit 0 although mise upgrade failed\n%s", out)
	}
	if r.called("Lazy! restore") < 0 {
		t.Fatalf("stopped at the failed step; calls:\n%s", r.log())
	}
	if !strings.Contains(out, "failed") || !strings.Contains(out, "CLI tools") {
		t.Errorf("summary does not name the failed step\n%s", out)
	}
}

func TestUpdateCheckOnlyRecordsHowFarBehindTheRepoIs(t *testing.T) {
	b := newBox(t)
	b.configured(t)
	r := &fakeRunner{rules: []rule{sourceAt(b.repo), {match: "rev-list --count", out: "3\n"}}}
	out, code := run(t, r, nil, "update", "--check")
	if code != 0 {
		t.Fatalf("exit %d\n%s", code, out)
	}
	if r.called("git -C "+b.repo+" fetch") < 0 {
		t.Fatalf("no fetch; calls:\n%s", r.log())
	}
	if r.called("chezmoi apply") >= 0 || r.called("pull") >= 0 || r.called("mise upgrade") >= 0 {
		t.Fatalf("--check changed something; calls:\n%s", r.log())
	}
	if got := strings.TrimSpace(read(t, filepath.Join(b.state, "update-check"))); got != "3" {
		t.Fatalf("update-check = %q, want 3", got)
	}
}

func TestUpdateClearsTheBehindNotice(t *testing.T) {
	b := newBox(t)
	b.configured(t)
	check := filepath.Join(b.state, "update-check")
	write(t, check, "3\n")
	old := time.Now().Add(-48 * time.Hour)
	os.Chtimes(check, old, old)
	r := &fakeRunner{rules: []rule{sourceAt(b.repo)}}
	if out, code := run(t, r, nil, "update"); code != 0 {
		t.Fatalf("exit %d\n%s", code, out)
	}
	if got := strings.TrimSpace(read(t, check)); got != "0" {
		t.Fatalf("update-check = %q after updating, want 0", got)
	}
}

func TestUpdateOnAMachineThatWasNeverSetUp(t *testing.T) {
	newBox(t)
	r := &fakeRunner{rules: []rule{{match: "workingTree", fail: true, out: "chezmoi: no config"}}}
	out, code := run(t, r, nil, "update")
	if code == 0 || !strings.Contains(out, "dev setup") {
		t.Fatalf("exit %d, want a pointer to dev setup\n%s", code, out)
	}
}

func TestUpdateReviewsLocallyEditedDotfilesBeforeOverwriting(t *testing.T) {
	b := newBox(t)
	b.configured(t)
	write(t, filepath.Join(b.home, ".zshrc"), "edited by hand\n")
	r := &fakeRunner{rules: []rule{sourceAt(b.repo), {match: "chezmoi status", out: " M .zshrc\n"}}}
	p := &fakePrompter{answers: map[string]bool{"Apply": false}}
	out, _ := run(t, r, p, "update")
	if r.called("chezmoi apply") >= 0 {
		t.Fatalf("overwrote edited dotfiles without asking; calls:\n%s", r.log())
	}
	if backups, _ := filepath.Glob(filepath.Join(b.home, ".dotfiles-backup/*/.zshrc")); len(backups) != 1 {
		t.Fatalf("no backup: %v\n%s", backups, out)
	}
	if r.called("mise upgrade") < 0 {
		t.Errorf("declining the dotfiles stopped the other updates; calls:\n%s", r.log())
	}
}

func TestUpdateClearsTheNoticeOnceThePullWorkedEvenIfAStepFails(t *testing.T) {
	b := newBox(t)
	b.configured(t)
	check := filepath.Join(b.state, "update-check")
	write(t, check, "3\n")
	r := &fakeRunner{rules: []rule{sourceAt(b.repo), {match: "mise upgrade", fail: true}}}
	run(t, r, nil, "update")
	if got := strings.TrimSpace(read(t, check)); got != "0" {
		t.Fatalf("update-check = %q after a successful pull, want 0", got)
	}
}
