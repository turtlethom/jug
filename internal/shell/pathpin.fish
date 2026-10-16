# pathpin shell integration for fish.
# Add to ~/.config/fish/config.fish:  pathpin init fish | source
# Pick your own command name with --cmd:  pathpin init fish --cmd pp | source
#
# How it works: the function runs the real binary with PATHPIN_CD_FILE set to a
# fresh temp file. If pathpin wrote a directory there (enter in the menu, or
# `pathpin <label>`), the function cds to it. See posix.sh for the bash/zsh
# version, which this mirrors.
function __PATHPIN_CMD__
    # fish has no ${TMPDIR:-/tmp}, so the fallback takes two lines.
    set -l tmpdir /tmp
    set -q TMPDIR; and set tmpdir $TMPDIR
    set -l tmp (mktemp $tmpdir/pathpin.XXXXXX); or return
    # `command` runs the binary, not this function (which may be named pathpin).
    PATHPIN_CD_FILE=$tmp PATHPIN_CMD=__PATHPIN_CMD__ command pathpin $argv
    set -l ret $status # saved now: every later command overwrites $status
    set -l dir (cat -- $tmp 2>/dev/null)
    rm -f -- $tmp
    # Empty means pathpin chose no directory (q, esc, ls, an error).
    if test -n "$dir"; and test -d "$dir"
        cd $dir
    end
    return $ret # pathpin's own exit status
end
