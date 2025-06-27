# Fisher uninstall hook for Smart Suggestion
# This script runs when the plugin is uninstalled via Fisher

function _smart_suggestion_uninstall --on-event smart_suggestion_uninstall
    echo "🗑️  Uninstalling Smart Suggestion..."
    
    # Remove the installation directory
    if test -d ~/.config/smart-suggestion
        rm -rf ~/.config/smart-suggestion
        echo "✅ Removed Smart Suggestion installation directory"
    end
    
    # Clean up any temporary files
    rm -f /tmp/smart_suggestion /tmp/.smart_suggestion_error
    
    echo "✅ Smart Suggestion uninstalled successfully!"
    echo "   You may need to restart your shell to complete the removal."
end