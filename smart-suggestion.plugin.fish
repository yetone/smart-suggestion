#!/usr/bin/env fish

# Smart Suggestion Plugin for Fish
# Determine the plugin directory and core script location
set -l plugin_dir (dirname (status --current-filename))

# Try to find the core script in multiple locations (for different installation methods)
set -g core_script ""
for location in "$plugin_dir/smart-suggestion-core.sh" "$HOME/.config/smart-suggestion/smart-suggestion-core.sh"
    if test -f $location
        set -g core_script $location
        break
    end
end

if test -z "$core_script"
    echo "❌ Smart Suggestion core script not found. Please ensure the plugin is properly installed."
    return 1
end

# Set up default configurations  
if not bash -c "source '$core_script' && smart_suggestion_set_defaults"
    return 1
end

# Set fish-specific defaults
set -q SMART_SUGGESTION_KEY; or set -g SMART_SUGGESTION_KEY \ca
set -q SMART_SUGGESTION_SEND_CONTEXT; or set -g SMART_SUGGESTION_SEND_CONTEXT true
set -q SMART_SUGGESTION_DEBUG; or set -g SMART_SUGGESTION_DEBUG false
set -q SMART_SUGGESTION_PROXY_MODE; or set -g SMART_SUGGESTION_PROXY_MODE true

# Auto-detect AI provider if not set
if not set -q SMART_SUGGESTION_AI_PROVIDER
    if set -q OPENAI_API_KEY; and test -n "$OPENAI_API_KEY"
        set -g SMART_SUGGESTION_AI_PROVIDER openai
    else if set -q ANTHROPIC_API_KEY; and test -n "$ANTHROPIC_API_KEY"
        set -g SMART_SUGGESTION_AI_PROVIDER anthropic
    else if set -q GEMINI_API_KEY; and test -n "$GEMINI_API_KEY"
        set -g SMART_SUGGESTION_AI_PROVIDER gemini
    end
end

function _run_smart_suggestion_proxy
    if status is-interactive
        bash -c "source \"$core_script\" && smart_suggestion_run_proxy"
    end
end

function _fetch_suggestions
    set -l input $argv[1]
    bash -c "source \"$core_script\" && smart_suggestion_fetch \"$input\""
    return $status
end

function _show_loading_animation
    set -l pid $argv[1]
    bash -c "source \"$core_script\" && smart_suggestion_show_loading $pid"
end

function _do_smart_suggestion
    # Get current command line input
    set -l input (commandline -b)
    
    if test -z "$input"
        set input "help me with a command"
    end
    
    # Clean up temporary files
    bash -c "source \"$core_script\" && smart_suggestion_cleanup" 2>/dev/null
    
    # Call the Go binary directly  
    env SMART_SUGGESTION_KEY="$SMART_SUGGESTION_KEY" \
        SMART_SUGGESTION_SEND_CONTEXT="$SMART_SUGGESTION_SEND_CONTEXT" \
        SMART_SUGGESTION_DEBUG="$SMART_SUGGESTION_DEBUG" \
        SMART_SUGGESTION_PROXY_MODE="$SMART_SUGGESTION_PROXY_MODE" \
        SMART_SUGGESTION_AI_PROVIDER="$SMART_SUGGESTION_AI_PROVIDER" \
        bash -c "source \"$core_script\" && smart_suggestion_fetch \"$input\"" 2>/dev/null
    
    # Process the response
    set -l response (bash -c "source \"$core_script\" && smart_suggestion_process_response" 2>/dev/null)
    if test $status -ne 0
        return 1
    end

    # Parse response (split on |)
    set -l response_parts (string split '|' $response)
    if test (count $response_parts) -ge 2
        set -l first_char $response_parts[1]
        set -l suggestion $response_parts[2]

        # Handle the suggestion based on type
        if test "$first_char" = '='
            # Replace current command line
            commandline -r "$suggestion"
        else if test "$first_char" = '+'
            # Append to current command line
            commandline -i "$suggestion"
        end
    end
    
    commandline -f repaint
end

function smart-suggestion
    # Check if core script exists
    if test -z "$core_script" -o ! -f "$core_script"
        echo "❌ Smart Suggestion core script not found"
        return 1
    end
    
    # Pass fish environment variables to bash
    env SMART_SUGGESTION_KEY="$SMART_SUGGESTION_KEY" \
        SMART_SUGGESTION_SEND_CONTEXT="$SMART_SUGGESTION_SEND_CONTEXT" \
        SMART_SUGGESTION_DEBUG="$SMART_SUGGESTION_DEBUG" \
        SMART_SUGGESTION_PROXY_MODE="$SMART_SUGGESTION_PROXY_MODE" \
        SMART_SUGGESTION_AI_PROVIDER="$SMART_SUGGESTION_AI_PROVIDER" \
        bash -c "source \"$core_script\" && smart_suggestion_show_config"
end

# Set up key binding for ALL modes - works with and without vim mode
# This covers both vim mode users and regular users
bind -M insert \co '_do_smart_suggestion; commandline -f repaint'    # vim insert mode
bind -M default \co '_do_smart_suggestion; commandline -f repaint'   # normal mode & non-vim users  
bind -M visual \co '_do_smart_suggestion; commandline -f repaint'    # vim visual mode

# Also bind to the global scope as fallback for any other modes
bind \co '_do_smart_suggestion; commandline -f repaint'

# Start proxy mode if enabled and not in tmux
if test "$SMART_SUGGESTION_PROXY_MODE" = "true" -a -z "$TMUX"
    _run_smart_suggestion_proxy
end
