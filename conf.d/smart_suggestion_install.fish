# Fisher install hook for Smart Suggestion
# This script runs when the plugin is installed via Fisher

function _smart_suggestion_install --on-event smart_suggestion_install
    echo "🚀 Installing Smart Suggestion..."
    
    # Get the plugin installation directory
    set -l plugin_dir $__fish_config_dir/fisher/github.com/(basename (status --current-filename | string replace '_install.fish' ''))
    
    # Find the actual plugin directory (Fisher might install to different paths)
    for dir in $__fish_config_dir/fisher/*smart-suggestion* ~/.local/share/fisher/*smart-suggestion*
        if test -d $dir -a -f $dir/build.sh
            set plugin_dir $dir
            break
        end
    end
    
    if not test -d $plugin_dir
        echo "❌ Could not find Smart Suggestion plugin directory"
        return 1
    end
    
    echo "📁 Plugin directory: $plugin_dir"
    
    # Check if Go is available
    if not command -q go
        echo "❌ Go is required to build Smart Suggestion binary"
        echo "   Please install Go from https://golang.org/dl/"
        return 1
    end
    
    # Build the binary
    echo "🔨 Building Smart Suggestion binary..."
    if pushd $plugin_dir >/dev/null 2>&1
        if ./build.sh
            echo "✅ Smart Suggestion binary built successfully!"
            
            # Create the target directory if it doesn't exist
            mkdir -p ~/.config/smart-suggestion
            
            # Copy files to the standard location
            cp smart-suggestion ~/.config/smart-suggestion/
            cp smart-suggestion-core.sh ~/.config/smart-suggestion/
            cp smart-suggestion.plugin.fish ~/.config/smart-suggestion/
            
            echo "✅ Smart Suggestion installed successfully!"
            echo ""
            echo "📋 Next steps:"
            echo "1. Set up your AI provider API key:"
            echo "   set -Ux OPENAI_API_KEY 'your-api-key'     # For OpenAI"
            echo "   set -Ux ANTHROPIC_API_KEY 'your-api-key'  # For Anthropic"
            echo "   set -Ux GEMINI_API_KEY 'your-api-key'     # For Google Gemini"
            echo ""
            echo "2. Use Smart Suggestion:"
            echo "   - Type a command or describe what you want to do"
            echo "   - Press Ctrl+O to get AI suggestions"
            echo ""
            echo "3. Run 'smart-suggestion' to see configuration options"
        else
            echo "❌ Failed to build Smart Suggestion binary"
            return 1
        end
        popd >/dev/null 2>&1
    else
        echo "❌ Could not change to plugin directory: $plugin_dir"
        return 1
    end
end