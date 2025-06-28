# Smart Suggestion for Zsh & Fish

> [!NOTE]
>
> This project is a fork of [zsh-copilot](https://github.com/Myzel394/zsh-copilot) by [Myzel394](https://github.com/Myzel394), with added support for fish shell.

Get AI-powered command suggestions **directly** in your zsh or fish shell. No complex setup, no external tools - just press `CTRL + O` and get intelligent command suggestions powered by OpenAI, Anthropic Claude, or Google Gemini.

> [!NOTE]
>
> This project is still in its early stages, and some features may be immature and unstable. I appreciate your understanding.

<https://github.com/user-attachments/assets/90eaf300-c891-4ef9-958d-9890f53f9929>

## Features

- **🚀 Context-aware intelligent prediction**: Predicts the next command you are likely to input based on context (history, aliases, terminal buffer)
- **🤖 Multiple AI Providers**: Support for OpenAI GPT, Anthropic Claude, and Google Gemini
- **🔧 Highly Configurable**: Customize keybindings, AI provider, context sharing, and more

## Questions
* Why don't I use [zsh-copilot](https://github.com/Myzel394/zsh-copilot) and instead fork a separate version?

    Because the context of zsh-copilot only includes history commands and does not include the terminal buffer (i.e., the stdout/stderr of history commands), it cannot achieve the context-aware intelligent prediction I want, this is the feature I want the most, and it's also the main reason why I forked. Additionally, since zsh-copilot is written in shell, it's very difficult to concatenate JSON and implement stdio interception. Therefore, I re-implemented almost all logic using Go, which made it too different from the original project to merge back.

## Installation

### Prerequisites

Make sure you have the following installed:

- **zsh** or **fish** shell
- For zsh: **[zsh-autosuggestions](https://github.com/zsh-users/zsh-autosuggestions)** plugin
- For fish: Built-in autosuggestions (included by default)
- An API key for one of the supported AI providers

### Method 1: Quick Install (Recommended)

The easiest way to install smart-suggestion is using our installation script:

```bash
curl -fsSL https://raw.githubusercontent.com/yetone/smart-suggestion/main/install.sh | bash
```

This script will:

- Detect your platform (Linux, macOS, Windows)
- Download the appropriate pre-built binary
- Install the plugin to `~/.config/smart-suggestion`
- Configure your `~/.zshrc` automatically with proxy mode enabled by default
- Check for zsh-autosuggestions dependency

**Uninstall:**

```bash
curl -fsSL https://raw.githubusercontent.com/yetone/smart-suggestion/main/install.sh | bash -s -- --uninstall
```

### Method 2: Fisher (for fish) - Recommended for Fish Users

Install with a single command:

```bash
fisher install yetone/smart-suggestion
```

That's it! Fisher will automatically:
- Download the plugin
- Build the Go binary (requires Go 1.21+)
- Set up the configuration
- Make the plugin available immediately

Then set up your AI provider API key:

```bash
set -Ux OPENAI_API_KEY "your-api-key"     # For OpenAI
# OR
set -Ux ANTHROPIC_API_KEY "your-api-key"  # For Anthropic
# OR
set -Ux GEMINI_API_KEY "your-api-key"     # For Google Gemini
```

Start using Smart Suggestion by pressing `Ctrl+O` in your terminal!

### Method 3: Oh My Zsh (for zsh)

1. Clone the repository into your Oh My Zsh custom plugins directory:

```bash
git clone https://github.com/yetone/smart-suggestion ${ZSH_CUSTOM:-~/.oh-my-zsh/custom}/plugins/smart-suggestion
```

2. Add `smart-suggestion` to your plugins array in `~/.zshrc`:

```bash
plugins=(
    # your other plugins...
    zsh-autosuggestions
    smart-suggestion
)
```

3. Build the Go binary:

```bash
cd ${ZSH_CUSTOM:-~/.oh-my-zsh/custom}/plugins/smart-suggestion
./build.sh
```

4. Reload your shell:

```bash
source ~/.zshrc  # for zsh
# OR
source ~/.config/fish/config.fish  # for fish
```

### Method 4: Manual Installation from Source

1. Clone the repository:

```bash
git clone https://github.com/yetone/smart-suggestion ~/.config/smart-suggestion
```

2. Build the Go binary (requires Go 1.21+):

```bash
cd ~/.config/smart-suggestion
./build.sh
```

3. Add to your `~/.zshrc`:

```bash
source ~/.config/smart-suggestion/smart-suggestion.plugin.zsh  # for zsh
# OR
source ~/.config/smart-suggestion/smart-suggestion.plugin.fish  # for fish
```

4. Reload your shell:

```bash
source ~/.zshrc  # for zsh
# OR
source ~/.config/fish/config.fish  # for fish
```

### Method 5: Manual Installation from Release

1. Download the latest release for your platform from [GitHub Releases](https://github.com/yetone/smart-suggestion/releases)

2. Extract the archive:

```bash
mkdir -p ~/.config/smart-suggestion
tar -xzf smart-suggestion-*.tar.gz -C ~/.config/smart-suggestion --strip-components=1
```

3. Add to your `~/.zshrc`:

```bash
source ~/.config/smart-suggestion/smart-suggestion.plugin.zsh  # for zsh
# OR
source ~/.config/smart-suggestion/smart-suggestion.plugin.fish  # for fish
```

4. Reload your shell:

```bash
source ~/.zshrc  # for zsh
# OR
source ~/.config/fish/config.fish  # for fish
```

## Configuration

### AI Provider Setup

You need an API key for at least one of the supported AI providers:

#### OpenAI (default)

```bash
export OPENAI_API_KEY="your-openai-api-key"
```

#### Azure OpenAI

```bash
export AZURE_OPENAI_API_KEY="your-azure-openai-api-key" # i.e. c0123456789012345678901234567890
export AZURE_OPENAI_RESOURCE_NAME="your-azure-openai-resource-name" # i.e. awesome-corp when your endpoint is https://awesome-corp.openai.azure.com
export AZURE_OPENAI_DEPLOYMENT_NAME="your-deployment-name" # i.e. gpt-4o
export AZURE_OPENAI_API_VERSION="2024-10-21"  # Optional, defaults to 2024-10-21
```

#### Anthropic Claude

```bash
export ANTHROPIC_API_KEY="your-anthropic-api-key"
```

#### Google Gemini

```bash
export GEMINI_API_KEY="your-gemini-api-key"
```

### Environment Variables

Configure the plugin behavior with these environment variables:

| Variable | Description | Default | Options |
|----------|-------------|---------|---------|
| `SMART_SUGGESTION_AI_PROVIDER` | AI provider to use | Auto-detected | `openai`, `azure_openai`, `anthropic`, `gemini` |
| `SMART_SUGGESTION_KEY` | Keybinding to trigger suggestions | `^o` (zsh), `\co` (fish) | Any shell keybinding |
| `SMART_SUGGESTION_SEND_CONTEXT` | Send shell context to AI | `true` | `true`, `false` |
| `SMART_SUGGESTION_PROXY_MODE` | Enable proxy mode for better context | `true` | `true`, `false` |
| `SMART_SUGGESTION_DEBUG` | Enable debug logging | `false` | `true`, `false` |
| `SMART_SUGGESTION_SYSTEM_PROMPT` | Custom system prompt | Built-in | Any string |

### Advanced Configuration

#### Custom API URLs

```bash
export OPENAI_API_URL="your-custom-openai-endpoint.com"
export ANTHROPIC_API_URL="your-custom-anthropic-endpoint.com"
export GEMINI_API_URL="your-custom-gemini-endpoint.com"
```

#### Custom Models

```bash
export GEMINI_MODEL="gemini-1.5-pro"  # Default: gemini-1.5-flash
```

#### History Lines for Context

```bash
export SMART_SUGGESTION_HISTORY_LINES="20"  # Default: 10
```

### View Current Configuration

To see all available configurations and their current values:

```bash
smart-suggestion
```

## Usage

1. **Start typing a command** or describe what you want to do
2. **Press `CTRL + O`** (or your configured key)
3. **Wait for the AI suggestion** (loading animation will show)
   - *Note: On first use, proxy mode will automatically start in the background to capture terminal context*
4. **The suggestion will appear** as:
   - An autosuggestion you can accept with `→` (for completions)
   - A completely new command that replaces your input (for new commands)

## How It Works

1. **Input Capture**: The plugin captures your current command line input
2. **Proxy Mode (Default)**: Automatically starts a background shell recording session to capture terminal output for better context
3. **Context Collection**: Gathers rich shell context including user info, directory, command history, aliases, and terminal buffer content via proxy mode
4. **AI Processing**: Sends the input and context to your configured AI provider
5. **Smart Response**: AI returns either a completion (`+`) or new command (`=`)
6. **Shell Integration**: The suggestion is displayed using shell-specific autosuggestions (zsh-autosuggestions for zsh, built-in for fish) or replaces your input

### Proxy Mode (New Default)

Smart Suggestion now automatically enables **proxy mode** by default, which provides significantly better context awareness by recording your terminal session. This mode:

- **Automatically starts** when you first use smart suggestions
- **Records terminal output** using the `script` command for maximum compatibility
- **Provides rich context** to the AI including command outputs and error messages
- **Works seamlessly** across different terminal environments

You can disable proxy mode if needed:

```bash
export SMART_SUGGESTION_PROXY_MODE=false
```

For advanced proxy configuration, see [PROXY_USAGE.md](PROXY_USAGE.md).

## Troubleshooting

### Debug Mode

Enable debug logging to troubleshoot issues:

```bash
export SMART_SUGGESTION_DEBUG=true
```

Debug logs are written to `/tmp/smart-suggestion.log`.

### Common Issues

1. **"Binary not found" error**: Run `./build.sh` in the plugin directory
2. **No suggestions**: Check your API key and internet connection
3. **Wrong suggestions**: Try adjusting the context settings or system prompt
4. **Key binding conflicts**: Change `SMART_SUGGESTION_KEY` to a different key

### Build Issues

If the build fails:

```bash
# Check Go installation
go version

# Clean and rebuild
rm -f smart-suggestion
./build.sh
```

## Contributing

Contributions are welcome! Please feel free to submit issues and pull requests.

## License

This project is open source. Please check the repository for license details.
