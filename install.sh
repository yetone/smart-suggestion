#!/bin/bash

# Smart Suggestion Installer
# This script automatically installs smart-suggestion for zsh

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Configuration
REPO_URL="https://github.com/yetone/smart-suggestion"
LATEST_RELEASE_URL="https://api.github.com/repos/yetone/smart-suggestion/releases/latest"
INSTALL_DIR="$HOME/.config/smart-suggestion"
PLUGIN_FILE="smart-suggestion.plugin.zsh"

# Helper functions
log_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

log_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

log_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# Check if command exists
command_exists() {
    command -v "$1" >/dev/null 2>&1
}

is_omz() {
    [[ -d "$HOME/.oh-my-zsh" ]]
}

if is_omz; then
    log_info "Oh My Zsh installation detected."
    INSTALL_DIR="${ZSH_CUSTOM:-$HOME/.oh-my-zsh/custom}/plugins/smart-suggestion"
fi

# Detect OS and architecture
detect_platform() {
    local os=""
    local arch=""

    # Detect OS
    case "$(uname -s)" in
        Linux*)     os="linux";;
        Darwin*)    os="darwin";;
        MINGW*|MSYS*|CYGWIN*) os="windows";;
        *)          log_error "Unsupported OS: $(uname -s)"; exit 1;;
    esac

    # Detect architecture
    case "$(uname -m)" in
        x86_64|amd64)   arch="amd64";;
        arm64|aarch64)  arch="arm64";;
        *)              log_error "Unsupported architecture: $(uname -m)"; exit 1;;
    esac

    echo "${os}-${arch}"
}

# Check prerequisites
check_prerequisites() {
    log_info "Checking prerequisites..."

    # Check if zsh is available
    if ! command_exists zsh; then
        log_error "zsh is not installed. Please install zsh first."
        exit 1
    fi

    # Check if curl or wget is available
    if ! command_exists curl && ! command_exists wget; then
        log_error "Either curl or wget is required for installation."
        exit 1
    fi

    # Check if tar is available
    if ! command_exists tar; then
        log_error "tar is required for installation."
        exit 1
    fi

    log_success "Prerequisites check passed!"
}

# Download file using curl or wget
download_file() {
    local url="$1"
    local output="$2"

    # Clean the URL of any potential issues
    url=$(echo "$url" | tr -d '\r\n' | sed 's/[[:space:]]*$//')

    if command_exists curl; then
        curl -fsSL "$url" -o "$output"
    elif command_exists wget; then
        wget -q "$url" -O "$output"
    else
        log_error "No download tool available"
        exit 1
    fi
}

# Get latest release info
get_latest_release() {
    local temp_file="${TMPDIR:-/tmp}/smart_suggestion_release.json"
    local platform="$1"  # Pass platform as parameter to avoid calling detect_platform inside

    # Download release info quietly
    download_file "$LATEST_RELEASE_URL" "$temp_file" || { log_error "Failed to download release info"; return 1; }

    # Extract tag name and download URL for our platform
    local tag_name=""
    local download_url=""

    if command_exists jq; then
        tag_name=$(jq -r '.tag_name' "$temp_file" 2>/dev/null)
        download_url=$(jq -r ".assets[] | select(.name | contains(\"$platform\")) | .browser_download_url" "$temp_file" 2>/dev/null)
    else
        # Fallback parsing without jq - improved regex
        tag_name=$(grep -o '"tag_name"[[:space:]]*:[[:space:]]*"[^"]*"' "$temp_file" | head -1 | sed 's/.*"tag_name"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/' 2>/dev/null)
        download_url=$(grep -o '"browser_download_url"[[:space:]]*:[[:space:]]*"[^"]*smart-suggestion-'$platform'[^"]*"' "$temp_file" | head -1 | sed 's/.*"browser_download_url"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/' 2>/dev/null)
    fi

    # Clean the URLs of any potential whitespace or control characters
    tag_name=$(echo "$tag_name" | tr -d '\r\n' | sed 's/^[[:space:]]*//' | sed 's/[[:space:]]*$//')
    download_url=$(echo "$download_url" | tr -d '\r\n' | sed 's/^[[:space:]]*//' | sed 's/[[:space:]]*$//')

    # Debug output
    if [[ -n "$DEBUG" ]]; then
        log_info "Debug: platform=$platform" >&2
        log_info "Debug: tag_name=$tag_name" >&2
        log_info "Debug: download_url=$download_url" >&2
    fi

    if [[ -z "$tag_name" || -z "$download_url" ]]; then
        log_error "Could not find release for platform: $platform" >&2
        log_info "Available releases can be found at: $REPO_URL/releases" >&2

        # Show available assets for debugging
        if command_exists jq; then
            log_info "Available assets:" >&2
            jq -r '.assets[].name' "$temp_file" 2>/dev/null | while read asset; do
                log_info "  - $asset" >&2
            done
        else
            log_info "Available assets (partial list):" >&2
            grep -o '"name"[[:space:]]*:[[:space:]]*"[^"]*"' "$temp_file" | sed 's/.*"name"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/' | head -10 | while read asset; do
                log_info "  - $asset" >&2
            done
        fi

        rm -f "$temp_file"
        exit 1
    fi

    # Only output the result to stdout
    echo "$tag_name|$download_url"
    rm -f "$temp_file"
}

