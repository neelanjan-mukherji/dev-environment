#!/usr/bin/env bash
# Test code is passed as strings to child shells on purpose.
# shellcheck disable=SC2016
# Render the chezmoi source into a throwaway home for each profile and assert on
# the produced files. Never touches the real $HOME. Requires: chezmoi, zsh, bash.
# Usage: ci/test-render.sh [test-name-filter]
set -Eeuo pipefail

repo="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd -P)"
filter="${1:-}"
CHEZMOI="${CHEZMOI:-chezmoi}"
# The dev binary (`make build`); the bootstrap and dry-run tests run it.
DEV="${DEV:-$repo/cli/dist/dev}"
work="$(mktemp -d)"
# Go makes its module cache read-only; make everything writable so it can go.
trap 'chmod -R u+w "$work" 2>/dev/null; rm -rf "$work"' EXIT

pass=0 fail=0 failed=()

# render <name> <profile> [env assignments...] -> sets $home
render() {
    local name="$1" profile="$2"; shift 2
    local root="$work/$name"
    home="$root/home"
    mkdir -p "$home"
    env DEVENV_PROFILE="$profile" DEVENV_NAME="Test User" DEVENV_EMAIL="test@example.com" "$@" \
        "$CHEZMOI" init --source "$repo" --no-tty --promptDefaults \
        --config "$root/chezmoi.toml" --cache "$root/cache" --persistent-state "$root/state.boltdb" >/dev/null
    "$CHEZMOI" apply --source "$repo" --no-tty --force \
        --config "$root/chezmoi.toml" --cache "$root/cache" --persistent-state "$root/state.boltdb" \
        --destination "$home" --exclude scripts,externals ${RENDER_OS:+--override-data "{\"chezmoi\":{\"os\":\"$RENDER_OS\"}}"}
}

has()     { grep -qF -- "$2" "$home/$1" || { echo "    expected '$1' to contain: $2"; return 1; }; }
lacks()   { ! grep -qF -- "$2" "$home/$1" || { echo "    expected '$1' NOT to contain: $2"; return 1; }; }
exists()  { [[ -e "$home/$1" ]] || { echo "    missing file: $1"; return 1; }; }

t() {
    local name="$1"; shift
    [[ -z "$filter" || "$name" == *"$filter"* ]] || return 0
    # On CI, name the test before it runs, so a hang shows which one.
    if [[ -n "${CI:-}" ]]; then echo "run  $name"; fi
    # Run outside any conditional so `set -e` aborts the test at its first failed assertion.
    local log="$work/$name.log" rc
    set +e; ( set -e; "$@" ) >"$log" 2>&1; rc=$?; set -e
    if ((rc == 0)); then pass=$((pass + 1)); echo "ok   $name"
    else fail=$((fail + 1)); failed+=("$name"); echo "FAIL $name"; sed 's/^/    /' "$log"; fi
}

# ---------------------------------------------------------------- tests

test_identity_goes_to_local_gitconfig_only() {
    render identity minimal
    has .gitconfig.local 'name = "Test User"'
    has .gitconfig.local 'email = "test@example.com"'
    has .gitconfig 'path = ~/.gitconfig.local'
    lacks .gitconfig '[user]'
}

test_minimal_profile_installs_only_core_cli_tools() {
    render mise-minimal minimal
    local f=.config/mise/config.toml
    for tool in fzf ripgrep fd bat eza zoxide starship jq jless lazygit yazi duf gh neovim tmux btop; do
        has "$f" "$tool = "
    done
    for tool in node go java kubectl; do lacks "$f" "$tool = "; done; lacks "$f" kondo
}

test_dev_profile_adds_languages_but_not_kubernetes_or_java() {
    render mise-dev dev
    local f=.config/mise/config.toml
    has "$f" 'node = "26"'
    has "$f" 'go = "1.27"'
    has "$f" 'python = "3.14"'
    has "$f" 'rust = "stable"'
    has "$f" '"github:tbillington/kondo"'
    lacks "$f" 'java = '
    lacks "$f" 'kubectl'
}

test_full_profile_adds_all_four_corretto_jdks_and_kubernetes() {
    render mise-full full
    local f=.config/mise/config.toml
    has "$f" 'java = ["corretto-21", "corretto-25", "corretto-17", "corretto-8"]'
    has "$f" 'kubectl = "1.37"'
    has "$f" 'k9s = "latest"'
    # Its "latest" includes milestone builds (4.2.0-M2); it follows the Boot release line.
    has "$f" 'spring-boot = "4.1"'
    lacks "$f" 'awscli'
}

test_mise_retries_downloads_long_enough_to_ride_out_rate_limits() {
    # Maven Central and archive.apache.org answer 429 or time out under load.
    render mise-retries minimal
    has .config/mise/config.toml 'http_retries = 6'
}

test_components_can_be_added_and_removed_on_top_of_a_profile() {
    render mise-override dev DEVENV_ADD=cloud,java DEVENV_REMOVE=go
    local f=.config/mise/config.toml
    has "$f" 'awscli = "latest"'
    has "$f" 'corretto-21'
    lacks "$f" 'go = '
    has "$f" 'node = "26"'
}

test_cloud_brings_python_for_azure_cli() {
    render cloud-python minimal DEVENV_ADD=cloud
    has .config/mise/config.toml '"pipx:azure-cli"'
    has .config/mise/config.toml 'python = "3.14"'
}

test_config_dir_is_not_readable_by_other_users() {
    # ~/.config holds tokens (gh, cloud CLIs); applying must never widen its mode.
    render perms minimal
    local mode
    mode="$(stat -c %a "$home/.config")"
    [[ $mode == 700 ]] || { echo "    ~/.config mode is $mode"; return 1; }
}

test_desktop_is_in_no_profile_unless_added() {
    local root="$work/desktop-gating" p on
    for p in minimal dev full; do
        init_config "$root/$p" DEVENV_PROFILE="$p"
        on="$("$CHEZMOI" execute-template --source "$repo" --config "$root/$p/chezmoi.toml" \
            --cache "$root/$p/cache" --persistent-state "$root/$p/state.boltdb" '{{ includeTemplate "components" . }}')"
        [[ ",$on," != *,desktop,* ]] || { echo "    $p: $on"; return 1; }
    done
    init_config "$root/added" DEVENV_PROFILE=minimal DEVENV_ADD=desktop
    on="$("$CHEZMOI" execute-template --source "$repo" --config "$root/added/chezmoi.toml" \
        --cache "$root/added/cache" --persistent-state "$root/added/state.boltdb" '{{ includeTemplate "components" . }}')"
    # No requirements: adding desktop to minimal brings nothing else.
    [[ "$on" == desktop ]] || { echo "    minimal + desktop: $on"; return 1; }
}

# desktop_data '<template>': render a template against the desktop data.
desktop_data() {
    local root="$work/desktop-data"
    [[ -f "$root/chezmoi.toml" ]] || init_config "$root"
    "$CHEZMOI" execute-template --source "$repo" --config "$root/chezmoi.toml" \
        --cache "$root/cache" --persistent-state "$root/state.boltdb" "$1"
}

test_desktop_data_lists_shared_and_windows_only_extensions() {
    local shared windows
    shared="$(desktop_data '{{ range .desktop.extensions.shared }}{{ . }}{{ "\n" }}{{ end }}')"
    windows="$(desktop_data '{{ range .desktop.extensions.windows }}{{ . }}{{ "\n" }}{{ end }}')"
    # Marketplace IDs as published by Catppuccin and Microsoft.
    grep -qx 'catppuccin.catppuccin-vsc' <<<"$shared" || { echo "    shared: $shared"; return 1; }
    grep -qx 'ms-vscode-remote.remote-wsl' <<<"$windows" || { echo "    windows: $windows"; return 1; }
    ! grep -qx 'ms-vscode-remote.remote-wsl' <<<"$shared" || { echo "    WSL extension in the shared list"; return 1; }
    # Lower-case publisher.name, as `code --list-extensions` prints them (the doctor compares).
    ! grep -v '^[a-z0-9-]*\.[a-z0-9-]*$' <<<"$shared"$'\n'"$windows" || { echo "    bad ID"; return 1; }
}

test_desktop_data_has_the_reviewed_macos_tweaks() {
    local out
    out="$(desktop_data '{{ range .desktop.tweaks.darwin }}{{ .id }}|{{ .restart }}|{{ .command }}|{{ range .reads }}{{ .domain }} {{ .key }}={{ .value }};{{ end }}{{ "\n" }}{{ end }}')"
    # Keys as documented by Apple (defaults domains of Finder, the global domain and screencapture).
    local want
    for want in \
        'defaults write NSGlobalDomain AppleShowAllExtensions -bool true|NSGlobalDomain AppleShowAllExtensions=1;' \
        '|Finder|defaults write com.apple.finder AppleShowAllFiles -bool true|com.apple.finder AppleShowAllFiles=1;' \
        '|Finder|defaults write com.apple.finder ShowPathbar -bool true|com.apple.finder ShowPathbar=1;' \
        'NSGlobalDomain KeyRepeat=2;NSGlobalDomain InitialKeyRepeat=15;' \
        '|SystemUIServer|mkdir -p "$HOME/Pictures/Screenshots" && defaults write com.apple.screencapture location -string "$HOME/Pictures/Screenshots"|com.apple.screencapture location=$HOME/Pictures/Screenshots;'; do
        grep -qF -- "$want" <<<"$out" || { echo "    no tweak with: $want"; echo "$out"; return 1; }
    done
    [[ $(wc -l <<<"$out") == 5 ]] || { echo "$out"; return 1; }
    # Every tweak is per-user: no sudo.
    ! grep -q sudo <<<"$out"
}

