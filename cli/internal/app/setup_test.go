package app_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/neelanjan-mukherji/dev-environment/cli/internal/setup"
)

// box is a fake machine: an empty home directory and a checkout of the repo.
type box struct {
	home, repo, state string
}

func newBox(t *testing.T) box {
	t.Helper()
	b := box{home: t.TempDir(), repo: t.TempDir()}
	b.state = filepath.Join(b.home, ".local/state/dev")
	t.Setenv("HOME", b.home)
	t.Setenv("XDG_STATE_HOME", "")
	t.Setenv("XDG_CONFIG_HOME", "")
	write(t, filepath.Join(b.repo, "home/.chezmoi.toml.tmpl"), "# config template\n")
	write(t, filepath.Join(b.repo, "home/.chezmoiscripts/run_before_00-migrate-v1.sh.tmpl"), "# migrate\n")
	return b
}

// configured makes the box look like a machine where setup already ran.
func (b box) configured(t *testing.T) {
	t.Helper()
	write(t, filepath.Join(b.home, ".config/chezmoi/chezmoi.toml"), "[data]\nprofile = \"dev\"\n")
}

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func read(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// rendered is what chezmoi prints for the config template with these answers.
func rendered(profile, name, email string) rule {
	return rule{match: "execute-template --init", out: `sourceDir = "/src"

[data]
profile = "` + profile + `"
name = "` + name + `"
email = "` + email + `"
`}
}

func appendTo(t *testing.T, path, line string) func() {
	return func() {
		f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
		if err != nil {
			t.Error(err)
			return
		}
		defer f.Close()
		f.WriteString(line + "\n")
	}
}

func TestSetupYesConfiguresThenAppliesWithoutAsking(t *testing.T) {
	b := newBox(t)
	r := &fakeRunner{rules: []rule{rendered("minimal", "CI User", "ci@example.com")}}
	out, code := run(t, r, nil, "setup", "--yes", "--source", b.repo, "--profile", "minimal", "--name", "CI User", "--email", "ci@example.com")
	if code != 0 {
		t.Fatalf("exit %d\n%s\ncalls:\n%s", code, out, r.log())
	}
	initAt := r.called("DEVENV_PROFILE=minimal", "DEVENV_NAME=CI User", "DEVENV_EMAIL=ci@example.com", "chezmoi init --source "+b.repo, "--promptDefaults", "--no-tty")
	applyAt := r.called("DEVENV_YES=1", "chezmoi apply", "--keep-going")
	if initAt < 0 || applyAt < 0 || applyAt < initAt {
		t.Fatalf("want init with the answers, then apply --keep-going; calls:\n%s", r.log())
	}
}

func TestSetupWithoutYesAndNobodyToAskStops(t *testing.T) {
	b := newBox(t)
	r := &fakeRunner{rules: []rule{rendered("dev", "", "")}}
	out, code := run(t, r, nil, "setup", "--source", b.repo)
	if code == 0 || !strings.Contains(out, "--yes") {
		t.Fatalf("exit %d, want failure pointing at --yes\n%s", code, out)
	}
	if r.called("chezmoi apply") >= 0 {
		t.Fatalf("applied with nobody to confirm; calls:\n%s", r.log())
	}
}

func TestSetupOnAFreshMachineUsesTheEditedSettings(t *testing.T) {
	b := newBox(t)
	r := &fakeRunner{rules: []rule{rendered("dev", "Me", "")}}
	p := &fakePrompter{
		answers: map[string]bool{"Apply": true},
		edit: func(s setup.Settings) setup.Settings {
			if s.Profile != "dev" || s.Name != "Me" {
				t.Errorf("form not prefilled with chezmoi's defaults: %+v", s)
			}
			s.Profile, s.Email = "full", "1+me@users.noreply.github.com"
			return s
		},
	}
	out, code := run(t, r, p, "setup", "--source", b.repo)
	if code != 0 {
		t.Fatalf("exit %d\n%s", code, out)
	}
	if !p.edited {
		t.Fatal("a fresh machine was not shown the settings form")
	}
	if r.called("DEVENV_PROFILE=full", "DEVENV_NAME=Me", "DEVENV_EMAIL=1+me@users.noreply.github.com", "chezmoi init") < 0 {
		t.Fatalf("init did not get the edited settings; calls:\n%s", r.log())
	}
	if r.called("DEVENV_YES", "chezmoi apply") >= 0 {
		t.Fatalf("interactive apply must not answer the install steps' questions; calls:\n%s", r.log())
	}
}

func TestSetupOnAConfiguredMachineOnlyAsksToConfirm(t *testing.T) {
	b := newBox(t)
	b.configured(t)
	r := &fakeRunner{rules: []rule{rendered("dev", "Me", "")}}
	p := &fakePrompter{answers: map[string]bool{"Use these settings": true, "Apply": true}}
	if out, code := run(t, r, p, "setup", "--source", b.repo); code != 0 {
		t.Fatalf("exit %d\n%s", code, out)
	}
	if p.edited {
		t.Error("settings form shown although the settings were accepted")
	}
	if r.called("chezmoi apply") < 0 {
		t.Fatalf("not applied; calls:\n%s", r.log())
	}
}

func TestSetupBacksUpChangedFilesAndStopsWhenDeclined(t *testing.T) {
	b := newBox(t)
	b.configured(t)
	write(t, filepath.Join(b.home, ".zshrc"), "my old zshrc\n")
	r := &fakeRunner{rules: []rule{rendered("dev", "", ""), {match: "chezmoi status", out: " M .zshrc\n"}}}
	p := &fakePrompter{answers: map[string]bool{"Use these settings": true, "Apply": false}}
	out, code := run(t, r, p, "setup", "--source", b.repo)
	if code != 0 {
		t.Fatalf("exit %d\n%s", code, out)
	}
	if r.called("chezmoi apply") >= 0 {
		t.Fatalf("applied although declined; calls:\n%s", r.log())
	}
	backups, _ := filepath.Glob(filepath.Join(b.home, ".dotfiles-backup/*/.zshrc"))
	if len(backups) != 1 || read(t, backups[0]) != "my old zshrc\n" {
		t.Fatalf("want one backup of .zshrc, got %v\n%s", backups, out)
	}
	if !strings.Contains(out, ".zshrc") {
		t.Errorf("changed file not listed\n%s", out)
	}
}

func TestSetupSummarisesThisRunsFailuresAndSkips(t *testing.T) {
	b := newBox(t)
	failed := filepath.Join(b.state, "failed.log")
	skipped := filepath.Join(b.state, "skipped.log")
	write(t, failed, "2026-01-01 00:00:00\tan old failure (exit 1)\n")
	write(t, skipped, "2026-01-01 00:00:00\tan old skip\n")
	r := &fakeRunner{rules: []rule{
		rendered("dev", "", ""),
		{match: "chezmoi apply", fail: true, do: func() {
			appendTo(t, failed, "2026-11-18 10:00:00\tDocker Engine (exit 100)")()
			appendTo(t, skipped, "2026-11-18 10:00:01\tLogin shell: no sudo")()
		}},
	}}
	out, code := run(t, r, nil, "setup", "--yes", "--source", b.repo)
	if code == 0 {
		t.Fatalf("exit 0 although a step failed\n%s", out)
	}
	for _, want := range []string{"Docker Engine (exit 100)", "Login shell: no sudo", "--resume"} {
		if !strings.Contains(out, want) {
			t.Errorf("summary lacks %q\n%s", want, out)
		}
	}
	for _, old := range []string{"an old failure", "an old skip"} {
		if strings.Contains(out, old) {
			t.Errorf("summary repeats an earlier run's %q\n%s", old, out)
		}
	}
	// failed.log holds the latest run only, so `dev doctor` reflects it.
	if got := read(t, failed); strings.Contains(got, "old failure") || !strings.Contains(got, "Docker Engine") {
		t.Errorf("failed.log should hold only this run's failures, got:\n%s", got)
	}
}

func TestSetupResumeRetriesWithoutReconfiguring(t *testing.T) {
	b := newBox(t)
	b.configured(t)
	r := &fakeRunner{}
	if out, code := run(t, r, nil, "setup", "--resume", "--yes", "--source", b.repo); code != 0 {
		t.Fatalf("exit %d\n%s", code, out)
	}
	if r.called("chezmoi init") >= 0 || r.called("chezmoi apply", "--keep-going") < 0 {
		t.Fatalf("want apply only; calls:\n%s", r.log())
	}
}

func TestSetupReadsMachineYamlAndFlagsWin(t *testing.T) {
	b := newBox(t)
	cfg := filepath.Join(b.home, "machine.yaml")
	write(t, cfg, "profile: full\nname: From File\nadd: [cloud]\nyes: true\n")
	r := &fakeRunner{rules: []rule{rendered("minimal", "From File", "")}}
	if out, code := run(t, r, nil, "setup", "--config", cfg, "--profile", "minimal", "--source", b.repo); code != 0 {
		t.Fatalf("exit %d\n%s", code, out)
	}
	if r.called("DEVENV_PROFILE=minimal", "DEVENV_NAME=From File", "DEVENV_ADD=cloud", "chezmoi init") < 0 {
		t.Fatalf("want file values with the --profile flag on top; calls:\n%s", r.log())
	}
}

func TestSetupMigratesV1LinksBeforeTheReview(t *testing.T) {
	b := newBox(t)
	write(t, filepath.Join(b.home, "dotfiles/zsh/.zshrc"), "# v1\n")
	if err := os.Symlink(filepath.Join(b.home, "dotfiles/zsh/.zshrc"), filepath.Join(b.home, ".zshrc")); err != nil {
		t.Fatal(err)
	}
	r := &fakeRunner{rules: []rule{rendered("full", "", ""), {match: "bash " + filepath.Join(b.state, "migrate-v1.sh"), fail: true}}}
	out, code := run(t, r, nil, "setup", "--yes", "--source", b.repo)
	if code == 0 {
		t.Fatalf("exit 0 although the migration failed\n%s", out)
	}
	if r.called("chezmoi init", "DEVENV_DEFAULT_PROFILE=full") < 0 {
		t.Errorf("a v1 machine should default to the full profile; calls:\n%s", r.log())
	}
	if r.called("bash "+filepath.Join(b.state, "migrate-v1.sh")) < 0 || r.called("chezmoi status") >= 0 || r.called("chezmoi apply") >= 0 {
		t.Fatalf("want the migration, and nothing reviewed or applied after it failed; calls:\n%s", r.log())
	}
}

func TestSetupDryRunLeavesHomeAlone(t *testing.T) {
	b := newBox(t)
	r := &fakeRunner{rules: []rule{rendered("dev", "", "")}}
	out, code := run(t, r, nil, "setup", "--dry-run", "--yes", "--source", b.repo)
	if code != 0 {
		t.Fatalf("exit %d\n%s", code, out)
	}
	if r.called("chezmoi apply") >= 0 {
		t.Fatalf("dry run applied; calls:\n%s", r.log())
	}
	if i := r.called("chezmoi init", "--config "); i < 0 || strings.Contains(r.calls[i], "--config "+b.home) {
		t.Fatalf("dry run must init with a scratch config outside $HOME; calls:\n%s", r.log())
	}
	entries, _ := os.ReadDir(b.home)
	if len(entries) != 0 {
		t.Fatalf("dry run created %v in $HOME", entries)
	}
}

func TestSetupOffersIdentityStepsToAPerson(t *testing.T) {
	b := newBox(t)
	b.configured(t)
	key := filepath.Join(b.home, ".ssh/id_ed25519")
	r := &fakeRunner{rules: []rule{
		rendered("dev", "Me", "1+me@users.noreply.github.com"),
		{match: "gh auth status", fail: true},
		{match: "ssh-keygen", do: func() { write(t, key+".pub", "ssh-ed25519 AAAAC3Nza me\n") }},
	}}
	p := &fakePrompter{answers: map[string]bool{"Use these settings": true, "Apply": true, "SSH key": true, "Log in to GitHub": true}}
	if out, code := run(t, r, p, "setup", "--source", b.repo); code != 0 {
		t.Fatalf("exit %d\n%s", code, out)
	}
	keygen := r.called("ssh-keygen -t ed25519", "1+me@users.noreply.github.com")
	login := r.called("gh auth login")
	upload := r.called("gh ssh-key add", key+".pub")
	if keygen < 0 || login < 0 || upload < 0 || upload < login {
		t.Fatalf("want ssh-keygen, gh auth login, then gh ssh-key add; calls:\n%s", r.log())
	}
}

func TestSetupYesSkipsIdentityStepsThatNeedAPerson(t *testing.T) {
	b := newBox(t)
	r := &fakeRunner{rules: []rule{rendered("dev", "", ""), {match: "gh auth status", fail: true}}}
	out, code := run(t, r, nil, "setup", "--yes", "--source", b.repo)
	if code != 0 {
		t.Fatalf("exit %d\n%s", code, out)
	}
	if r.called("ssh-keygen") >= 0 || r.called("gh auth login") >= 0 {
		t.Fatalf("--yes ran an identity step; calls:\n%s", r.log())
	}
	if !strings.Contains(out, "gh auth login") {
		t.Errorf("no hint about logging in later\n%s", out)
	}
}

func TestMigrateV1WithoutV1LinksDoesNothing(t *testing.T) {
	b := newBox(t)
	r := &fakeRunner{}
	out, code := run(t, r, nil, "migrate-v1", "--yes", "--source", b.repo)
	if code != 0 || !strings.Contains(out, "Nothing to migrate") {
		t.Fatalf("exit %d, want a no-op\n%s", code, out)
	}
	if len(r.calls) != 0 {
		t.Fatalf("ran commands:\n%s", r.log())
	}
}

func TestMigrateV1RunsSetupWithTheMigration(t *testing.T) {
	b := newBox(t)
	write(t, filepath.Join(b.home, "dotfiles/nvim/.config/nvim/init.lua"), "-- v1\n")
	os.MkdirAll(filepath.Join(b.home, ".config"), 0o755)
	if err := os.Symlink("../dotfiles/nvim/.config/nvim", filepath.Join(b.home, ".config/nvim")); err != nil {
		t.Fatal(err)
	}
	r := &fakeRunner{rules: []rule{rendered("full", "", "")}}
	if out, code := run(t, r, nil, "migrate-v1", "--yes", "--source", b.repo); code != 0 {
		t.Fatalf("exit %d\n%s", code, out)
	}
	migrate := r.called("bash " + filepath.Join(b.state, "migrate-v1.sh"))
	if migrate < 0 || r.called("chezmoi apply") < migrate {
		t.Fatalf("want the migration, then apply; calls:\n%s", r.log())
	}
}