# Install smart-suggestion
install_smart_suggestion() {
    local platform=$(detect_platform)
    log_info "Installing smart-suggestion for $platform..."

    # Get release info and parse it carefully
    local release_info
    release_info=$(get_latest_release "$platform")

    if [[ -z "$release_info" || "$release_info" != *"|"* ]]; then
        log_error "Failed to get release information"
        exit 1
    fi

    local tag_name=$(echo "$release_info" | cut -d'|' -f1)
    local download_url=$(echo "$release_info" | cut -d'|' -f2)

    # Additional cleaning of the variables
    tag_name=$(echo "$tag_name" | tr -d '\r\n' | sed 's/^[[:space:]]*//' | sed 's/[[:space:]]*$//')
    download_url=$(echo "$download_url" | tr -d '\r\n' | sed 's/^[[:space:]]*//' | sed 's/[[:space:]]*$//')

    if [[ -z "$tag_name" || -z "$download_url" ]]; then
        log_error "Failed to parse release information: tag_name='$tag_name', download_url='$download_url'"
        exit 1
    fi

    log_info "Found release $tag_name"

    # Create install directory
    mkdir -p "$INSTALL_DIR"

    # Download and extract
    local temp_archive="${TMPDIR:-/tmp}/smart-suggestion-$platform.tar.gz"
    log_info "Downloading from: $download_url"
    download_file "$download_url" "$temp_archive"

    # Extract to install directory
    log_info "Extracting to $INSTALL_DIR..."
    tar -xzf "$temp_archive" -C "$INSTALL_DIR" --strip-components=1

    # Make binary executable
    chmod +x "$INSTALL_DIR/smart-suggestion"

    # Clean up
    rm -f "$temp_archive"

    log_success "Smart Suggestion $tag_name installed successfully!"
}

# Check if zsh-autosuggestions is installed
check_zsh_autosuggestions() {
    log_info "Checking for zsh-autosuggestions..."

    # Common installation paths for zsh-autosuggestions
    local autosuggestions_paths=(
        "$HOME/.oh-my-zsh/custom/plugins/zsh-autosuggestions"
        "$HOME/.oh-my-zsh/plugins/zsh-autosuggestions"
        "/usr/share/zsh-autosuggestions"
        "/usr/local/share/zsh-autosuggestions"
        "$HOME/.zsh/zsh-autosuggestions"
    )

    local found=false
    for path in "${autosuggestions_paths[@]}"; do
        if [[ -f "$path/zsh-autosuggestions.zsh" ]]; then
            found=true
            break
        fi
    done

    if [[ "$found" == "false" ]]; then
        log_warning "zsh-autosuggestions not found in common locations."
        log_warning "Please ensure zsh-autosuggestions is installed and sourced in your .zshrc"
        log_info "Install it with: git clone https://github.com/zsh-users/zsh-autosuggestions ~/.zsh/zsh-autosuggestions"
    else
        log_success "zsh-autosuggestions found!"
    fi
}

# Setup zshrc configuration
setup_zshrc() {
    local zshrc_file="$HOME/.zshrc"
    local source_line="source $INSTALL_DIR/$PLUGIN_FILE # smart-suggestion"

    log_info "Setting up zsh configuration..."

    # Check if already configured
    if [[ -f "$zshrc_file" ]] && grep -q "smart-suggestion" "$zshrc_file"; then
        log_warning "Smart Suggestion appears to already be configured in $zshrc_file"
        return
    fi

    if is_omz; then
        echo "Please enable the smart-suggestion plugin with Oh My Zsh:"
        echo -e "   ${YELLOW}omz plugin enable smart-suggestion${NC}"
        return
    fi
    # Backup existing .zshrc
    if [[ -f "$zshrc_file" ]]; then
        cp "$zshrc_file" "$zshrc_file.backup.$(date +%Y%m%d_%H%M%S)"
        log_info "Backed up existing .zshrc"
    fi

    # Add source line to .zshrc
    echo "" >> "$zshrc_file"
    echo "# Smart Suggestion # smart-suggestion" >> "$zshrc_file"
    echo "$source_line" >> "$zshrc_file"

    log_success "Added smart-suggestion to $zshrc_file with proxy mode enabled by default"
}

