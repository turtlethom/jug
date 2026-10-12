# pathpin shell integration for fish.
# Add to ~/.config/fish/config.fish:  pathpin init fish | source
# Pick your own command name with --cmd:  pathpin init fish --cmd pp | source
function __PATHPIN_CMD__
    set -l tmpdir /tmp
    set -q TMPDIR; and set tmpdir $TMPDIR
    set -l tmp (mktemp $tmpdir/pathpin.XXXXXX); or return
    PATHPIN_CD_FILE=$tmp PATHPIN_CMD=__PATHPIN_CMD__ command pathpin $argv
    set -l ret $status
    set -l dir (cat -- $tmp 2>/dev/null)
    rm -f -- $tmp
    if test -n "$dir"; and test -d "$dir"
        cd $dir
    end
    return $ret
end