test_desktop_data_has_the_reviewed_windows_tweaks() {
    local out
    out="$(desktop_data '{{ range .desktop.tweaks.windows }}{{ .id }}|{{ .restart }}|{{ .command }}|{{ range .reads }}{{ .key }}!{{ .name }}={{ .value }};{{ end }}|{{ .min_build }}{{ "\n" }}{{ end }}')"
    # Explorer's per-user settings (HKCU only: no administrator rights).
    local adv='HKCU\Software\Microsoft\Windows\CurrentVersion\Explorer\Advanced' menu='HKCU\Software\Classes\CLSID\{86ca1aa0-34aa-4e8b-a509-50c905bae2a2}\InprocServer32' want
    for want in \
        "|Explorer|reg.exe add \"$adv\" /v HideFileExt /t REG_DWORD /d 0 /f|$adv!HideFileExt=0;|" \
        "|Explorer|reg.exe add \"$adv\" /v Hidden /t REG_DWORD /d 1 /f|$adv!Hidden=1;|" \
        "|Explorer|reg.exe add \"$menu\" /f /ve|$menu!=;|22000"; do
        grep -qF -- "$want" <<<"$out" || { echo "    no tweak with: $want"; echo "$out"; return 1; }
    done
    [[ $(wc -l <<<"$out") == 3 ]] || { echo "$out"; return 1; }
    ! grep -q 'HKLM' <<<"$out"
}

test_desktop_tweak_ids_are_unique_and_described() {
    local ids
    ids="$(desktop_data '{{ range concat .desktop.tweaks.darwin .desktop.tweaks.windows }}{{ .id }}{{ if not .description }} UNDESCRIBED{{ end }}{{ "\n" }}{{ end }}')"
    ! grep -q UNDESCRIBED <<<"$ids" || { echo "$ids"; return 1; }
    # IDs name the declined markers in ~/.local/state/dev/: unique per OS and file-name safe.
    ! grep -v '^[a-z0-9-]*$' <<<"$ids" || { echo "$ids"; return 1; }
    [[ -z "$(desktop_data '{{ range .desktop.tweaks.darwin }}{{ .id }}{{ "\n" }}{{ end }}' | sort | uniq -d)" ]]
    [[ -z "$(desktop_data '{{ range .desktop.tweaks.windows }}{{ .id }}{{ "\n" }}{{ end }}' | sort | uniq -d)" ]]
}

test_unknown_component_is_rejected() {
    ! render bad-component dev DEVENV_ADD=nope 2>&1
}

# Interactive shells get 60s: one waiting for an answer (e.g. compinit's
# "insecure directories" question) fails its test instead of hanging CI.
# Start an interactive shell against the rendered home with no plugins or
# mise-managed tools present (externals and scripts are excluded), and no network.
shell_in_home() {
    local sh="$1"; shift
    local out
    # `script` gives the shell a real pty, as in normal use. Any warning or error
    # it prints shows up as extra output, so the marker must be the only line.
    out="$(env -i HOME="$home" PATH="/usr/bin:/bin" TERM=xterm-256color \
        timeout -k 5 60 script -qec "$sh -i -c '$* && echo __OK__'" /dev/null </dev/null | tr -d '\r')"
    [[ "$out" == "__OK__" ]] || { echo "    $sh output (expected only __OK__):"; printf '%s\n' "$out" | sed 's/^/      /'; return 1; }
}

test_zsh_starts_cleanly_without_plugins_and_has_shared_aliases() {
    render zsh minimal
    shell_in_home zsh 'alias gs >/dev/null && (( ${+galiases[G]} )) && whence -w y zi >/dev/null'
}

test_zsh_does_not_stop_at_insecure_completion_dirs() {
    # GitHub's runner image has world-writable completion dirs; a plain compinit
    # asks about them and an unattended shell waits forever.
    render zsh-insecure minimal
    local d="$work/insecure-fpath" fp out
    mkdir -p "$d"; chmod 777 "$d"
    fp="$(env -i zsh -fc 'echo $FPATH')"
    out="$(env -i HOME="$home" PATH="/usr/bin:/bin" TERM=xterm-256color FPATH="$d:$fp" \
        timeout -k 5 30 script -qec "zsh -i -c 'echo __OK__'" /dev/null </dev/null | tr -d '\r')"
    [[ "$out" == *__OK__* ]] || { echo "    zsh did not start:"; printf '%s\n' "$out" | sed 's/^/      /'; return 1; }
}

test_bash_fallback_has_the_same_aliases_and_functions() {
    render bash minimal
    shell_in_home bash '[[ :$PATH: == *":$HOME/.local/bin:"* ]] && alias gs >/dev/null && alias gco >/dev/null && declare -F y zi >/dev/null && [[ $EDITOR == nvim ]]'
}

# shell_output <shell> <command>: what an interactive shell in $home prints.
shell_output() {
    env -i HOME="$home" PATH="/usr/bin:/bin" TERM=xterm-256color \
        timeout -k 5 60 script -qec "$1 -i -c '$2'" /dev/null </dev/null | tr -d '\r'
}

test_shells_say_once_a_day_when_the_repo_is_behind_and_never_update() {
    render notice minimal
    local state="$home/.local/state/dev" sh out
    mkdir -p "$home/.local/bin" "$state"
    printf '#!/bin/sh\necho "$*" >>"$HOME/dev-calls"\n' >"$home/.local/bin/dev"
    chmod +x "$home/.local/bin/dev"
    for sh in zsh bash; do
        # A day-old check that found 3 new commits: tell, and check again in the background.
        printf '3\n' >"$state/update-check"
        touch -d '2 days ago' "$state/update-check"
        rm -f "$home/dev-calls"
        out="$(shell_output "$sh" true)"
        grep -q '3 new commit.*dev update' <<<"$out" || { echo "    $sh printed:"; echo "$out"; return 1; }
        for _ in 1 2 3 4 5 6 7 8 9 10; do [[ -s "$home/dev-calls" ]] && break; sleep 0.2; done
        [[ "$(cat "$home/dev-calls" 2>/dev/null)" == "update --check" ]] || { echo "    $sh did not run the check"; return 1; }
        # Turned off in the local overrides: silent even when behind.
        printf '3\n' >"$state/update-check"
        printf 'export DEVENV_NO_UPDATE_CHECK=1\n' | tee "$home/.zshrc.local" >"$home/.bashrc.local"
        out="$(shell_output "$sh" true)"
        rm -f "$home/.zshrc.local" "$home/.bashrc.local"
        [[ -z "$out" ]] || { echo "    $sh ignored DEVENV_NO_UPDATE_CHECK in its local overrides:"; echo "$out"; return 1; }
        # Up to date and checked recently: silent, and no check.
        printf '0\n' >"$state/update-check"
        rm -f "$home/dev-calls"
        out="$(shell_output "$sh" true)"
        [[ -z "$out" ]] || { echo "    $sh printed:"; echo "$out"; return 1; }
        sleep 0.5
        [[ ! -e "$home/dev-calls" ]] || { echo "    $sh checked although it checked recently"; return 1; }
    done
}

test_macos_gets_wezterm_with_the_theme_and_nerd_font_and_linux_does_not() {
    RENDER_OS=darwin render wezterm-mac minimal DEVENV_THEME=macchiato
    has .config/wezterm/wezterm.lua 'config.color_scheme = "Catppuccin Macchiato"'
    has .config/wezterm/wezterm.lua 'JetBrainsMono Nerd Font'
    render wezterm-linux minimal
    [[ ! -e "$home/.config/wezterm" ]] || { echo "    wezterm config rendered on linux"; return 1; }
}

test_path_order_survives_a_system_reordering_like_macos_path_helper() {
    # /etc/zprofile's path_helper runs after ~/.zshenv and puts the system first;
    # ~/.zprofile sources path.sh again, which must move our dirs back to the front.
    render path-helper minimal
    exists .zprofile
    has .zprofile '.config/shell/path.sh'
    local want="$home/.local/bin:$home/bin:$home/.local/share/mise/shims:$home/.cargo/bin:/usr/bin:/bin"
    local reordered="/usr/bin:/bin:$home/.cargo/bin:$home/.local/bin" sh got
    for sh in sh zsh bash; do
        got="$(env -i HOME="$home" PATH="$reordered" "$sh" -c '. "$HOME/.config/shell/path.sh"; echo "$PATH"')"
        [[ "$got" == "$want" ]] || { echo "    $sh: $got"; echo "    want: $want"; return 1; }
    done
}

