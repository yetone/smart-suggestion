# Smart Suggestion function for Fisher
# This makes the smart-suggestion command available globally

function smart-suggestion --description 'Show Smart Suggestion configuration'
    # Get the plugin directory
    set -l plugin_dir (dirname (status --current-filename))/..
    
    # Source the core and show config
    bash -c 'source "'$plugin_dir'/smart-suggestion-core.sh" && smart_suggestion_show_config'
end