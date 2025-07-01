#!/usr/bin/env zsh

# Default key binding
(( ! ${+SMART_SUGGESTION_KEY} )) &&
    typeset -g SMART_SUGGESTION_KEY='^o'

# Configuration options
(( ! ${+SMART_SUGGESTION_SEND_CONTEXT} )) &&
    typeset -g SMART_SUGGESTION_SEND_CONTEXT=true

(( ! ${+SMART_SUGGESTION_DEBUG} )) &&
    typeset -g SMART_SUGGESTION_DEBUG=false

# Proxy mode configuration - now enabled by default
(( ! ${+SMART_SUGGESTION_PROXY_MODE} )) &&
    typeset -g SMART_SUGGESTION_PROXY_MODE=true

# New option to select AI provider
if [[ -z "$SMART_SUGGESTION_AI_PROVIDER" ]]; then
    if [[ -n "$OPENAI_API_KEY" ]]; then
        typeset -g SMART_SUGGESTION_AI_PROVIDER="openai"
    elif [[ -n "$AZURE_OPENAI_API_KEY" && -n "$AZURE_OPENAI_RESOURCE_NAME" && -n "$AZURE_OPENAI_DEPLOYMENT_NAME" ]]; then
        typeset -g SMART_SUGGESTION_AI_PROVIDER="azure_openai"
    elif [[ -n "$ANTHROPIC_API_KEY" ]]; then
        typeset -g SMART_SUGGESTION_AI_PROVIDER="anthropic"
    elif [[ -n "$GEMINI_API_KEY" ]]; then
        typeset -g SMART_SUGGESTION_AI_PROVIDER="gemini"
    elif [[ -n "$DEEPSEEK_API_KEY" ]]; then
        typeset -g SMART_SUGGESTION_AI_PROVIDER="deepseek"
    else
        echo "No AI provider selected. Please set either OPENAI_API_KEY, AZURE_OPENAI_API_KEY (with AZURE_OPENAI_RESOURCE_NAME and AZURE_OPENAI_DEPLOYMENT_NAME), ANTHROPIC_API_KEY, GEMINI_API_KEY, or DEEPSEEK_API_KEY."
        return 1
    fi
fi

if [[ "$SMART_SUGGESTION_DEBUG" == 'true' ]]; then
    touch /tmp/smart-suggestion.log
fi

function _run_smart_suggestion_proxy() {
    if [[ $- == *i* ]]; then
        local binary_path="$HOME/.config/smart-suggestion/smart-suggestion"
        if [[ ! -f "$binary_path" ]]; then
            echo "smart-suggestion binary not found at $binary_path. Please run ./build.sh to build it."
            return 1
        fi
        "$binary_path" proxy
    fi
}

function _fetch_suggestions() {
    local binary_path="$HOME/.config/smart-suggestion/smart-suggestion"
    
    # Check if the binary exists
    if [[ ! -f "$binary_path" ]]; then
        echo "smart-suggestion binary not found at $binary_path. Please run ./build.sh to build it." > /tmp/.smart_suggestion_error
        return 1
    fi
    
    # Prepare debug flag
    local debug_flag=""
    if [[ "$SMART_SUGGESTION_DEBUG" == 'true' ]]; then
        debug_flag="--debug"
    fi
    
    # Prepare context flag
    local context_flag=""
    if [[ "$SMART_SUGGESTION_SEND_CONTEXT" == 'true' ]]; then
        context_flag="--context"
    fi
    
    # Call the Go binary with proper arguments
    "$binary_path" \
        --provider "$SMART_SUGGESTION_AI_PROVIDER" \
        --input "$input" \
        --output "/tmp/smart_suggestion" \
        $debug_flag \
        $context_flag
    
    return $?
}


function _show_loading_animation() {
    local pid=$1
    local interval=0.1
    local animation_chars=("⠋" "⠙" "⠹" "⠸" "⠼" "⠴" "⠦" "⠧" "⠇" "⠏")
    local i=1

    cleanup() {
      kill $pid
      echo -ne "\e[?25h"
    }
    trap cleanup SIGINT
    
    while kill -0 $pid 2>/dev/null; do
        # Display current animation frame
        zle -R "${animation_chars[i]}"

        # Update index, make sure it starts at 1
        i=$(( (i + 1) % ${#animation_chars[@]} ))

        if [[ $i -eq 0 ]]; then
            i=1
        fi
        
        sleep $interval
    done

    echo -ne "\e[?25h"
    trap - SIGINT
}

function _do_smart_suggestion() {
    ##### Get input
    rm -f /tmp/smart_suggestion
    rm -f /tmp/.smart_suggestion_error
    local input=$(echo "${BUFFER:0:$CURSOR}" | tr '\n' ';')

    _zsh_autosuggest_clear

    ##### Fetch message
    read < <(_fetch_suggestions & echo $!)
    local pid=$REPLY

    _show_loading_animation $pid
    local response_code=$?

    if [[ "$SMART_SUGGESTION_DEBUG" == 'true' ]]; then
        echo "{\"date\":\"$(date)\",\"log\":\"Fetched message\",\"input\":\"$input\",\"response_code\":\"$response_code\"}" >> /tmp/smart-suggestion.log
    fi

    if [[ ! -f /tmp/smart_suggestion ]]; then
        _zsh_autosuggest_clear
        echo $(cat /tmp/.smart_suggestion_error 2>/dev/null || echo "No suggestion available at this time. Please try again later.")
        return 1
    fi

    local message=$(cat /tmp/smart_suggestion)

    ##### Process response

    local first_char=${message:0:1}
    local suggestion=${message:1:${#message}}

    ##### And now, let's actually show the suggestion to the user!

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
    echo "Smart Suggestion is now active. Press $SMART_SUGGESTION_KEY to get suggestions."
    echo ""
    echo "Configurations:"
    echo "    - SMART_SUGGESTION_KEY: Key to press to get suggestions (default: ^o, value: $SMART_SUGGESTION_KEY)."
    echo "    - SMART_SUGGESTION_SEND_CONTEXT: If \`true\`, smart-suggestion will send context information (whoami, shell, pwd, etc.) to the AI model (default: true, value: $SMART_SUGGESTION_SEND_CONTEXT)."
    echo "    - SMART_SUGGESTION_AI_PROVIDER: AI provider to use ('openai', 'azure_openai', 'anthropic', 'gemini', or 'deepseek', value: $SMART_SUGGESTION_AI_PROVIDER)."
    echo "    - SMART_SUGGESTION_DEBUG: Enable debug logging (default: false, value: $SMART_SUGGESTION_DEBUG)."
}

zle -N _do_smart_suggestion
bindkey "$SMART_SUGGESTION_KEY" _do_smart_suggestion

if [[ "$SMART_SUGGESTION_PROXY_MODE" == "true" && -z "$TMUX" ]]; then
    _run_smart_suggestion_proxy
fi