test_shell_files_are_syntactically_valid() {
    render syntax full
    cd "$home"
    for f in .zshrc .zshenv .config/zsh/*.zsh .config/shell/*.sh; do zsh -n "$f" || { echo "    zsh -n failed: $f"; return 1; }; done
    for f in .bashrc .profile .config/shell/*.sh; do bash -n "$f" || { echo "    bash -n failed: $f"; return 1; }; done
}

test_v1_version_managers_are_gone_and_mise_is_activated() {
    render managers full
    ! grep -rlE 'sdkman|NVM_DIR' "$home" || { echo "    SDKMAN/nvm references remain"; return 1; }
    has .zshrc 'mise activate zsh'
    has .bashrc 'mise activate bash'
    # Non-interactive shells get mise shims via path.sh (see the PATH order test).
    has .config/shell/path.sh 'mise/shims'
}

# Expected hex values come from Catppuccin's published palette, not from our data file.
test_theme_defaults_to_catppuccin_mocha_across_tools() {
    render theme-mocha minimal
    has .config/starship.toml 'palette = "catppuccin_mocha"'
    has .config/starship.toml 'mauve = "#cba6f7"'
    has .config/shell/env.sh 'bg+:#313244'
    has .tmux.conf '#1e1e2e'
    has .config/lazygit/config.yml '"#89b4fa"'
    has .config/nvim/lua/config/theme.lua 'flavor = "mocha"'
    has .config/bat/config '--theme="Catppuccin Mocha"'
    has .config/yazi/theme.toml 'dark = "catppuccin-mocha"'
    has .config/btop/btop.conf 'color_theme = "catppuccin_mocha"'
}

test_theme_flavor_is_switchable_in_one_place_and_survives_reinit() {
    render theme-latte minimal DEVENV_THEME=latte
    has .config/starship.toml 'mauve = "#8839ef"'
    has .config/bat/config '--theme="Catppuccin Latte"'
    has .config/nvim/lua/config/theme.lua 'flavor = "latte"'
    # Re-running init without the env var must keep the machine's choice.
    render theme-latte minimal
    has .config/starship.toml 'palette = "catppuccin_latte"'
}

test_unknown_theme_flavor_is_rejected() {
    ! render bad-theme minimal DEVENV_THEME=solarized 2>&1
}

test_clipboard_is_chosen_at_runtime_not_hardwired_to_wsl() {
    render clipboard minimal
    lacks .tmux.conf 'copy-pipe-and-cancel "clip.exe"'
    has .tmux.conf 'copy-pipe-and-cancel "clipcopy"'
    has .local/bin/clipcopy 'clip.exe'
    has .local/bin/clipcopy 'pbcopy'
    [[ -x "$home/.local/bin/clipcopy" ]]
    has .config/nvim/init.lua 'has("wsl")'
}

test_btop_keeps_its_own_settings_and_only_the_theme_line_is_managed() {
    render btop minimal
    printf '#? Config file for btop\ncolor_theme = "Default"\nupdate_ms = 1500\n' > "$home/.config/btop/btop.conf"
    render btop minimal
    has .config/btop/btop.conf 'color_theme = "catppuccin_mocha"'
    has .config/btop/btop.conf 'update_ms = 1500'
    lacks .config/btop/btop.conf '"Default"'
}

# Render each install script for a profile; prints "<name>\t<rendered path>".
# render_scripts <profile> [os]: every install script chezmoi runs for that
# profile (and OS, default this one), rendered; prints "name<TAB>path" per script.
render_scripts() {
    local profile="$1" os="${2:-}"
    # One root per DEVENV_ADD, since a re-init carries the previous run's additions over.
    local root="$work/scripts-$profile-${os:-here}${DEVENV_ADD:+-add-${DEVENV_ADD//,/-}}" f out
    local override=()
    [[ -n "$os" ]] && override=(--override-data "{\"chezmoi\":{\"os\":\"$os\"}}")
    mkdir -p "$root/out"
    DEVENV_PROFILE="$profile" DEVENV_NAME=T DEVENV_EMAIL=t@example.com "$CHEZMOI" init --source "$repo" --no-tty --promptDefaults \
        --config "$root/chezmoi.toml" --cache "$root/cache" --persistent-state "$root/state.boltdb" >/dev/null
    # The scripts chezmoi runs on that OS (.chezmoiignore drops .sh on Windows, .ps1 elsewhere).
    local managed
    managed="$("$CHEZMOI" managed --source "$repo" --config "$root/chezmoi.toml" --cache "$root/cache" \
        --persistent-state "$root/state.boltdb" --destination "$root/home" --include scripts \
        --path-style source-relative "${override[@]}")"
    for f in $managed; do
        f="$repo/home/$f"
        out="$root/out/$(basename "$f" .tmpl)"
        if "$CHEZMOI" execute-template --source "$repo" --config "$root/chezmoi.toml" \
            --cache "$root/cache" --persistent-state "$root/state.boltdb" "${override[@]}" <"$f" >"$out" 2>"$out.err"; then
            printf '%s\t%s\n' "$(basename "$out")" "$out"
        else
            printf '!render-failed:%s\t%s\n' "$(basename "$f")" "$out.err"
        fi
    done
}

test_install_scripts_pass_shellcheck_for_every_profile() {
    local p os name out
    for os in linux darwin; do
        for p in minimal dev full; do
            while IFS=$'\t' read -r name out; do
                [[ "$name" != '!render-failed'* ]] || { echo "    $os/$p: ${name#!render-failed:}"; cat "$out"; return 1; }
                [[ -n "$(tr -d '[:space:]' <"$out")" ]] || continue   # chezmoi skips empty scripts
                shellcheck -s bash "$out" || { echo "    shellcheck failed: $os/$p/$name"; return 1; }
            done < <(render_scripts "$p" "$os")
        done
    done
}

# rendered_names <profile> <os>: the install scripts that are not empty there.
rendered_names() {
    local name out
    while IFS=$'\t' read -r name out; do
        [[ "$name" != '!render-failed'* ]] || { echo "$name"; continue; }
        if [[ -n "$(tr -d '[:space:]' <"$out")" ]]; then echo "$name"; fi
    done < <(render_scripts "$1" "$2")
}

test_each_os_gets_its_own_package_and_docker_steps() {
    local mac linux
    mac="$(rendered_names dev darwin)"
    linux="$(rendered_names dev linux)"
    printf 'darwin:\n%s\nlinux:\n%s\n' "$mac" "$linux"   # shown if an assertion fails
    grep -q 'system-packages-darwin' <<<"$mac"
    grep -q 'docker-darwin' <<<"$mac"
    grep -qE 'system-packages\.sh|docker\.sh' <<<"$mac" && return 1   # no apt steps on macOS
    grep -qE 'system-packages\.sh' <<<"$linux"
    grep -q darwin <<<"$linux" && return 1
    # Both get the login shell step; minimal macOS gets no Docker.
    grep -q login-shell <<<"$mac"
    grep -q login-shell <<<"$linux"
    ! grep -q docker <<<"$(rendered_names minimal darwin)"
}

test_docker_script_only_runs_when_docker_component_is_enabled() {
    local name out
    while IFS=$'\t' read -r name out; do
        [[ $name == *docker* ]] || continue
        [[ -z "$(tr -d '[:space:]' <"$out")" ]] || { echo "    minimal profile renders a docker script"; return 1; }
    done < <(render_scripts minimal)
    while IFS=$'\t' read -r name out; do
        [[ $name == *docker* ]] && grep -q 'download.docker.com' "$out" && return 0
    done < <(render_scripts dev)
    echo "    dev profile has no docker install script"; return 1
}

test_mise_install_reruns_when_the_tool_list_changes() {
    local a b
    a="$(render_scripts minimal | awk -F'\t' '$1 ~ /mise/ {print $2}' | xargs cat)"
    b="$(render_scripts full | awk -F'\t' '$1 ~ /mise/ {print $2}' | xargs cat)"
    [[ -n "$a" && "$a" != "$b" ]] || { echo "    mise install script is identical across profiles (run_onchange would not fire)"; return 1; }
}

test_externals_fetch_plugin_managers_and_flavor_theme_files() {
    render externals minimal DEVENV_THEME=frappe
    local list
    list="$("$CHEZMOI" managed --source "$repo" --config "$work/externals/chezmoi.toml" \
        --cache "$work/externals/cache" --persistent-state "$work/externals/state.boltdb" \
        --destination "$home" --include externals --path-style relative)"
    for want in ".local/share/zinit/zinit.git" ".tmux/plugins/tpm" ".config/bat/themes/Catppuccin Frappe.tmTheme" \
                ".config/btop/themes/catppuccin_frappe.theme" ".config/yazi/flavors/catppuccin-frappe.yazi/flavor.toml" \
                ".config/yazi/flavors/catppuccin-latte.yazi/flavor.toml" ".config/eza/theme.yml"; do
        grep -qxF "$want" <<<"$list" || { echo "    externals missing: $want"; printf '%s\n' "$list" | sed 's/^/      /'; return 1; }
    done
}

test_login_sh_and_zsh_get_the_same_path_order() {
    render path minimal
    local want="$home/.local/bin:$home/bin:$home/.local/share/mise/shims:$home/.cargo/bin:/usr/bin:/bin"
    local sh_path zsh_path
    sh_path="$(env -i HOME="$home" PATH=/usr/bin:/bin sh -c '. "$HOME/.profile"; echo "$PATH"')"
    zsh_path="$(env -i HOME="$home" PATH=/usr/bin:/bin zsh -c 'echo "$PATH"')"
    [[ "$sh_path" == "$want" ]] || { echo "    .profile PATH: $sh_path"; echo "    expected:      $want"; return 1; }
    [[ "$zsh_path" == "$want" ]] || { echo "    .zshenv PATH:  $zsh_path"; echo "    expected:      $want"; return 1; }
}

# init_only <name> [env assignments...]: chezmoi init in a fake HOME, no identity
# preset; prints the generated config.
init_only() {
    local root="$work/$1"; shift
    mkdir -p "$root/fakehome"
    [[ -f "$root/gitconfig" ]] && cp "$root/gitconfig" "$root/fakehome/.gitconfig"
    env -u DEVENV_PROFILE -u DEVENV_NAME -u DEVENV_EMAIL -u GH_TOKEN -u GITHUB_TOKEN \
        HOME="$root/fakehome" XDG_CONFIG_HOME="$root/fakehome/.config" "$@" \
        "$CHEZMOI" init --source "$repo" --no-tty --promptDefaults \
        --config "$root/chezmoi.toml" --cache "$root/cache" --persistent-state "$root/state.boltdb" >/dev/null
    cat "$root/chezmoi.toml"
}

test_component_overrides_can_be_emptied_again() {
    # dev remove passes a single space for "no overrides left"; an unset variable keeps the old ones.
    local root="$work/empty-overrides" cfg
    mkdir -p "$root/fakehome"
    run_init() {
        env -u DEVENV_ADD -u DEVENV_REMOVE HOME="$root/fakehome" DEVENV_PROFILE=dev DEVENV_NAME=' ' DEVENV_EMAIL=' ' "$@" \
            "$CHEZMOI" init --source "$repo" --no-tty --promptDefaults \
            --config "$root/chezmoi.toml" --cache "$root/cache" --persistent-state "$root/state.boltdb" >/dev/null
    }
    run_init DEVENV_ADD=cloud DEVENV_REMOVE=go
    run_init
    cfg="$(cat "$root/chezmoi.toml")"
    grep -qx 'add = \["cloud"\]' <<<"$cfg" || { echo "$cfg"; return 1; }
    run_init DEVENV_ADD=' ' DEVENV_REMOVE=' '
    cfg="$(cat "$root/chezmoi.toml")"
    grep -qx 'add = \[\]' <<<"$cfg" || { echo "$cfg"; return 1; }
    grep -qx 'remove = \[\]' <<<"$cfg" || { echo "$cfg"; return 1; }
}

test_dev_profile_reads_the_selection_from_a_real_chezmoi_config() {
    local root="$work/dev-profile" out
    mkdir -p "$root/home"
    env -u DEVENV_ADD -u DEVENV_REMOVE HOME="$root/home" XDG_CONFIG_HOME="" DEVENV_PROFILE=dev DEVENV_NAME=' ' DEVENV_EMAIL=' ' \
        DEVENV_ADD=cloud DEVENV_REMOVE=go "$CHEZMOI" init --source "$repo" --no-tty --promptDefaults >/dev/null
    out="$(env HOME="$root/home" XDG_CONFIG_HOME="" PATH="$PATH" "$DEV" profile 2>&1)" || { echo "$out"; return 1; }
    local want
    for want in 'profile: *dev' '+ cloud' '- go'; do
        grep -q -- "$want" <<<"$out" || { echo "$out"; return 1; }
    done
    # Effective: the dev profile without go, plus cloud and the python it needs.
    grep -q 'components: docker,devtools,node,python,rust,cloud' <<<"$out" || { echo "$out"; return 1; }
}

test_dev_knows_the_same_components_as_the_config_template() {
    local template cli
    template="$(sed -n 's/.*\$known := list \(.*\) -}}.*/\1/p' "$repo/home/.chezmoi.toml.tmpl" | tr -d '"' | tr ' ' '\n' | sort)"
    cli="$("$DEV" add --help | sed -n 's/.*Add components (\([^)]*\)).*/\1/p' | head -1 | tr -d ' ' | tr ',' '\n' | sort)"
    [[ -n "$template" && "$template" == "$cli" ]] || { echo "    template: ${template//$'\n'/ }"; echo "    dev:      ${cli//$'\n'/ }"; return 1; }
}

