# jug shell integration for fish.
# Add to ~/.config/fish/config.fish:  jug init fish | source
function jug
    set -l tmpdir /tmp
    set -q TMPDIR; and set tmpdir $TMPDIR
    set -l tmp (mktemp $tmpdir/jug.XXXXXX); or return
    JUG_CD_FILE=$tmp command jug $argv
    set -l ret $status
    set -l dir (cat -- $tmp 2>/dev/null)
    rm -f -- $tmp
    if test -n "$dir"; and test -d "$dir"
        cd $dir
    end
    return $ret
end
