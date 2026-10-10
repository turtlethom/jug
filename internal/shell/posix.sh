# jug shell integration for bash and zsh.
# Add to ~/.bashrc or ~/.zshrc:  eval "$(jug init bash)"   (or: jug init zsh)
jug() {
    local __jug_tmp __jug_dir __jug_ret
    __jug_tmp="$(mktemp "${TMPDIR:-/tmp}/jug.XXXXXX")" || return
    JUG_CD_FILE="$__jug_tmp" command jug "$@"
    __jug_ret=$?
    __jug_dir="$(cat -- "$__jug_tmp" 2>/dev/null)"
    rm -f -- "$__jug_tmp"
    if [ -n "$__jug_dir" ] && [ -d "$__jug_dir" ]; then
        cd -- "$__jug_dir" || return
    fi
    return "$__jug_ret"
}