test_bootstrap_help_works_when_piped_into_sh() {
    local out
    out="$(sh -s -- --help <"$repo/install.sh")"
    grep -q 'DEVENV_DEV_BIN' <<<"$out" || { echo "$out"; return 1; }
    grep -q 'dev setup --help' <<<"$out" || { echo "$out"; return 1; }
}

test_profile_defaults_to_dev_but_full_when_migrating_from_v1() {
    grep -qx 'profile = "dev"' <<<"$(init_only default-profile)"
    grep -qx 'profile = "full"' <<<"$(init_only v1-profile DEVENV_DEFAULT_PROFILE=full)"
}

test_identity_defaults_to_an_existing_github_noreply_address_only() {
    mkdir -p "$work/noreply" "$work/private"
    printf '[user]\n\tname = Jane Doe\n\temail = 123+jane@users.noreply.github.com\n' >"$work/noreply/gitconfig"
    printf '[user]\n\tname = Jane Doe\n\temail = jane@example.org\n' >"$work/private/gitconfig"
    local cfg
    cfg="$(init_only noreply)"
    grep -qx 'email = "123+jane@users.noreply.github.com"' <<<"$cfg" || { echo "$cfg"; return 1; }
    grep -qx 'name = "Jane Doe"' <<<"$cfg"
    # A personal address is never offered as the default for public commits.
    cfg="$(init_only private)"
    grep -qx 'email = ""' <<<"$cfg" || { echo "$cfg"; return 1; }
}

test_identity_defaults_also_come_from_an_existing_gitconfig_local() {
    # v1 machines keep the identity in ~/.gitconfig.local, not always included globally.
    mkdir -p "$work/local-id/fakehome"
    printf '[user]\n\tname = Jane Doe\n\temail = 123+jane@users.noreply.github.com\n' >"$work/local-id/fakehome/.gitconfig.local"
    local cfg
    cfg="$(init_only local-id)"
    grep -qx 'name = "Jane Doe"' <<<"$cfg" || { echo "$cfg"; return 1; }
    grep -qx 'email = "123+jane@users.noreply.github.com"' <<<"$cfg"
}

test_identity_left_blank_on_purpose_beats_the_git_config_default() {
    # dev setup passes a single space when a person clears the field.
    mkdir -p "$work/blank-id"
    printf '[user]\n\tname = Jane Doe\n\temail = 123+jane@users.noreply.github.com\n' >"$work/blank-id/gitconfig"
    local cfg
    cfg="$(init_only blank-id DEVENV_NAME=' ' DEVENV_EMAIL=' ')"
    grep -qx 'name = ""' <<<"$cfg" || { echo "$cfg"; return 1; }
    grep -qx 'email = ""' <<<"$cfg"
}

# lib_sh '<bash code>': run code after the rendered install-script library, with
# state in a scratch dir. Prints stdout; stderr goes to $work/lib.err.
lib_sh() {
    local root="$work/lib"
    mkdir -p "$root/state"
    [[ -f "$root/chezmoi.toml" ]] || DEVENV_PROFILE=minimal DEVENV_NAME=T DEVENV_EMAIL=t@example.com "$CHEZMOI" init --source "$repo" --no-tty --promptDefaults \
        --config "$root/chezmoi.toml" --cache "$root/cache" --persistent-state "$root/state.boltdb" >/dev/null
    printf '{{ template "lib.sh" . }}\n%s\n' "$1" |
        "$CHEZMOI" execute-template --source "$repo" --config "$root/chezmoi.toml" \
            --cache "$root/cache" --persistent-state "$root/state.boltdb" >"$root/script.sh"
    XDG_STATE_HOME="$root/state" bash -Eeuo pipefail "$root/script.sh" 2>"$work/lib.err"
}

test_commands_stay_visible_when_their_output_is_discarded() {
    lib_sh 'run echo hidden-output >/dev/null'
    grep -qF '$ echo hidden-output' "$work/lib.err" || { echo "    stderr:"; cat "$work/lib.err"; return 1; }
}

# init_config <dir> [VAR=value...]: a machine config in <dir> (minimal, test identity;
# DEVENV_PROFILE= in the caller's env leaves the profile to its default).
init_config() {
    local root="$1"; shift
    mkdir -p "$root"
    env DEVENV_PROFILE="${DEVENV_PROFILE-minimal}" DEVENV_NAME=T DEVENV_EMAIL=t@example.com "$@" \
        "$CHEZMOI" init --source "$repo" --no-tty --promptDefaults \
        --config "$root/chezmoi.toml" --cache "$root/cache" --persistent-state "$root/state.boltdb" >/dev/null
}

# render_script <dir> <script name prefix>: one install script rendered with <dir>'s config.
render_script() {
    local root="$1" f
    f="$(ls "$repo"/home/.chezmoiscripts/*"$2".sh.tmpl)"
    "$CHEZMOI" execute-template --source "$repo" --config "$root/chezmoi.toml" \
        --cache "$root/cache" --persistent-state "$root/state.boltdb" <"$f" >"$root/$2.sh"
    echo "$root/$2.sh"
}

