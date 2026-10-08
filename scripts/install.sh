#!/usr/bin/env bash
# ---------------------------------------------------------
# FCD INSTALL SCRIPT
# Builds the binary, installs wrappers + completions,
# updates the user's RC file, and ensures PATH is correct.
# ---------------------------------------------------------
set -e

echo "--------------------------------------------------"
echo "[*] Starting FCD installation..."
echo "--------------------------------------------------"

# ---------------------------------------------------------
# Resolve directories
# ---------------------------------------------------------
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$SCRIPT_DIR/.."
BUILD_DIR="$PROJECT_ROOT/build"

mkdir -p "$BUILD_DIR"

# ---------------------------------------------------------
# Build binary
# ---------------------------------------------------------
echo "[*] Building fcd binary..."
# -C runs the build from the module root, so this works from any directory
go build -C "$PROJECT_ROOT" -o "$BUILD_DIR/fcd" .

BIN_DIR="$HOME/.local/bin"
mkdir -p "$BIN_DIR"

cp "$BUILD_DIR/fcd" "$BIN_DIR/"
echo "[✓] Installed fcd binary to $BIN_DIR"
echo "---"

# ---------------------------------------------------------
# Install wrappers
# ---------------------------------------------------------
SHARE_DIR="$HOME/.local/share/fcd"
mkdir -p "$SHARE_DIR"

# Bash/Zsh wrapper
cat >"$SHARE_DIR/fcd.sh" <<'EOF'
fcd() {
    local output ret
    output=$("$HOME/.local/bin/fcd" "$@")
    ret=$?
    if [ $ret -ne 0 ]; then
        echo "$output"
        return $ret
    fi
    if [ -d "$output" ]; then
        cd "$output" || return
    elif [ -n "$output" ]; then
        echo "$output"
    fi
}
EOF

# Fish wrapper
cat >"$SHARE_DIR/fcd.fish" <<'EOF'
function fcd
    set output (~/.local/bin/fcd $argv)
    set ret $status
    if test $ret -ne 0
        echo $output
        return $ret
    end
    if test -d "$output"
        cd $output
    else if test -n "$output"
        echo $output
    end
end
EOF

echo "[✓] Installed shell wrappers in $SHARE_DIR"
echo "---"

# ---------------------------------------------------------
# Detect user's shell + RC file
# ---------------------------------------------------------
SHELL_NAME=$(basename "$SHELL")
case "$SHELL_NAME" in
    bash)
        RC_FILE="$HOME/.bashrc"
        [[ "$OSTYPE" == "darwin"* ]] && RC_FILE="$HOME/.bash_profile"
        ;;
    zsh)
        RC_FILE="$HOME/.zshrc"
        ;;
    fish)
        RC_FILE="$HOME/.config/fish/config.fish"
        ;;
    *)
        RC_FILE="$HOME/.profile"
        ;;
esac

echo "[*] Detected shell: $SHELL_NAME"
echo "[*] Using RC file: $RC_FILE"
echo "---"

# ---------------------------------------------------------
# Add wrapper source line to RC file
# ---------------------------------------------------------
WRAPPER_COMMENT="# fcd wrapper (generated)"
if [ "$SHELL_NAME" = "fish" ]; then
    WRAPPER_LINE="source $SHARE_DIR/fcd.fish"
else
    WRAPPER_LINE="source $SHARE_DIR/fcd.sh"
fi

if ! grep -Fxq "$WRAPPER_LINE" "$RC_FILE" 2>/dev/null; then
    echo "$WRAPPER_COMMENT" >> "$RC_FILE"
    echo "$WRAPPER_LINE" >> "$RC_FILE"
    echo "[✓] Added wrapper source line to $RC_FILE"
fi
echo "---"

# ---------------------------------------------------------
# Install completions
# ---------------------------------------------------------
echo "--------------------------------------------------"
echo "[*] Installing completions..."
echo "--------------------------------------------------"

ZSH_COMPLETIONS="$HOME/.zsh/completions"
BASH_COMPLETION_FILE="$SHARE_DIR/fcd.bash"
FISH_COMPLETION_DIR="$HOME/.config/fish/completions"

case "$SHELL_NAME" in
    bash)
        "$BIN_DIR/fcd" completion bash > "$BASH_COMPLETION_FILE"
        COMPLETION_LINE="source $BASH_COMPLETION_FILE"
        ;;
    zsh)
        mkdir -p "$ZSH_COMPLETIONS"
        "$BIN_DIR/fcd" completion zsh > "$ZSH_COMPLETIONS/_fcd"
        COMPLETION_LINE=""  # Zsh autoloads
        ;;
    fish)
        mkdir -p "$FISH_COMPLETION_DIR"
        "$BIN_DIR/fcd" completion fish > "$FISH_COMPLETION_DIR/fcd.fish"
        COMPLETION_LINE="" # fish autoloads
        ;;
esac

if [ -n "$COMPLETION_LINE" ] && ! grep -Fxq "$COMPLETION_LINE" "$RC_FILE"; then
    echo "$COMPLETION_LINE" >> "$RC_FILE"
    echo "[✓] Added completion source line to $RC_FILE"
fi

# ---------------------------------------------------------
# Zsh-only: ensure ~/.zsh/completions is in $fpath
# ---------------------------------------------------------
if [ "$SHELL_NAME" = "zsh" ]; then
    FPATH_LINE="fpath+=$ZSH_COMPLETIONS"
    if ! grep -Fxq "$FPATH_LINE" "$RC_FILE"; then
        echo "$FPATH_LINE" >> "$RC_FILE"
        echo "autoload -Uz compinit" >> "$RC_FILE"
        echo "compinit" >> "$RC_FILE"
        echo "[✓] Added Zsh completions directory + compinit reload"
    fi
fi

# ---------------------------------------------------------
# Ensure ~/.local/bin is in PATH
# ---------------------------------------------------------
if [ "$SHELL_NAME" = "fish" ]; then
    if ! echo $fish_user_paths | grep -q "$HOME/.local/bin"; then
        set -U fish_user_paths $HOME/.local/bin $fish_user_paths
        echo "[✓] Added ~/.local/bin to fish_user_paths"
    fi
else
    PATH_LINE='export PATH=$HOME/.local/bin:$PATH'
    if ! grep -Fxq "$PATH_LINE" "$RC_FILE" 2>/dev/null; then
        echo "# Add ~/.local/bin to PATH for FCD (generated)" >>"$RC_FILE"
        echo "$PATH_LINE" >>"$RC_FILE"
        echo "[✓] Added ~/.local/bin to PATH"
    fi
fi

echo "[✓] Installation complete!"
echo "---"
echo "Restart your shell or run: source $RC_FILE"
echo "Then you can use: fcd"
