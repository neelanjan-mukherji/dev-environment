package setup

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/neelanjan-mukherji/dev-environment/cli/internal/sys"
)

// apply applies the dotfiles and runs the install scripts, carrying on past a
// failed step (chezmoi --keep-going). It returns this run's skipped and failed
// steps. failed.log keeps only the latest run, so `dev doctor`'s "no step
// failed" means the last run was clean.
func (r *run) apply(ctx context.Context, extra ...string) (skipped, failed []string, err error) {
	if err := os.MkdirAll(r.state, 0o755); err != nil {
		return nil, nil, err
	}
	failedLog := filepath.Join(r.state, "failed.log")
	if exists(failedLog) {
		if err := os.Rename(failedLog, filepath.Join(r.state, "failed.prev.log")); err != nil {
			return nil, nil, err
		}
	}
	skippedLog := filepath.Join(r.state, "skipped.log")
	before := len(logEntries(skippedLog))

	c := r.chezmoi(append([]string{"apply", "--force", "--keep-going"}, extra...)...)
	c.Env, c.Attach = append(c.Env, r.yesEnv()...), true
	_, err = r.Runner.Run(ctx, c)
	skipped, failed = logEntries(skippedLog)[before:], logEntries(failedLog)
	if err == nil && len(failed) > 0 {
		err = errors.New("an install step failed")
	}
	return skipped, failed, err
}

func (r *run) applyAndSummarise(ctx context.Context) error {
	r.ui.step("Applying")
	skipped, failed, err := r.apply(ctx)
	r.ui.step("Summary")
	r.ui.summary(scriptRows(skipped, failed))
	if len(skipped) > 0 {
		r.ui.info("Skipped steps are listed in " + filepath.Join(r.state, "skipped.log"))
	}
	if err != nil {
		r.ui.warn("Some steps failed (log: " + filepath.Join(r.state, "failed.log") + ").")
		r.ui.warn("Fix the cause, then run `dev setup --resume`; finished steps are not repeated.")
		return ErrReported
	}
	r.ui.info("Done. Open a new terminal (or run `exec zsh`).")
	return nil
}

func scriptRows(skipped, failed []string) []row {
	var rows []row
	for _, f := range failed {
		rows = append(rows, row{statusFailed, f})
	}
	for _, s := range skipped {
		rows = append(rows, row{statusSkipped, s})
	}
	return rows
}

// logEntries reads a state log, dropping each entry's timestamp.
func logEntries(path string) []string {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var out []string
	for _, l := range nonEmptyLines(string(b)) {
		if _, rest, ok := strings.Cut(l, "\t"); ok {
			l = rest
		}
		out = append(out, l)
	}
	return out
}

// backup copies the current version of each changed file (lines of `chezmoi
// status`) to ~/.dotfiles-backup/<time>/, following links. It returns the
// directory and how many files it copied.
func backup(home string, statusLines []string) (string, int, error) {
	dir := filepath.Join(home, ".dotfiles-backup", time.Now().Format("20060102-150405"))
	n := 0
	for _, line := range statusLines {
		if len(line) < 4 {
			continue
		}
		rel := line[3:]
		src := filepath.Join(home, rel)
		if fi, err := os.Stat(src); err != nil || !fi.Mode().IsRegular() {
			continue
		}
		if err := copyFile(src, filepath.Join(dir, rel)); err != nil {
			return dir, n, err
		}
		n++
	}
	return dir, n, nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	fi, err := in.Stat()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o700); err != nil {
		return err
	}
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, fi.Mode().Perm())
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

// identity offers the steps that make the machine usable with GitHub: an SSH
// key, a gh login and the key on the account. Each one is skippable, and they
// need a person (a passphrase, a browser), so --yes only prints what is left.
func (r *run) identity(ctx context.Context) error {
	key := filepath.Join(r.home, ".ssh/id_ed25519")
	email := r.settings.Email
	_, ghErr := r.Runner.Run(ctx, sys.Command("gh", "auth", "status"))
	loggedIn := ghErr == nil

	if r.o.Yes {
		if !loggedIn {
			r.ui.info("Next: `gh auth login` to use GitHub from this machine.")
		}
		if !exists(key) {
			r.ui.info("No SSH key yet; `dev setup` (without --yes) offers to create one.")
		}
		return nil
	}
	r.ui.step("GitHub access")
	if !exists(key) {
		if ok, err := r.Prompter.Confirm("Create an SSH key (" + key + ")? ssh-keygen asks for a passphrase."); err != nil {
			return err
		} else if ok {
			if err := os.MkdirAll(filepath.Dir(key), 0o700); err != nil {
				return err
			}
			if err := r.attached(ctx, nil, "ssh-keygen", "-t", "ed25519", "-C", email, "-f", key); err != nil {
				r.ui.warn("ssh-keygen failed: " + err.Error())
			}
		}
	}
	if !loggedIn {
		if ok, err := r.Prompter.Confirm("Log in to GitHub with gh (opens a browser or shows a code)?"); err != nil {
			return err
		} else if ok {
			if err := r.attached(ctx, nil, "gh", "auth", "login", "--git-protocol", "ssh", "--skip-ssh-key"); err != nil {
				r.ui.warn("gh auth login failed: " + err.Error())
			} else {
				loggedIn = true
			}
		}
	}
	pub := key + ".pub"
	if !loggedIn || !exists(pub) {
		return nil
	}
	material, err := os.ReadFile(pub)
	if err != nil {
		return err
	}
	if listed, err := r.Runner.Run(ctx, sys.Command("gh", "ssh-key", "list")); err == nil && keyListed(listed, string(material)) {
		return nil
	}
	if ok, err := r.Prompter.Confirm("Add the SSH key to your GitHub account?"); err != nil || !ok {
		return err
	}
	host, _ := os.Hostname()
	if out, err := r.Runner.Run(ctx, sys.Command("gh", "ssh-key", "add", pub, "--title", host)); err != nil {
		r.ui.warn(fmt.Sprintf("gh ssh-key add failed: %v %s", err, strings.TrimSpace(out)))
	}
	return nil
}

// keyListed reports whether `gh ssh-key list` output contains the key's material.
func keyListed(list, pub string) bool {
	fields := strings.Fields(pub)
	return len(fields) >= 2 && strings.Contains(list, fields[1])
}