# fake_distro <pm>: prints a PATH where the only package manager is a fake <pm>
# (dnf, pacman or apk) that has nothing installed, and sudo is passwordless.
# Package-manager and sudo calls are logged to $work/distro-<pm>/log.
# system_without <name>...: prints a dir of links to this system's commands,
# minus the package managers, sudo and the named ones.
system_without() {
    local sys="$work/sysbin${*:+-}${*// /-}" f
    if [[ ! -d "$sys" ]]; then
        mkdir -p "$sys"
        for f in /usr/bin/* /bin/*; do
            case " apt-get dpkg-query dnf rpm pacman apk sudo $* " in *" ${f##*/} "*) continue ;; esac
            [[ -e "$sys/${f##*/}" || -L "$sys/${f##*/}" ]] || ln -s "$f" "$sys/${f##*/}"
        done
    fi
    echo "$sys"
}

fake_distro() {
    local pm="$1" d="$work/distro-$1" sys
    sys="$(system_without)"
    rm -rf "$d"; mkdir -p "$d/bin"; : >"$d/log"
    fake() { printf '#!/bin/sh\n%s\n' "$2" >"$d/bin/$1"; chmod +x "$d/bin/$1"; }
    fake sudo 'case "$1" in -n) shift ;; -v) exit 0 ;; esac; echo "sudo $*" >>'"$d/log"'; exec "$@"'
    case "$pm" in
        dnf) fake dnf 'echo "dnf $*" >>'"$d/log"; fake rpm 'exit 1' ;;
        pacman) fake pacman '[ "$1" = -Q ] && exit 1; echo "pacman $*" >>'"$d/log" ;;
        apk) fake apk '[ "$1" = info ] && exit 1; echo "apk $*" >>'"$d/log" ;;
    esac
    echo "$d/bin:$sys"
}

test_system_packages_use_the_distros_package_manager() {
    local root="$work/pm" script path pm
    init_config "$root"
    script="$(render_script "$root" 10-system-packages)"
    for pm in dnf pacman apk; do
        path="$(fake_distro "$pm")"
        env -i HOME="$root/home" PATH="$path" XDG_STATE_HOME="$root/state" bash "$script" >/dev/null 2>&1 ||
            { echo "    $pm: script failed"; return 1; }
        local log="$work/distro-$pm/log"
        case "$pm" in
            dnf) grep -q '^sudo dnf install -y -q .*util-linux-user.*zsh' "$log" ;;
            pacman) grep -q '^sudo pacman -Syu --needed --noconfirm base-devel.*zsh' "$log" ;;
            apk) grep -q '^sudo apk add -q bash .*neovim.*zsh' "$log" ;;
        esac || { echo "    $pm:"; cat "$log"; return 1; }
    done
}

test_no_sudo_is_remembered_and_root_steps_are_skipped_not_run() {
    local root="$work/nosudo" script path
    init_config "$root" DEVENV_NO_SUDO=1
    grep -qx 'nosudo = true' "$root/chezmoi.toml" || { cat "$root/chezmoi.toml"; return 1; }
    init_config "$root"   # re-init without the variable keeps it
    grep -qx 'nosudo = true' "$root/chezmoi.toml" || { echo "    not remembered"; return 1; }
    script="$(render_script "$root" 10-system-packages)"
    path="$(fake_distro dnf)"   # passwordless sudo is there; no-sudo must not use it
    env -i HOME="$root/home" PATH="$path" XDG_STATE_HOME="$root/state" bash "$script" >/dev/null 2>&1
    ! grep -q sudo "$work/distro-dnf/log" || { echo "    used sudo:"; cat "$work/distro-dnf/log"; return 1; }
    grep -q 'no sudo' "$root/state/dev/skipped.log"
    init_config "$root" DEVENV_NO_SUDO=0
    grep -qx 'nosudo = false' "$root/chezmoi.toml" || { echo "    DEVENV_NO_SUDO=0 did not turn it off"; return 1; }
    # A new no-sudo machine defaults to minimal (docs/v2-design.md: minimal only); others to dev.
    DEVENV_PROFILE='' init_config "$work/nosudo-new" DEVENV_NO_SUDO=1
    grep -qx 'profile = "minimal"' "$work/nosudo-new/chezmoi.toml" || { grep profile "$work/nosudo-new/chezmoi.toml"; return 1; }
    DEVENV_PROFILE='' init_config "$work/sudo-new"
    grep -qx 'profile = "dev"' "$work/sudo-new/chezmoi.toml" || { grep profile "$work/sudo-new/chezmoi.toml"; return 1; }
}

# (The Alpine default profile, "minimal", is checked by the Alpine container run:
# chezmoi init ignores --override-data, so it can't be faked here.)
test_alpine_gets_neovim_and_jless_from_apk_not_mise() {
    local alpine='{"chezmoi":{"osRelease":{"id":"alpine"}}}'
    render alpine minimal
    "$CHEZMOI" apply --source "$repo" --no-tty --force --config "$work/alpine/chezmoi.toml" --cache "$work/alpine/cache" \
        --persistent-state "$work/alpine/state.boltdb" --destination "$home" --exclude scripts,externals --override-data "$alpine"
    lacks .config/mise/config.toml 'neovim ='
    lacks .config/mise/config.toml 'jless ='
    lacks .config/mise/config.toml 'ripgrep ='
    lacks .config/mise/config.toml 'eza ='
    has .config/mise/config.toml 'fd = "latest"'
    # Elsewhere mise keeps them.
    render not-alpine minimal
    has .config/mise/config.toml 'neovim = "0.11"'
    has .config/mise/config.toml 'jless = "latest"'
}

test_login_shell_step_reports_once_when_it_cannot_change_the_shell() {
    # No sudo and no terminal (a no-sudo server in CI): skipped once, then quiet,
    # so a second install stays a no-op.
    local root="$work/loginshell" script bin first second
    init_config "$root"
    script="$(render_script "$root" 40-login-shell)"
    local sys nozsh
    sys="$(system_without)"; nozsh="$(system_without zsh)"
    bin="$root/bin"; mkdir -p "$bin"
    printf '#!/bin/sh\necho "me:x:1000:1000::/home/me:/bin/bash"\n' >"$bin/getent"; chmod +x "$bin/getent"
    local yes
    for yes in 0 1; do   # asked nobody, or --yes
        rm -rf "$root/state"
        first="$(env -i HOME="$root/home" PATH="$bin:$sys" XDG_STATE_HOME="$root/state" DEVENV_YES=$yes setsid -w bash "$script" </dev/null 2>&1)"
        grep -q 'Login shell' <<<"$first" || { echo "    yes=$yes first run:"; echo "$first"; return 1; }
        second="$(env -i HOME="$root/home" PATH="$bin:$sys" XDG_STATE_HOME="$root/state" DEVENV_YES=$yes setsid -w bash "$script" </dev/null 2>&1)"
        [[ -z "$second" ]] || { echo "    yes=$yes second run was not quiet:"; echo "$second"; return 1; }
        [[ "$(grep -c 'login shell' "$root/state/dev/skipped.log")" = 1 ]]
    done
    # No zsh (base packages skipped without sudo): also said once, and retried
    # as soon as zsh shows up.
    rm -rf "$root/state"
    for _ in 1 2; do
        env -i HOME="$root/home" PATH="$bin:$nozsh" XDG_STATE_HOME="$root/state" DEVENV_YES=1 setsid -w bash "$script" </dev/null >/dev/null 2>&1
    done
    [[ "$(grep -c 'zsh is not installed' "$root/state/dev/skipped.log")" = 1 ]] || { cat "$root/state/dev/skipped.log"; return 1; }
    # zsh installed and sudo available: retried (fake sudo and chsh; never the real chsh).
    local can="$root/can"; mkdir -p "$can" "$root/home"
    printf '#!/bin/sh\ncase "$1" in -n) shift ;; -v) exit 0 ;; esac; exec "$@"\n' >"$can/sudo"
    printf '#!/bin/sh\necho "chsh $*" >>"$HOME/chsh.log"\n' >"$can/chsh"
    printf '#!/bin/sh\ncat >>"$HOME/etc-shells"\n' >"$can/tee"   # never the real /etc/shells
    chmod +x "$can/sudo" "$can/chsh" "$can/tee"
    first="$(env -i HOME="$root/home" PATH="$can:$bin:$sys" XDG_STATE_HOME="$root/state" DEVENV_YES=1 setsid -w bash "$script" </dev/null 2>&1)" ||
        { echo "    retry failed:"; echo "$first"; return 1; }
    grep -q 'chsh -s .*zsh' "$root/home/chsh.log" || { echo "    not retried once zsh and sudo were there"; return 1; }
}

# ---- Desktop: VS Code on Linux (#4)

test_linux_vscode_step_only_renders_with_desktop_on_linux() {
    local os names
    names="$(rendered_names full linux)"
    ! grep -q vscode <<<"$names" || { echo "    rendered without desktop:"; echo "$names"; return 1; }
    names="$(DEVENV_ADD=desktop rendered_names minimal linux)"
    grep -qx 'run_onchange_after_35-vscode.sh' <<<"$names" || { echo "    missing with desktop:"; echo "$names"; return 1; }
    for os in darwin windows; do
        names="$(DEVENV_ADD=desktop rendered_names minimal "$os")"
        ! grep -qx 'run_onchange_after_35-vscode.sh' <<<"$names" || { echo "    rendered on $os"; return 1; }
    done
}

