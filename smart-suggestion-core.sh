#!/bin/bash

# Smart Suggestion Core Functions
# Shared functionality between zsh and fish plugins

# Set default configurations if not already set
smart_suggestion_set_defaults() {
    # Default key binding - shell-specific plugins will handle this
    : ${SMART_SUGGESTION_KEY:='^o'}
    
    # Configuration options
    : ${SMART_SUGGESTION_SEND_CONTEXT:=true}
    : ${SMART_SUGGESTION_DEBUG:=false}
    : ${SMART_SUGGESTION_PROXY_MODE:=true}
    
    # Auto-detect AI provider if not set
    if [[ -z "$SMART_SUGGESTION_AI_PROVIDER" ]]; then
        if [[ -n "$OPENAI_API_KEY" ]]; then
            export SMART_SUGGESTION_AI_PROVIDER="openai"
        elif [[ -n "$AZURE_OPENAI_API_KEY" && -n "$AZURE_OPENAI_RESOURCE_NAME" && -n "$AZURE_OPENAI_DEPLOYMENT_NAME" ]]; then
            typeset -g SMART_SUGGESTION_AI_PROVIDER="azure_openai"
        elif [[ -n "$ANTHROPIC_API_KEY" ]]; then
            export SMART_SUGGESTION_AI_PROVIDER="anthropic"
        elif [[ -n "$GEMINI_API_KEY" ]]; then
            export SMART_SUGGESTION_AI_PROVIDER="gemini"
        else
            echo "No AI provider selected. Please set either OPENAI_API_KEY or AZURE_OPENAI_API_KEY (with AZURE_OPENAI_RESOURCE_NAME and AZURE_OPENAI_DEPLOYMENT_NAME) or ANTHROPIC_API_KEY or GEMINI_API_KEY."
            return 1
        fi
    fi
    
    # Create debug log if needed
    if [[ "$SMART_SUGGESTION_DEBUG" == 'true' ]]; then
        touch /tmp/smart-suggestion.log
    fi
    
    return 0
}

# Get the binary path
smart_suggestion_get_binary_path() {
    # Try multiple locations for the binary
    local locations=(
        "$HOME/.config/smart-suggestion/smart-suggestion"
        "$(dirname "${BASH_SOURCE[0]}")/smart-suggestion"
        "./smart-suggestion"
    )
    
    for location in "${locations[@]}"; do
        if [[ -f "$location" ]]; then
            echo "$location"
            return 0
        fi
    done
    
    # Default to the config location
    echo "$HOME/.config/smart-suggestion/smart-suggestion"
}

# Check if binary exists
smart_suggestion_check_binary() {
    local binary_path=$(smart_suggestion_get_binary_path)
    if [[ ! -f "$binary_path" ]]; then
        echo "smart-suggestion binary not found at $binary_path. Please run ./build.sh to build it."
        return 1
    fi
    return 0
}

# Run the proxy mode
smart_suggestion_run_proxy() {
    local binary_path=$(smart_suggestion_get_binary_path)
    if ! smart_suggestion_check_binary; then
        return 1
    fi
    "$binary_path" proxy
}

# Fetch suggestions from AI
smart_suggestion_fetch() {
    local input="$1"
    local binary_path=$(smart_suggestion_get_binary_path)
    
    # Check if the binary exists
    if ! smart_suggestion_check_binary; then
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

# Show loading animation
smart_suggestion_show_loading() {
    local pid=$1
    local interval=0.1
    local animation_chars=("⠋" "⠙" "⠹" "⠸" "⠼" "⠴" "⠦" "⠧" "⠇" "⠏")
    local i=0

    cleanup() {
        kill $pid 2>/dev/null
        printf "\e[?25h"
    }
    trap cleanup SIGINT
    
    while kill -0 $pid 2>/dev/null; do
        # Display current animation frame
        printf "%s\r" "${animation_chars[i]}"

        # Update index
        i=$(( (i + 1) % ${#animation_chars[@]} ))
        
        sleep $interval
    done

    printf "\e[?25h"
    trap - SIGINT
}

# Process the suggestion response
smart_suggestion_process_response() {
    if [[ ! -f /tmp/smart_suggestion ]]; then
        cat /tmp/.smart_suggestion_error 2>/dev/null || echo "No suggestion available at this time. Please try again later."
        return 1
    fi

    local message=$(cat /tmp/smart_suggestion)
    local first_char=${message:0:1}
    local suggestion=${message:1:${#message}}
    
    # Return the type and suggestion for shell-specific handling
    echo "$first_char|$suggestion"
    return 0
}

# Log debug information
smart_suggestion_log_debug() {
    local input="$1"
    local response_code="$2"
    
    if [[ "$SMART_SUGGESTION_DEBUG" == 'true' ]]; then
        echo "{\"date\":\"$(date)\",\"log\":\"Fetched message\",\"input\":\"$input\",\"response_code\":\"$response_code\"}" >> /tmp/smart-suggestion.log
    fi
}

# Show configuration information
smart_suggestion_show_config() {
    echo "Smart Suggestion is now active. Press $SMART_SUGGESTION_KEY to get suggestions."
    echo ""
    echo "Configurations:"
    echo "    - SMART_SUGGESTION_KEY: Key to press to get suggestions (default: ^o, value: $SMART_SUGGESTION_KEY)."
    echo "    - SMART_SUGGESTION_SEND_CONTEXT: If \`true\`, smart-suggestion will send context information (whoami, shell, pwd, etc.) to the AI model (default: true, value: $SMART_SUGGESTION_SEND_CONTEXT)."
    echo "    - SMART_SUGGESTION_AI_PROVIDER: AI provider to use ('openai', 'azure_openai', 'anthropic', or 'gemini', value: $SMART_SUGGESTION_AI_PROVIDER)."
    echo "    - SMART_SUGGESTION_DEBUG: Enable debug logging (default: false, value: $SMART_SUGGESTION_DEBUG)."
}

# Clean up temporary files
smart_suggestion_cleanup() {
    rm -f /tmp/smart_suggestion
    rm -f /tmp/.smart_suggestion_error
}
