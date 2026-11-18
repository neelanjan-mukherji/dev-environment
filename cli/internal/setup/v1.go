package setup

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// v1Links are where v1 (GNU Stow) linked into its dotfiles/ tree.
var v1Links = []string{".zshrc", ".gitconfig", ".tmux.conf", ".config/nvim", ".config/zsh"}

// HasV1 reports whether this machine still has a v1 (Stow) setup to migrate.
// `dev setup` migrates it by itself; `dev migrate-v1` does only that.
func HasV1() bool {
	home, err := os.UserHomeDir()
	return err == nil && hasV1Links(home)
}

// hasV1Links reports whether home still has v1 Stow links. Besides the
// well-known ones, any link under ~/.config (3 levels) into a dotfiles/ tree counts.
func hasV1Links(home string) bool {
	for _, f := range v1Links {
		if intoDotfiles(filepath.Join(home, f)) {
			return true
		}
	}
	found := false
	root := filepath.Join(home, ".config")
	_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil || found {
			return fs.SkipDir
		}
		if strings.Count(strings.TrimPrefix(p, root), string(filepath.Separator)) > 3 {
			return fs.SkipDir
		}
		if d.Type()&fs.ModeSymlink != 0 && intoDotfiles(p) {
			found = true
			return fs.SkipAll
		}
		return nil
	})
	return found
}

func intoDotfiles(p string) bool {
	target, err := os.Readlink(p)
	return err == nil && strings.Contains(target, "dotfiles/")
}