test_linux_vscode_step_passes_shellcheck() {
    local name out
    while IFS=$'\t' read -r name out; do
        [[ "$name" == *vscode.sh ]] || continue
        shellcheck -s bash "$out" || return 1
        return 0
    done < <(DEVENV_ADD=desktop render_scripts minimal linux)
    echo "    no vscode script rendered"; return 1
}

# vscode_run <name> <PATH> <kernel osrelease> [VAR=value...]: runs the rendered Linux
# VS Code step (desktop on) without a tty; state in $work/vscode-<name>/state.
vscode_run() {
    local root="$work/vscode-$1" path="$2" kernel="$3" script; shift 3
    init_config "$root" DEVENV_ADD=desktop
    script="$(render_script "$root" 35-vscode)"
    printf '%s\n' "$kernel" >"$root/osrelease"
    env -i HOME="$root/home" PATH="$path" XDG_STATE_HOME="$root/state" DEVENV_OSRELEASE="$root/osrelease" "$@" \
        setsid -w bash "$script" </dev/null 2>&1
}

# vscode_skipped <name>: the skip reasons the step recorded.
vscode_skipped() { cut -f2- "$work/vscode-$1/state/dev/skipped.log" 2>/dev/null; }

# Kernel release strings as WSL 2 and a stock Ubuntu kernel report them.
wsl_kernel=6.6.87.2-microsoft-standard-WSL2
linux_kernel=6.8.0-45-generic

test_linux_vscode_step_is_skipped_under_wsl_because_the_host_owns_it() {
    local out path
    path="$(fake_distro dnf)"
    out="$(vscode_run wsl "$path" "$wsl_kernel" DEVENV_YES=1)" || { echo "$out"; return 1; }
    grep -qi '^vscode.*windows host' <<<"$(vscode_skipped wsl)" || { echo "$out"; return 1; }
    [[ ! -s "$work/distro-dnf/log" ]] || { echo "    the package manager ran:"; cat "$work/distro-dnf/log"; return 1; }
}

test_linux_vscode_step_is_skipped_on_pacman_and_apk_with_the_reason() {
    local out pm
    for pm in pacman apk; do
        out="$(vscode_run "$pm" "$(fake_distro "$pm")" "$linux_kernel" DEVENV_YES=1)" || { echo "$out"; return 1; }
        grep -q "^vscode.*$pm" <<<"$(vscode_skipped "$pm")" || { echo "    $pm:"; echo "$out"; return 1; }
        [[ ! -s "$work/distro-$pm/log" ]] || { echo "    $pm ran:"; cat "$work/distro-$pm/log"; return 1; }
    done
}

test_linux_vscode_step_is_skipped_without_sudo() {
    local out
    out="$(vscode_run nosudo "$(fake_distro dnf)" "$linux_kernel" DEVENV_YES=1 DEVENV_NO_SUDO=1)" || { echo "$out"; return 1; }
    grep -q '^vscode.*sudo' <<<"$(vscode_skipped nosudo)" || { echo "$out"; return 1; }
    [[ ! -s "$work/distro-dnf/log" ]] || { echo "    ran as root:"; cat "$work/distro-dnf/log"; return 1; }
}

test_linux_vscode_step_leaves_an_existing_code_alone() {
    local out path bin="$work/vscode-hand/bin"
    path="$(fake_distro dnf)"
    mkdir -p "$bin"
    printf '#!/bin/sh
echo 1.106.0
' >"$bin/code"; chmod +x "$bin/code"
    out="$(vscode_run hand "$bin:$path" "$linux_kernel" DEVENV_YES=1)" || { echo "$out"; return 1; }
    [[ ! -s "$work/distro-dnf/log" ]] || { echo "    reinstalled:"; cat "$work/distro-dnf/log"; return 1; }
    [[ -z "$(vscode_skipped hand)" ]] || { echo "    recorded a skip: $(vscode_skipped hand)"; return 1; }
}

test_linux_vscode_repository_is_only_added_after_asking() {
    local out
    # No tty and no --yes: nobody said yes, so nothing is written or installed.
    out="$(vscode_run unasked "$(fake_distro dnf)" "$linux_kernel")" || { echo "$out"; return 1; }
    grep -q '^vscode' <<<"$(vscode_skipped unasked)" || { echo "$out"; return 1; }
    # (Checking for passwordless sudo is fine; running anything as root is not.)
    ! grep -qvx 'sudo true' "$work/distro-dnf/log" || { echo "    ran without asking:"; cat "$work/distro-dnf/log"; return 1; }
}

# ---- Windows host

test_windows_host_gets_the_shared_configs_and_not_the_unix_shells() {
    RENDER_OS=windows render win minimal
    exists .config/nvim/init.lua
    exists .gitconfig
    exists .config/starship.toml
    exists .config/lazygit/config.yml
    exists .config/powershell/profile.ps1
    exists "AppData/Local/Microsoft/Windows Terminal/Fragments/dev-environment/theme.json"
    local f
    for f in .zshrc .bashrc .tmux.conf .config/zsh .config/mise .config/shell .local; do
        [[ ! -e "$home/$f" ]] || { echo "    $f rendered on Windows"; return 1; }
    done
    render not-win minimal
    [[ ! -e "$home/AppData" && ! -e "$home/.config/powershell" ]] || { echo "    Windows files rendered on Linux"; return 1; }
}

test_windows_terminal_scheme_matches_catppuccins_official_port() {
    # Expected values: github.com/catppuccin/windows-terminal mocha.json.
    RENDER_OS=windows render wt minimal DEVENV_THEME=mocha
    python3 - "$home/AppData/Local/Microsoft/Windows Terminal/Fragments/dev-environment/theme.json" <<'PY'
import json, sys
f = json.load(open(sys.argv[1]))
want = {"name": "Catppuccin Mocha", "cursorColor": "#F5E0DC", "selectionBackground": "#585B70",
        "background": "#1E1E2E", "foreground": "#CDD6F4", "black": "#45475A", "red": "#F38BA8",
        "green": "#A6E3A1", "yellow": "#F9E2AF", "blue": "#89B4FA", "purple": "#F5C2E7", "cyan": "#94E2D5",
        "white": "#BAC2DE", "brightBlack": "#585B70", "brightWhite": "#A6ADC8"}
got = {k: v.upper() if v.startswith("#") else v for k, v in f["schemes"][0].items()}
bad = {k: (got.get(k), v) for k, v in want.items() if got.get(k) != v}
assert not bad, bad
# Windows PowerShell and Command Prompt (GUIDs Windows Terminal derives from their names).
guids = {p["updates"] for p in f["profiles"]}
assert {"{61c54bbd-c2c6-5271-96e7-009a87ff44bf}", "{0caa0dad-35be-5f56-a8ff-afceeeaa6101}"} <= guids, guids
assert all(p["colorScheme"] == "Catppuccin Mocha" and p["font"]["face"] == "JetBrainsMono Nerd Font" for p in f["profiles"])
PY
}

test_windows_runs_only_powershell_install_scripts() {
    local win linux
    win="$(rendered_names full windows)"
    linux="$(rendered_names full linux)"
    grep -q 'winget-packages.ps1' <<<"$win" || { echo "$win"; return 1; }
    ! grep -qv '\.ps1$' <<<"$win" || { echo "    non-PowerShell scripts on Windows:"; echo "$win"; return 1; }
    ! grep -q '\.ps1$' <<<"$linux" || { echo "    PowerShell scripts on Linux:"; echo "$linux"; return 1; }
}

test_neovim_is_pinned_to_the_same_series_on_windows() {
    local series script
    render nvim-pin minimal
    series="$(sed -n 's/^neovim = "\(.*\)"$/\1/p' "$home/.config/mise/config.toml")"
    [[ -n "$series" ]] || { echo "    no neovim pin in the mise config"; return 1; }
    script="$(render_scripts minimal windows | awk -F'\t' '/winget-packages/ {print $2}')"
    grep -q "Id = 'Neovim.Neovim'; Version = '$series\.[0-9]*'; Pin = '$series\.\*'" "$script" ||
        { echo "    mise pins $series; winget:"; grep Neovim "$script"; return 1; }
}

test_powershell_files_parse() {
    if ! command -v pwsh >/dev/null 2>&1; then
        echo "    (pwsh not installed: parse check not run; CI runs it)" >&2
        return 0
    fi
    local files=() name out p
    for p in minimal full; do
        while IFS=$'\t' read -r name out; do files+=("$out"); done < <(render_scripts "$p" windows)
    done
    files+=("$repo/home/private_dot_config/powershell/profile.ps1" "$repo/install.ps1" "$repo/ci/verify.ps1")
    cat >"$work/parse.ps1" <<'PS'
$bad = 0
foreach ($f in $args) {
    $errors = $null
    [System.Management.Automation.Language.Parser]::ParseFile($f, [ref]$null, [ref]$errors) | Out-Null
    foreach ($e in $errors) { "    {0}:{1}: {2}" -f $f, $e.Extent.StartLineNumber, $e.Message; $bad++ }
}
exit [int]($bad -gt 0)
PS
    pwsh -NoLogo -NoProfile -File "$work/parse.ps1" "${files[@]}"
}

