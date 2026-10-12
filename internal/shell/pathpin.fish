# pathpin shell integration for fish.
# Add to ~/.config/fish/config.fish:  pathpin init fish | source
function pathpin
    set -l tmpdir /tmp
    set -q TMPDIR; and set tmpdir $TMPDIR
    set -l tmp (mktemp $tmpdir/pathpin.XXXXXX); or return
    PATHPIN_CD_FILE=$tmp command pathpin $argv
    set -l ret $status
    set -l dir (cat -- $tmp 2>/dev/null)
    rm -f -- $tmp
    if test -n "$dir"; and test -d "$dir"
        cd $dir
    end
    return $ret
end
