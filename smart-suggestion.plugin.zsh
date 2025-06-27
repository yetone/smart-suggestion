#!/usr/bin/env zsh

# Smart Suggestion Plugin for Zsh
# Source the shared core functions
source "${0:A:h}/smart-suggestion-core.sh"

# Set up default configurations
smart_suggestion_set_defaults || return 1

# Zsh-specific key binding default
(( ! ${+SMART_SUGGESTION_KEY} )) &&
    typeset -g SMART_SUGGESTION_KEY='^o'

function _run_smart_suggestion_proxy() {
    if [[ $- == *i* ]]; then
        smart_suggestion_run_proxy
    fi
}

function _fetch_suggestions() {
    smart_suggestion_fetch "$input"
    return $?
}

function _show_loading_animation() {
    smart_suggestion_show_loading $1
}

function _do_smart_suggestion() {
    # Clean up and get input
    smart_suggestion_cleanup
    local input=$(echo "${BUFFER:0:$CURSOR}" | tr '\n' ';')

    _zsh_autosuggest_clear

    # Fetch message in background
    read < <(_fetch_suggestions & echo $!)
    local pid=$REPLY

    _show_loading_animation $pid
    local response_code=$?

    # Log debug info
    smart_suggestion_log_debug "$input" "$response_code"

    # Process the response
    local response=$(smart_suggestion_process_response)
    if [[ $? -ne 0 ]]; then
        _zsh_autosuggest_clear
        echo "$response"
        return 1
    fi

    # Parse response
    local first_char=${response%%|*}
    local suggestion=${response#*|}

    # Handle the suggestion based on type
    if [[ "$first_char" == '=' ]]; then
        # Reset user input
        BUFFER=""
        CURSOR=0
        zle -U "$suggestion"
    elif [[ "$first_char" == '+' ]]; then
        _zsh_autosuggest_suggest "$suggestion"
    fi
}

function smart-suggestion() {
    smart_suggestion_show_config
}

# Set up key binding
zle -N _do_smart_suggestion
bindkey "$SMART_SUGGESTION_KEY" _do_smart_suggestion

# Start proxy mode if enabled and not in tmux
if [[ "$SMART_SUGGESTION_PROXY_MODE" == "true" && -z "$TMUX" ]]; then
    _run_smart_suggestion_proxy
fi