test_windows_bootstrap_shows_help_and_hands_over_to_setup() {
    if ! command -v pwsh >/dev/null 2>&1; then
        echo "    (pwsh not installed: not run; CI runs it)" >&2
        return 0
    fi
    local h="$work/bootstrap-win" out
    mkdir -p "$h"
    out="$(env HOME="$h" pwsh -NoLogo -NoProfile -File "$repo/install.ps1" --help)"
    grep -q 'DEVENV_DEV_BIN' <<<"$out" || { echo "$out"; return 1; }
    printf '#!/bin/sh\necho "fake dev $*"\n' >"$h/fake-dev"; chmod +x "$h/fake-dev"
    out="$(env HOME="$h" DEVENV_DEV_BIN="$h/fake-dev" PROCESSOR_ARCHITECTURE=AMD64 pwsh -NoLogo -NoProfile -File "$repo/install.ps1" --yes --profile minimal 2>&1)"
    grep -q "fake dev setup --yes --profile minimal --source $repo" <<<"$out" || { echo "$out"; return 1; }
}

test_powershell_steps_act_once_and_log_failures() {
    # Runs the rendered Windows scripts in pwsh (Linux pwsh in CI) against a fake
    # home and a fake Windows Terminal settings.json.
    if ! command -v pwsh >/dev/null 2>&1; then
        echo "    (pwsh not installed: not run; CI runs it)" >&2
        return 0
    fi
    local root="$work/pwsh" scripts hook wslf lib out
    scripts="$(render_scripts minimal windows)"
    hook="$(awk -F'\t' '/powershell-profile/ {print $2}' <<<"$scripts")"
    wslf="$(awk -F'\t' '/terminal-wsl-profiles/ {print $2}' <<<"$scripts")"
    local h="$root/home" la="$root/localappdata"
    local lst="$la/Packages/Microsoft.WindowsTerminal_8wekyb3d8bbwe/LocalState"
    mkdir -p "$h" "$lst"
    cat >"$lst/settings.json" <<'JSON'
{
    // older Terminal versions write comments
    "profiles": { "list": [
        { "guid": "{61c54bbd-c2c6-5271-96e7-009a87ff44bf}", "name": "Windows PowerShell" },
        { "guid": "{04fbbe2e-4bcf-5f93-aacf-0f7c217b572b}", "name": "Ubuntu-26.04", "source": "Microsoft.WSL" }
    ] }
}
JSON
    pw() { env HOME="$h" LOCALAPPDATA="$la" DEVENV_YES="${YES:-1}" pwsh -NoLogo -NoProfile -File "$@" 2>&1 </dev/null; }
    # Asked first: nobody to ask (no --yes, no terminal) means no change, recorded as a skip.
    out="$(YES=0 pw "$hook")"; grep -q 'Skipped: powershell profile line' <<<"$out" || { echo "$out"; return 1; }
    [[ ! -e "$h/Documents" ]] || { echo "    changed a profile without asking"; return 1; }
    # The profile hook: one line in each edition's profile, then silence.
    out="$(pw "$hook")"; grep -q 'PowerShell profile' <<<"$out" || { echo "$out"; return 1; }
    [[ -z "$(pw "$hook")" ]] || { echo "    profile hook not silent the second time"; return 1; }
    [[ "$(grep -rl 'config/powershell/profile.ps1"  # dev-environment' "$h/Documents" | wc -l)" = 2 ]]
    # WSL profiles get the theme through a second fragment, then silence.
    out="$(pw "$wslf")"; grep -q 'Theming 1 WSL profile' <<<"$out" || { echo "$out"; return 1; }
    [[ -z "$(pw "$wslf")" ]] || { echo "    WSL fragment step not silent the second time"; return 1; }
    grep -q '04fbbe2e-4bcf-5f93-aacf-0f7c217b572b' "$la/Microsoft/Windows Terminal/Fragments/dev-environment/wsl.json"
    # A failing command fails the step, is logged under its name, and its own flags pass through.
    lib="$root/fail.ps1"
    printf '{{ template "lib.ps1" . }}\nStep "Frobnicate"\nInvoke-Logged pwsh -NoProfile -Command "exit 3"\n' |
        "$CHEZMOI" execute-template --source "$repo" --config "$work/scripts-minimal-windows/chezmoi.toml" \
            --cache "$work/scripts-minimal-windows/cache" --persistent-state "$work/scripts-minimal-windows/state.boltdb" >"$lib"
    if pw "$lib" >/dev/null; then echo "    a failing command did not fail the step"; return 1; fi
    grep -q 'Frobnicate (pwsh exited with 3)' "$h/.local/state/dev/failed.log" || { cat "$h/.local/state/dev/failed.log"; return 1; }
}

test_jless_is_left_out_on_linux_arm64_where_it_has_no_build() {
    render arm64 minimal
    "$CHEZMOI" apply --source "$repo" --no-tty --force --config "$work/arm64/chezmoi.toml" --cache "$work/arm64/cache" \
        --persistent-state "$work/arm64/state.boltdb" --destination "$home" --exclude scripts,externals \
        --override-data '{"chezmoi":{"os":"linux","arch":"arm64"}}'
    lacks .config/mise/config.toml 'jless ='
    has .config/mise/config.toml 'ripgrep = "latest"'
    RENDER_OS=darwin render arm64-mac minimal   # macOS arm64 has a build
    has .config/mise/config.toml 'jless = "latest"'
}

test_a_failing_step_is_logged_with_its_name() {
    rm -f "$work/lib/state/dev/failed.log"
    lib_sh 'step "Frobnicate widgets"; false' && { echo "    expected the step to fail"; return 1; }
    grep -qF 'Frobnicate widgets' "$work/lib/state/dev/failed.log"
    # A step that finishes is not logged.
    rm -f "$work/lib/state/dev/failed.log"
    lib_sh 'step "Fine"; true'
    [[ ! -s "$work/lib/state/dev/failed.log" ]]
}

test_wsl_conf_systemd_is_enabled_without_duplicate_keys() {
    local out
    out="$(lib_sh 'printf "[boot]\nsystemd=false\ncommand=echo\n[network]\nhostname=x\n" | wsl_conf_with_systemd')"
    [[ "$out" == $'[boot]\nsystemd=true\ncommand=echo\n[network]\nhostname=x' ]] || { echo "    got:"; echo "$out"; return 1; }
    out="$(lib_sh 'printf "[network]\nhostname=x\n" | wsl_conf_with_systemd')"
    [[ "$out" == $'[network]\nhostname=x\n[boot]\nsystemd=true' ]] || { echo "    got:"; echo "$out"; return 1; }
    out="$(lib_sh 'printf "[boot]\ncommand=echo\n" | wsl_conf_with_systemd')"
    [[ "$out" == $'[boot]\nsystemd=true\ncommand=echo' ]] || { echo "    got:"; echo "$out"; return 1; }
    out="$(lib_sh 'wsl_conf_with_systemd </dev/null')"
    [[ "$out" == $'[boot]\nsystemd=true' ]] || { echo "    got:"; echo "$out"; return 1; }
}

test_dry_run_changes_nothing_in_home() {
    local fake="$work/dry/home"
    mkdir -p "$fake"
    printf 'original\n' >"$fake/.zshrc"
    env HOME="$fake" XDG_CONFIG_HOME="$fake/.config" XDG_STATE_HOME="" DEVENV_DIR="$work/dry/nonexistent" DEVENV_DEV_BIN="$DEV" \
        "$repo/install.sh" --dry-run --yes --profile minimal --name T --email t@example.com >"$work/dry/out" 2>&1 ||
        { tail -20 "$work/dry/out"; return 1; }
    grep -q "Install steps that would run" "$work/dry/out" || { tail -20 "$work/dry/out"; return 1; }
    [[ "$(cat "$fake/.zshrc")" == original ]]
    [[ "$(find "$fake" -mindepth 1 | wc -l)" -eq 1 ]] || { echo "    created:"; find "$fake" -mindepth 1; return 1; }
    [[ ! -e "$work/dry/nonexistent" ]]
}

# fake_release <dir>: a release like GoReleaser's, whose "dev" prints its arguments.
fake_release() {
    local dir="$1" os arch
    os="$(uname -s | tr '[:upper:]' '[:lower:]')"
    case "$(uname -m)" in x86_64) arch=amd64 ;; aarch64 | arm64) arch=arm64 ;; esac
    mkdir -p "$dir/build"
    printf '#!/bin/sh\necho "fake dev: $*"\n' >"$dir/build/dev"
    chmod +x "$dir/build/dev"
    tar -czf "$dir/dev_${os}_${arch}.tar.gz" -C "$dir/build" dev
    (cd "$dir" && sha256sum "dev_${os}_${arch}.tar.gz" >checksums.txt)
}

test_bootstrap_installs_a_verified_release_and_hands_over_to_setup() {
    local root="$work/boot" out
    fake_release "$root/release"
    mkdir -p "$root/home"
    out="$(env HOME="$root/home" DEVENV_RELEASE_URL="file://$root/release" \
        sh "$repo/install.sh" --from-nowhere --yes 2>&1)" || { echo "$out"; return 1; }
    [[ -x "$root/home/.local/bin/dev" ]] || { echo "$out"; return 1; }
    # From a clone, setup is pointed at it; every other argument is passed through.
    grep -qx "fake dev: setup --source $repo --from-nowhere --yes" <<<"$out" || { echo "$out"; return 1; }
}

