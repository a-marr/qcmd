# qcmd.zsh - Source this in your .zshrc
# Usage: q
#
# The q function opens your editor, sends your query to an LLM,
# and places the resulting command in your shell buffer ready to execute.
#
# Installation:
#   1. Copy this file to ~/.config/qcmd/qcmd.zsh (or anywhere you like)
#   2. Add to your ~/.zshrc: source ~/.config/qcmd/qcmd.zsh
#   3. Restart your shell or run: source ~/.zshrc
#
# Requirements:
#   - qcmd binary must be in your PATH
#   - $EDITOR or $VISUAL must be set (falls back to vi)

function q() {
    local query_file
    local cmd
    local exit_code

    # Create temp file for user input
    query_file=$(mktemp) || {
        echo "qcmd: failed to create temp file" >&2
        return 1
    }

    # Open editor for user input
    ${VISUAL:-${EDITOR:-vi}} "$query_file"

    # Check if user wrote anything (non-empty after removing blank lines and comments)
    if [[ ! -s "$query_file" ]] || ! grep -qv '^[[:space:]]*#' "$query_file" 2>/dev/null || ! grep -q '[^[:space:]]' "$query_file" 2>/dev/null; then
        rm -f "$query_file"
        return 0
    fi

    # Call qcmd binary with explicit ZLE output mode
    # stdout = command only, stderr = diagnostics (passed through to terminal)
    cmd=$(qcmd --query-file "$query_file" --output=zle)
    exit_code=$?

    rm -f "$query_file"

    case $exit_code in
        0)
            # Success - inject command into ZLE buffer
            # User can review and press Enter to execute
            if [[ -n "$cmd" ]]; then
                print -z "$cmd"
            fi
            ;;
        1)
            # User/input error - stderr already printed by qcmd
            return 1
            ;;
        2)
            # API/system error - stderr already printed by qcmd
            return 2
            ;;
        3)
            # Dangerous command - print but don't inject
            echo "" >&2
            echo "Command blocked from injection (safety check triggered)" >&2
            echo "Review the command below. Copy manually if intended:" >&2
            echo "" >&2
            echo "$cmd"
            echo "" >&2
            return 3
            ;;
        *)
            echo "qcmd: unexpected exit code $exit_code" >&2
            return $exit_code
            ;;
    esac
}

# Optional: ZLE widget for direct keybind (uncomment to enable)
# This allows triggering qcmd with a key combo instead of typing 'q'
#
# function _qcmd_widget() {
#     zle -I  # Invalidate display
#     q       # Call the q function
#     zle reset-prompt
# }
# zle -N _qcmd_widget
# bindkey '^Q' _qcmd_widget  # Ctrl+Q (change as desired)
