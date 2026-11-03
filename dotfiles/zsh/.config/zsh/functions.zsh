function chpwd() { :; }
function mkcd() { mkdir -p -- "$1" && cd -- "$1"; }
function extract() {
 [[ -f "$1" ]] || { print -u2 "File not found: $1"; return 1; }
 case "$1" in
  *.tar.gz|*.tgz) tar -xzf "$1";; *.tar.xz|*.txz) tar -xJf "$1";;
  *.tar.bz2|*.tbz2) tar -xjf "$1";; *.tar) tar -xf "$1";;
  *.gz) gunzip "$1";; *.zip) unzip "$1";; *.7z) 7z x "$1";;
  *) print -u2 "Unsupported archive: $1"; return 1;;
 esac
}
function ctmp() { local d; d="$(mktemp -d)" && cd "$d"; }
hash -d projects="$HOME/projects"
hash -d downloads="$HOME/Downloads"
hash -d config="$HOME/.config"
hash -d dotfiles="$HOME/dev-environment"
hash -d repos="$HOME/repos"
# zmv examples: zmv '(*).jpeg' '$1.jpg'
