# pathpin shell integration for bash and zsh.
# Add to ~/.bashrc or ~/.zshrc:  eval "$(pathpin init bash)"   (or: pathpin init zsh)
# Pick your own command name with --cmd:  eval "$(pathpin init bash --cmd pp)"
#
# How it works: the function runs the real binary with PATHPIN_CD_FILE set to a
# fresh temp file. If pathpin wrote a directory there (enter in the menu, or
# `pathpin <label>`), the function cds to it. The names are prefixed and local
# so they can't clobber the user's own variables.
__PATHPIN_CMD__() {
    local __pathpin_tmp __pathpin_dir __pathpin_ret
    __pathpin_tmp="$(mktemp "${TMPDIR:-/tmp}/pathpin.XXXXXX")" || return
    # The VAR=value prefix sets both variables for this one command only.
    # `command` skips shell functions, so a function named pathpin runs the
    # binary instead of calling itself forever.
    PATHPIN_CD_FILE="$__pathpin_tmp" PATHPIN_CMD=__PATHPIN_CMD__ command pathpin "$@"
    __pathpin_ret=$? # saved now: every later command overwrites $?
    __pathpin_dir="$(cat -- "$__pathpin_tmp" 2>/dev/null)"
    rm -f -- "$__pathpin_tmp"
    # Empty means pathpin chose no directory (q, esc, ls, an error).
    # `--` stops option parsing, so a path starting with - is still a path.
    if [ -n "$__pathpin_dir" ] && [ -d "$__pathpin_dir" ]; then
        cd -- "$__pathpin_dir" || return
    fi
    return "$__pathpin_ret" # pathpin's own exit status, so `pp x && ...` works
}