# Display post-installation instructions
show_post_install_instructions() {
    echo ""
    log_success "Installation completed successfully!"
    echo ""
    echo -e "${BLUE}Next steps:${NC}"
    echo "1. Set up your AI provider API key:"
    echo -e "   ${YELLOW}export OPENAI_API_KEY=\"your-api-key\"${NC}                        # For OpenAI"
    echo -e "   ${YELLOW}export AZURE_OPENAI_API_KEY=\"your-api-key\"${NC}                  # For Azure OpenAI"
    echo -e "   ${YELLOW}export AZURE_OPENAI_RESOURCE_NAME=\"your-resource-name\"${NC}      # For Azure OpenAI"
    echo -e "   ${YELLOW}export AZURE_OPENAI_DEPLOYMENT_NAME=\"your-deployment-name\"${NC}  # For Azure OpenAI"
    echo -e "   ${YELLOW}export ANTHROPIC_API_KEY=\"your-api-key\"${NC}                     # For Anthropic"
    echo -e "   ${YELLOW}export GEMINI_API_KEY=\"your-api-key\"${NC}                        # For Google Gemini"
    echo -e "   ${YELLOW}export DEEPSEEK_API_KEY=\"your-api-key\"${NC}                      # For DeepSeek"
    echo ""
    echo "2. Reload your shell:"
    echo -e "   ${YELLOW}source ~/.zshrc${NC}"
    echo ""
    echo "3. Test the installation:"
    echo -e "   ${YELLOW}$INSTALL_DIR/smart-suggestion${NC}"
    echo ""
    echo "4. Use smart suggestions:"
    echo "   - Type a command or describe what you want to do"
    echo -e "   - Press ${YELLOW}Ctrl+O${NC} to get AI suggestions"
    echo ""
    echo -e "${BLUE}Configuration:${NC}"
    echo -e "- Installation directory: ${YELLOW}$INSTALL_DIR${NC}"
    echo -e "- Plugin file: ${YELLOW}$INSTALL_DIR/$PLUGIN_FILE${NC}"
    echo -e "- Documentation: ${YELLOW}$INSTALL_DIR/README.md${NC}"
    echo ""
    echo -e "${BLUE}For more information:${NC}"
    echo "- Repository: $REPO_URL"
    echo "- Documentation: $REPO_URL#readme"
}

# Main installation function
main() {
    echo -e "${GREEN}Smart Suggestion Installer${NC}"
    echo "================================"
    echo ""

    # Check prerequisites
    check_prerequisites

    # Install smart-suggestion
    install_smart_suggestion

    # Check for zsh-autosuggestions
    check_zsh_autosuggestions

    # Setup zshrc
    setup_zshrc

    # Show post-installation instructions
    show_post_install_instructions
}

# Handle command line arguments
case "${1:-}" in
    --help|-h)
        echo "Smart Suggestion Installer"
        echo ""
        echo "Usage: $0 [options]"
        echo ""
        echo "Options:"
        echo "  --help, -h     Show this help message"
        echo "  --uninstall    Uninstall smart-suggestion"
        echo ""
        echo "Environment variables:"
        echo "  INSTALL_DIR    Installation directory (default: $INSTALL_DIR)"
        exit 0
        ;;
    --uninstall)
        log_info "Uninstalling smart-suggestion..."

        # Remove installation directory
        if [[ -d "$INSTALL_DIR" ]]; then
            rm -rf "$INSTALL_DIR"
            log_success "Removed installation directory: $INSTALL_DIR"
        fi

        # Remove from .zshrc
        if [[ -f "$HOME/.zshrc" ]]; then
            # Create backup
            cp "$HOME/.zshrc" "$HOME/.zshrc.backup.$(date +%Y%m%d_%H%M%S)"

            # Remove smart-suggestion lines
            grep -v "smart-suggestion" "$HOME/.zshrc" > "$HOME/.zshrc.tmp" && mv "$HOME/.zshrc.tmp" "$HOME/.zshrc"
            log_success "Removed smart-suggestion from .zshrc"
        fi

        log_success "Smart Suggestion uninstalled successfully!"
        echo "Please restart your shell or run: source ~/.zshrc"
        exit 0
        ;;
    "")
        # Default installation
        main
        ;;
    *)
        log_error "Unknown option: $1"
        echo "Use --help for usage information"
        exit 1
        ;;
esac
