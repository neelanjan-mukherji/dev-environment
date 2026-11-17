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
work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT

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
        --destination "$home" --exclude scripts,externals
}

has()     { grep -qF -- "$2" "$home/$1" || { echo "    expected '$1' to contain: $2"; return 1; }; }
lacks()   { ! grep -qF -- "$2" "$home/$1" || { echo "    expected '$1' NOT to contain: $2"; return 1; }; }
exists()  { [[ -e "$home/$1" ]] || { echo "    missing file: $1"; return 1; }; }

t() {
    local name="$1"; shift
    [[ -z "$filter" || "$name" == *"$filter"* ]] || return 0
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
    has "$f" 'spring-boot = "latest"'
    lacks "$f" 'awscli'
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

test_unknown_component_is_rejected() {
    ! render bad-component dev DEVENV_ADD=nope 2>&1
}

# Start an interactive shell against the rendered home with no plugins or
# mise-managed tools present (externals and scripts are excluded), and no network.
shell_in_home() {
    local sh="$1"; shift
    local out
    # `script` gives the shell a real pty, as in normal use. Any warning or error
    # it prints shows up as extra output, so the marker must be the only line.
    out="$(env -i HOME="$home" PATH="/usr/bin:/bin" TERM=xterm-256color \
        script -qec "$sh -i -c '$* && echo __OK__'" /dev/null </dev/null | tr -d '\r')"
    [[ "$out" == "__OK__" ]] || { echo "    $sh output (expected only __OK__):"; printf '%s\n' "$out" | sed 's/^/      /'; return 1; }
}

test_zsh_starts_cleanly_without_plugins_and_has_shared_aliases() {
    render zsh minimal
    shell_in_home zsh 'alias gs >/dev/null && (( ${+galiases[G]} )) && whence -w y zi >/dev/null'
}

test_bash_fallback_has_the_same_aliases_and_functions() {
    render bash minimal
    shell_in_home bash '[[ :$PATH: == *":$HOME/.local/bin:"* ]] && alias gs >/dev/null && alias gco >/dev/null && declare -F y zi >/dev/null && [[ $EDITOR == nvim ]]'
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
render_scripts() {
    local profile="$1"
    local root="$work/scripts-$profile" f out
    mkdir -p "$root/out"
    DEVENV_PROFILE="$profile" DEVENV_NAME=T DEVENV_EMAIL=t@example.com "$CHEZMOI" init --source "$repo" --no-tty --promptDefaults \
        --config "$root/chezmoi.toml" --cache "$root/cache" --persistent-state "$root/state.boltdb" >/dev/null
    for f in "$repo"/home/.chezmoiscripts/*; do
        out="$root/out/$(basename "$f" .tmpl)"
        "$CHEZMOI" execute-template --source "$repo" --config "$root/chezmoi.toml" \
            --cache "$root/cache" --persistent-state "$root/state.boltdb" <"$f" >"$out"
        printf '%s\t%s\n' "$(basename "$out")" "$out"
    done
}

test_install_scripts_pass_shellcheck_for_every_profile() {
    local p name out
    for p in minimal dev full; do
        while IFS=$'\t' read -r name out; do
            [[ -n "$(tr -d '[:space:]' <"$out")" ]] || continue   # chezmoi skips empty scripts
            shellcheck -s bash "$out" || { echo "    shellcheck failed: $p/$name"; return 1; }
        done < <(render_scripts "$p")
    done
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
    env HOME="$fake" XDG_CONFIG_HOME="$fake/.config" DEVENV_DIR="$work/dry/nonexistent" \
        "$repo/install.sh" --dry-run --yes --profile minimal --name T --email t@example.com >/dev/null 2>&1
    [[ "$(cat "$fake/.zshrc")" == original ]]
    [[ "$(find "$fake" -mindepth 1 | wc -l)" -eq 1 ]] || { echo "    created:"; find "$fake" -mindepth 1; return 1; }
    [[ ! -e "$work/dry/nonexistent" ]]
}

# ---------------------------------------------------------------- run

for fn in $(declare -F | awk '{print $3}' | grep '^test_'); do t "${fn#test_}" "$fn"; done
echo "---- $pass passed, $fail failed"
((fail == 0)) || { printf 'failed: %s\n' "${failed[@]}"; exit 1; }