test_bootstrap_from_source_builds_with_mise_when_go_is_not_on_path() {
    # A v1 machine has Go installed by mise but no shims on PATH.
    local h="$work/from-source" out want
    want="$(sed -n 's/^go //p' "$repo/cli/go.mod")"
    mkdir -p "$h/.local/bin"
    cat >"$h/.local/bin/mise" <<'MISE'
#!/bin/sh
# Fake: "mise exec go@V -- go build ... -o OUT ." writes a dev that reports its args.
echo "$*" >"$HOME/mise-args"
while [ $# -gt 0 ]; do [ "$1" = -o ] && out="$2"; shift; done
printf '#!/bin/sh\necho "fake dev $*"\n' >"$out" && chmod +x "$out"
MISE
    chmod +x "$h/.local/bin/mise"
    # No Go anywhere on PATH (CI runners have one in /usr/bin).
    out="$(env -i HOME="$h" PATH="$(system_without go gofmt)" sh "$repo/install.sh" --from-source --yes 2>&1)" || { echo "$out"; return 1; }
    grep -q "^exec go@$want -- go build" "$h/mise-args" || { cat "$h/mise-args"; return 1; }
    grep -q "fake dev setup --source $repo --yes" <<<"$out" || { echo "$out"; return 1; }
}

test_bootstrap_refuses_a_release_whose_checksum_does_not_match() {
    local root="$work/boot-bad" out
    fake_release "$root/release"
    sed -i 's/^[0-9a-f]*/'"$(printf '0%.0s' {1..64})"'/' "$root/release/checksums.txt"
    mkdir -p "$root/home"
    ! out="$(env HOME="$root/home" DEVENV_RELEASE_URL="file://$root/release" sh "$repo/install.sh" --yes 2>&1)" || { echo "$out"; return 1; }
    grep -q "Checksum mismatch" <<<"$out" || { echo "$out"; return 1; }
    [[ ! -e "$root/home/.local/bin/dev" ]]
}

# ---- Desktop: VS Code install on macOS and Windows (#3)
# IDs from Homebrew (formulae.brew.sh/cask/visual-studio-code) and winget-pkgs
# (manifests/m/Microsoft/VisualStudioCode), not from our data files.

# vscode_scripts <tag> <os> [VAR=value...]: the install scripts chezmoi runs on <os>
# for a minimal machine with those overrides, as "name<TAB>rendered path".
vscode_scripts() {
    local root="$work/vscode-$1" os="$2"; shift 2
    rm -rf "$root"
    init_config "$root" "$@"
    local f out
    for f in $("$CHEZMOI" managed --source "$repo" --config "$root/chezmoi.toml" --cache "$root/cache" \
        --persistent-state "$root/state.boltdb" --destination "$root/home" --include scripts \
        --path-style source-relative --override-data "{\"chezmoi\":{\"os\":\"$os\"}}"); do
        out="$root/$(basename "$f" .tmpl)"
        "$CHEZMOI" execute-template --source "$repo" --config "$root/chezmoi.toml" --cache "$root/cache" \
            --persistent-state "$root/state.boltdb" --override-data "{\"chezmoi\":{\"os\":\"$os\"}}" \
            <"$repo/home/$f" >"$out"
        printf '%s\t%s\n' "$(basename "$out")" "$out"
    done
}

# vscode_mentions <tag> <os> <id> [VAR=value...]: the scripts on <os> that name <id>.
vscode_mentions() {
    local tag="$1" os="$2" id="$3" name out; shift 3
    while IFS=$'\t' read -r name out; do
        if grep -qF -- "$id" "$out"; then echo "$name"; fi
    done < <(vscode_scripts "$tag" "$os" "$@")
}

test_vscode_is_installed_only_with_desktop_on_and_only_by_its_own_os() {
    local os got
    for os in linux darwin windows; do
        got="$(vscode_mentions "off-$os" "$os" visual-studio-code)$(vscode_mentions "off-$os" "$os" Microsoft.VisualStudioCode)"
        [[ -z "$got" ]] || { echo "    $os without desktop installs VS Code in: $got"; return 1; }
    done
    got="$(vscode_mentions on-darwin darwin visual-studio-code DEVENV_ADD=desktop)"
    [[ "$got" == *system-packages-darwin.sh ]] || { echo "    darwin cask in: '$got'"; return 1; }
    got="$(vscode_mentions on-windows windows Microsoft.VisualStudioCode DEVENV_ADD=desktop)"
    [[ "$got" == *winget-packages.ps1 ]] || { echo "    windows winget in: '$got'"; return 1; }
    # Neither OS gets the other's package.
    [[ -z "$(vscode_mentions on-darwin darwin Microsoft.VisualStudioCode DEVENV_ADD=desktop)" ]]
    [[ -z "$(vscode_mentions on-windows windows visual-studio-code DEVENV_ADD=desktop)" ]]
    [[ -z "$(vscode_mentions on-linux linux visual-studio-code DEVENV_ADD=desktop)$(vscode_mentions on-linux linux Microsoft.VisualStudioCode DEVENV_ADD=desktop)" ]]
    shellcheck -s bash "$work/vscode-on-darwin/run_before_11-system-packages-darwin.sh"
}

test_vscode_cask_is_installed_on_macos_unless_vscode_is_already_there() {
    local root="$work/vscode-cask" script sys out
    script="$(vscode_scripts cask-run darwin DEVENV_ADD=desktop | awk -F'\t' '/system-packages-darwin/ {print $2}')"
    sys="$(system_without brew code)"
    mkdir -p "$root/bin"
    # A Homebrew with every formula and the other casks installed; installs are logged.
    cat >"$root/bin/brew" <<SH
#!/bin/sh
case "\$1" in
    shellenv) ;;
    list) [ "\$2" = --cask ] && printf 'font-jetbrains-mono-nerd-font\nwezterm\n' || printf 'bash\ngit\ngnupg\ntmux\ntree\nxz\n' ;;
    *) echo "brew \$*" >>"$root/log" ;;
esac
SH
    chmod +x "$root/bin/brew"
    mac() { env -i HOME="$root/home" PATH="$root/bin:$sys" XDG_STATE_HOME="$root/state" bash "$script" 2>&1; }
    mkdir -p "$root/home"; : >"$root/log"
    out="$(mac)" || { echo "$out"; return 1; }
    grep -qx 'brew install --cask visual-studio-code' "$root/log" || { echo "$out"; cat "$root/log"; return 1; }
    # A VS Code installed by hand, as an app or a `code` on PATH, is left alone (and quiet).
    : >"$root/log"; mkdir -p "$root/home/Applications/Visual Studio Code.app"
    out="$(mac)"; [[ -z "$out" && ! -s "$root/log" ]] || { echo "$out"; cat "$root/log"; return 1; }
    rm -rf "$root/home/Applications"
    printf '#!/bin/sh\n' >"$root/bin/code"; chmod +x "$root/bin/code"
    out="$(mac)"; [[ -z "$out" && ! -s "$root/log" ]] || { echo "$out"; cat "$root/log"; return 1; }
}

test_vscode_is_installed_with_winget_for_the_user_unless_code_is_there() {
    if ! command -v pwsh >/dev/null 2>&1; then
        echo "    (pwsh not installed: not run; CI runs it)" >&2
        return 0
    fi
    local root="$work/vscode-winget" script out
    script="$(vscode_scripts winget-run windows DEVENV_ADD=desktop | awk -F'\t' '/winget-packages/ {print $2}')"
    mkdir -p "$root/bin" "$root/home"
    # winget has everything but VS Code; calls are logged.
    cat >"$root/bin/winget" <<SH
#!/bin/sh
echo "winget \$*" >>"$root/log"
case "\$1 \$*" in list*Microsoft.VisualStudioCode*) exit 1 ;; esac
exit 0
SH
    chmod +x "$root/bin/winget"
    win() { env HOME="$root/home" PATH="$root/bin:$PATH" DEVENV_YES=1 pwsh -NoLogo -NoProfile -File "$script" 2>&1 </dev/null; }
    : >"$root/log"
    out="$(win)" || { echo "$out"; return 1; }
    # User scope: no UAC prompt, no administrator rights.
    grep -q '^winget install --id Microsoft.VisualStudioCode --exact --source winget .*--scope user' "$root/log" ||
        { echo "$out"; cat "$root/log"; return 1; }
    # A `code` already on PATH is left alone.
    : >"$root/log"; printf '#!/bin/sh\n' >"$root/bin/code"; chmod +x "$root/bin/code"
    out="$(win)" || { echo "$out"; return 1; }
    ! grep -q 'Microsoft.VisualStudioCode' "$root/log" || { echo "$out"; cat "$root/log"; return 1; }
}

# ---------------------------------------------------------------- run

for fn in $(declare -F | awk '{print $3}' | grep '^test_'); do t "${fn#test_}" "$fn"; done
echo "---- $pass passed, $fail failed"
((fail == 0)) || { printf 'failed: %s\n' "${failed[@]}"; exit 1; }
