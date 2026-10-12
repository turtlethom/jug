# pathpin shell integration for bash and zsh.
# Add to ~/.bashrc or ~/.zshrc:  eval "$(pathpin init bash)"   (or: pathpin init zsh)
# Pick your own command name with --cmd:  eval "$(pathpin init bash --cmd pp)"
__PATHPIN_CMD__() {
    local __pathpin_tmp __pathpin_dir __pathpin_ret
    __pathpin_tmp="$(mktemp "${TMPDIR:-/tmp}/pathpin.XXXXXX")" || return
    PATHPIN_CD_FILE="$__pathpin_tmp" PATHPIN_CMD=__PATHPIN_CMD__ command pathpin "$@"
    __pathpin_ret=$?
    __pathpin_dir="$(cat -- "$__pathpin_tmp" 2>/dev/null)"
    rm -f -- "$__pathpin_tmp"
    if [ -n "$__pathpin_dir" ] && [ -d "$__pathpin_dir" ]; then
        cd -- "$__pathpin_dir" || return
    fi
    return "$__pathpin_ret"
}
