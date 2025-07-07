package main

import (
	"archive/tar"
	"bufio"
	"bytes"
	"compress/gzip"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/creack/pty"
	"github.com/spf13/cobra"
	"github.com/yetone/smart-suggestion/pkg"
	"golang.org/x/term"
)

// OpenAI API structures
type OpenAIMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type OpenAIRequest struct {
	Model    string          `json:"model"`
	Messages []OpenAIMessage `json:"messages"`
}

type OpenAIChoice struct {
	Message OpenAIMessage `json:"message"`
}

type OpenAIResponse struct {
	Choices []OpenAIChoice `json:"choices"`
	Error   *OpenAIError   `json:"error,omitempty"`
}

type OpenAIError struct {
	Message string `json:"message"`
	Type    string `json:"type"`
}

// Azure OpenAI uses the same structures as OpenAI but different API endpoints and authentication
// Azure OpenAI API structures (reuse OpenAI structures)
type AzureOpenAIRequest = OpenAIRequest
type AzureOpenAIResponse = OpenAIResponse
type AzureOpenAIError = OpenAIError

// DeepSeek API is OpenAI-compatible, reuse the same structures
type DeepSeekRequest = OpenAIRequest
type DeepSeekResponse = OpenAIResponse
type DeepSeekError = OpenAIError

// parseAndExtractCommand parses the raw response from the AI model,
// separating the reasoning from the command.
func parseAndExtractCommand(response string) string {
	closingTag := "</reasoning>"
	if pos := strings.LastIndex(response, closingTag); pos != -1 {
		commandPart := response[pos+len(closingTag):]
		return strings.TrimSpace(commandPart)
	}
	// Fallback for responses without reasoning tags
	return strings.TrimSpace(response)
}

// Anthropic API structures
type AnthropicMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type AnthropicRequest struct {
	Model     string             `json:"model"`
	MaxTokens int                `json:"max_tokens"`
	System    string             `json:"system"`
	Messages  []AnthropicMessage `json:"messages"`
}

type AnthropicContent struct {
	Text string `json:"text"`
	Type string `json:"type"`
}

type AnthropicResponse struct {
	Content []AnthropicContent `json:"content"`
	Type    string             `json:"type"`
	Error   *AnthropicError    `json:"error,omitempty"`
}

type AnthropicError struct {
	Message string `json:"message"`
	Type    string `json:"type"`
}

// Gemini API structures
type GeminiPart struct {
	Text string `json:"text"`
}

type GeminiContent struct {
	Parts []GeminiPart `json:"parts"`
	Role  string       `json:"role"`
}

type GeminiRequest struct {
	Contents []GeminiContent `json:"contents"`
}

type GeminiCandidate struct {
	Content GeminiContent `json:"content"`
}

type GeminiResponse struct {
	Candidates []GeminiCandidate `json:"candidates"`
	Error      *GeminiError      `json:"error,omitempty"`
}

type GeminiError struct {
	Message string `json:"message"`
	Code    int    `json:"code"`
}

type GitHubRelease struct {
	TagName string `json:"tag_name"`
	Assets  []struct {
		Name               string `json:"name"`
		BrowserDownloadURL string `json:"browser_download_url"`
	} `json:"assets"`
}

// Default system prompt
const defaultSystemPrompt = `You are a professional SRE engineer with decades of experience, proficient in all shell commands.

Your tasks:
    - First, you must reason about the user's intent in <reasoning> tags. This reasoning will not be shown to the user.
        Your reasoning process should follow these steps:
        1. What is the user's real intention behind the recent input context?
        2. Did the last few commands solve the intention? Why or why not?
        3. Based on the latest information, how can you solve the user's intention?
    - After reasoning, you will either complete the command or provide a new command that you think the user is trying to type.
    - You need to predict what command the user wants to input next based on shell history and shell buffer.

RULES for the final output (after the reasoning):
    - If you return a completely new command for the user, prefix is with an equal sign (=).
    - If you return a completion for the user's command, prefix it with a plus sign (+).
    - MAKE SURE TO ONLY INCLUDE THE REST OF THE COMPLETION!!!
    - Do not write any leading or trailing characters except if required for the completion to work.
    - Only respond with either a completion or a new command, not both.
    - Your response may only start with either a plus sign or an equal sign.
    - Your response MAY NOT start with both! This means that your response IS NOT ALLOWED to start with '+=' or '=+'.
    - Your response MAY NOT contain any newlines!
    - Do NOT add any additional text, comments, or explanations to your response.
    - Do not ask for more information, you won't receive it.
    - Your response will be run in the user's shell.
    - Make sure input is escaped correctly if needed so.
    - Your input should be able to run without any modifications to it.
    - DO NOT INTERACT WITH THE USER IN NATURAL LANGUAGE! If you do, you will be banned from the system.
    - Note that the double quote sign is escaped. Keep this in mind when you create quotes.

Example of your full response format:
<reasoning>
1. The user wants to see the logs for a pod that is in a CrashLoopBackOff state.
2. The previous command 'kubectl get pods' listed the pods and their statuses, but did not show the logs.
3. The next logical step is to use 'kubectl logs' on the failing pod to diagnose the issue.
</reasoning>
=kubectl -n my-namespace logs pod-name-aaa

Examples:
    * User input: 'list files in current directory';
      Your response:
<reasoning>
1. The user wants to list files.
2. No previous command.
3. 'ls' is the command for listing files.
</reasoning>
=ls
    * User input: 'cd /tm';
      Your response:
<reasoning>
1. The user wants to change directory to a temporary folder.
2. The user has typed '/tm' which is likely an abbreviation for '/tmp'.
3. Completing with 'p' will form '/tmp'.
</reasoning>
+p
    * Shell history: 'ls -l /tmp/smart-suggestion.log';
      Your response:
<reasoning>
1. The user just listed details of a log file. A common next step is to view the content of that file.
2. Listing the file does not show its content.
3. The 'cat' command can be used to display the file content.
</reasoning>
=cat /tmp/smart-suggestion.log
    * Shell buffer:
        # k -n my-namespace get pod
        NAME           READY   STATUS             RESTARTS         AGE
        pod-name-aaa   2/3     CrashLoopBackOff   358 (111s ago)   30h
        pod-name-bbb   2/3     CrashLoopBackOff   358 (3m8s ago)   30h
      Your response:
<reasoning>
1. The user is checking pods in a Kubernetes namespace.
2. The pods are in 'CrashLoopBackOff', indicating a problem. The user likely wants to see the logs to debug.
3. The command 'kubectl logs' will show the logs for 'pod-name-aaa'.
</reasoning>
=kubectl -n my-namespace logs pod-name-aaa
    * Shell buffer:
        # k -n my-namespace get pod
        NAME           READY   STATUS             RESTARTS         AGE
        pod-name-aaa   3/3     Running            0                30h
        pod-name-bbb   0/3     Pending            0                30h
      Your response:
<reasoning>
1. The user is checking pods. One pod is 'Pending'.
2. The 'get pod' command doesn't say why it's pending.
3. 'kubectl describe pod' will give more details about why the pod is pending.
</reasoning>
=kubectl -n my-namespace describe pod pod-name-bbb
    * Shell buffer:
        # k -n my-namespace get pod
        NAME           READY   STATUS             RESTARTS         AGE
        pod-name-aaa   3/3     Running            0                30h
        pod-name-bbb   0/3     Pending            0                30h
	  User input: 'k -n'
      Your response:
<reasoning>
1. The user is checking pods. One pod is 'Pending'. They started typing a command.
2. 'get pod' was useful but now they want to investigate 'pod-name-bbb'.
3. I will complete the command to describe the pending pod.
</reasoning>
+ my-namespace describe pod pod-name-bbb
    * Shell buffer:
        # k get node
        NAME      STATUS   ROLES    AGE   VERSION
        node-aaa  Ready    <none>   3h    v1.25.3
        node-bbb  NotReady <none>   3h    v1.25.3
      Your response:
<reasoning>
1. The user is checking Kubernetes nodes. One node is 'NotReady'.
2. 'get node' does not show the reason for the 'NotReady' status.
3. 'kubectl describe node' will provide detailed events and information about the node's status.
</reasoning>
=kubectl describe node node-bbb`

var (
	// These will be set during build time using ldflags
	Version   = "dev"
	BuildTime = "unknown"
	GitCommit = "unknown"
	OS        = "unknown"
	Arch      = "unknown"
)

var (
	provider     string
	input        string
	systemPrompt string
	debug        bool
	outputFile   string
	sendContext  bool
	proxyMode    bool
	proxyLogFile string
	sessionID    string

	// Global log rotator instance
	logRotator *pkg.LogRotator
)

// Initialize log rotator
func init() {
	config := pkg.DefaultLogRotateConfig()
	// Set smaller max size for more frequent rotation testing
	config.MaxSize = 5 * 1024 * 1024 // 5MB
	config.MaxBackups = 3
	config.Compress = true
	config.MaxAge = 7 // 7 days

	logRotator = pkg.NewLogRotator(config)
}

func main() {
	var rootCmd = &cobra.Command{
		Use:   "smart-suggestion",
		Short: "AI-powered smart suggestions for shell commands",
		Run:   runFetch,
	}

	// Add proxy command
	var proxyCmd = &cobra.Command{
		Use:   "proxy",
		Short: "Start shell proxy mode to record commands and output",
		Run:   runProxy,
	}

	// Add rotate-logs command
	var rotateCmd = &cobra.Command{
		Use:   "rotate-logs",
		Short: "Rotate log files to prevent them from growing too large",
		Run:   runRotateLogs,
	}

	// Add version command
	var versionCmd = &cobra.Command{
		Use:   "version",
		Short: "Show version information",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("Smart Suggestion %s\n", Version)
			fmt.Printf("Build Time: %s\n", BuildTime)
			fmt.Printf("Git Commit: %s\n", GitCommit)
			fmt.Printf("OS: %s\n", OS)
			fmt.Printf("Arch: %s\n", Arch)
		},
	}

	// Add update command
	var updateCmd = &cobra.Command{
		Use:   "update",
		Short: "Update smart-suggestion to the latest version",
		Run:   runUpdate,
	}

	// Root command flags
	rootCmd.Flags().StringVarP(&provider, "provider", "p", "", "AI provider (openai, azure_openai, anthropic, gemini, or deepseek)")
	rootCmd.Flags().StringVarP(&input, "input", "i", "", "User input")
	rootCmd.Flags().StringVarP(&systemPrompt, "system", "s", "", "System prompt (optional, uses default if not provided)")
	rootCmd.Flags().BoolVarP(&debug, "debug", "d", false, "Enable debug logging")
	rootCmd.Flags().StringVarP(&outputFile, "output", "o", "/tmp/smart_suggestion", "Output file path")
	rootCmd.Flags().BoolVarP(&sendContext, "context", "c", false, "Include context information")

	// Proxy command flags
	proxyCmd.Flags().StringVarP(&proxyLogFile, "log-file", "l", "/tmp/smart_suggestion_proxy.log", "Proxy log file path")
	proxyCmd.Flags().StringVarP(&sessionID, "session-id", "", "", "Session ID for log isolation (auto-generated if not provided)")
	proxyCmd.Flags().BoolVarP(&debug, "debug", "d", false, "Enable debug logging")

	// Rotate-logs command flags
	rotateCmd.Flags().StringVarP(&proxyLogFile, "log-file", "l", "/tmp/smart_suggestion_proxy.log", "Log file path to rotate (required)")
	rotateCmd.Flags().BoolVarP(&debug, "debug", "d", false, "Enable debug logging")

	// Update command flags
	updateCmd.Flags().BoolP("check-only", "c", false, "Only check for updates, don't install")

	rootCmd.AddCommand(proxyCmd)
	rootCmd.AddCommand(rotateCmd)
	rootCmd.AddCommand(versionCmd)
	rootCmd.AddCommand(updateCmd)

	// Only require provider and input for the main fetch command
	if len(os.Args) > 1 && os.Args[1] != "proxy" && os.Args[1] != "rotate-logs" {
		rootCmd.MarkFlagRequired("provider")
		rootCmd.MarkFlagRequired("input")
	}

	// Require log-file for rotate-logs command
	if len(os.Args) > 1 && os.Args[1] == "rotate-logs" {
		rotateCmd.MarkFlagRequired("log-file")
	}

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func runFetch(cmd *cobra.Command, args []string) {
	if systemPrompt == "" {
		systemPrompt = defaultSystemPrompt
	}

	// Build the complete prompt with context if requested
	completePrompt := systemPrompt
	if sendContext {
		contextInfo, err := buildContextInfo()
		if err != nil {
			if debug {
				logDebug("Failed to build context info", map[string]any{
					"error": err.Error(),
				})
			}
			// Continue without context if there's an error
		} else {
			completePrompt = systemPrompt + "\n\n" + contextInfo
		}
	}

	// Update the global systemPrompt for API calls
	systemPrompt = completePrompt

	var suggestion string
	var err error

	switch strings.ToLower(provider) {
	case "openai":
		suggestion, err = fetchOpenAI()
	case "azure_openai":
		suggestion, err = fetchAzureOpenAI()
	case "anthropic":
		suggestion, err = fetchAnthropic()
	case "gemini":
		suggestion, err = fetchGemini()
	case "deepseek":
		suggestion, err = fetchDeepSeek()
	default:
		err = fmt.Errorf("unsupported provider: %s", provider)
	}

	if err != nil {
		if debug {
			logDebug("Error occurred", map[string]any{
				"error":    err.Error(),
				"provider": provider,
				"input":    input,
			})
		}

		errorMsg := fmt.Sprintf("Error fetching suggestions from %s API: %v", provider, err)
		if err := os.WriteFile("/tmp/.smart_suggestion_error", []byte(errorMsg), 0644); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to write error file: %v\n", err)
		}
		os.Exit(1)
	}

	// Parse the suggestion to extract only the command part
	finalSuggestion := parseAndExtractCommand(suggestion)

	if debug {
		logDebug("Successfully fetched suggestion", map[string]any{
			"provider":          provider,
			"input":             input,
			"original_response": suggestion,
			"parsed_suggestion": finalSuggestion,
		})
	}

	if err := os.WriteFile(outputFile, []byte(finalSuggestion), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to write suggestion to file: %v\n", err)
		os.Exit(1)
	}
}

func fetchOpenAI() (string, error) {
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		return "", fmt.Errorf("OPENAI_API_KEY environment variable is not set")
	}

	baseURL := os.Getenv("OPENAI_BASE_URL")
	if baseURL == "" {
		baseURL = "https://api.openai.com"
	}

	// Handle different base URL formats
	var url string
	if strings.HasPrefix(baseURL, "http://") || strings.HasPrefix(baseURL, "https://") {
		// Base URL already includes protocol
		baseURL = strings.TrimSuffix(baseURL, "/")
		url = fmt.Sprintf("%s/v1/chat/completions", baseURL)
	} else {
		// Base URL is just hostname, add https protocol
		url = fmt.Sprintf("https://%s/v1/chat/completions", baseURL)
	}

	request := OpenAIRequest{
		Model: "gpt-4o-mini",
		Messages: []OpenAIMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: input},
		},
	}

	jsonData, err := json.Marshal(request)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	if debug {
		logDebug("Sending OpenAI request", map[string]any{
			"url":     url,
			"request": string(jsonData),
		})
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	if debug {
		logDebug("Received OpenAI response", map[string]any{
			"status":   resp.Status,
			"response": string(body),
		})
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	var response OpenAIResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return "", fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if response.Error != nil {
		return "", fmt.Errorf("OpenAI API error: %s", response.Error.Message)
	}

	if len(response.Choices) == 0 {
		return "", fmt.Errorf("no choices returned from OpenAI API")
	}

	return response.Choices[0].Message.Content, nil
}

func fetchAzureOpenAI() (string, error) {
	apiKey := os.Getenv("AZURE_OPENAI_API_KEY")
	if apiKey == "" {
		return "", fmt.Errorf("AZURE_OPENAI_API_KEY environment variable is not set")
	}

	// Get deployment name - required for both custom and standard URLs
	deploymentName := os.Getenv("AZURE_OPENAI_DEPLOYMENT_NAME")
	if deploymentName == "" {
		return "", fmt.Errorf("AZURE_OPENAI_DEPLOYMENT_NAME environment variable is not set")
	}

	// Check if custom base URL is provided
	baseURL := os.Getenv("AZURE_OPENAI_BASE_URL")
	var url string

	if baseURL != "" {
		// Custom base URL provided - use it directly
		apiVersion := os.Getenv("AZURE_OPENAI_API_VERSION")
		if apiVersion == "" {
			apiVersion = "2024-10-21" // Default to latest stable version
		}

		// Handle different base URL formats
		if strings.HasPrefix(baseURL, "http://") || strings.HasPrefix(baseURL, "https://") {
			// Base URL already includes protocol
			baseURL = strings.TrimSuffix(baseURL, "/")
			url = fmt.Sprintf("%s/openai/deployments/%s/chat/completions?api-version=%s", baseURL, deploymentName, apiVersion)
		} else {
			// Base URL is just hostname, add https protocol
			url = fmt.Sprintf("https://%s/openai/deployments/%s/chat/completions?api-version=%s", baseURL, deploymentName, apiVersion)
		}
	} else {
		// Standard Azure OpenAI format - requires resource name and deployment name
		resourceName := os.Getenv("AZURE_OPENAI_RESOURCE_NAME")
		if resourceName == "" {
			return "", fmt.Errorf("AZURE_OPENAI_RESOURCE_NAME environment variable is not set")
		}

		// API version for Azure OpenAI
		apiVersion := os.Getenv("AZURE_OPENAI_API_VERSION")
		if apiVersion == "" {
			apiVersion = "2024-10-21" // Default to latest stable version
		}

		// Azure OpenAI endpoint format
		url = fmt.Sprintf("https://%s.openai.azure.com/openai/deployments/%s/chat/completions?api-version=%s",
			resourceName, deploymentName, apiVersion)
	}

	request := AzureOpenAIRequest{
		Model: deploymentName, // In Azure OpenAI, this should match the deployment name
		Messages: []OpenAIMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: input},
		},
	}

	jsonData, err := json.Marshal(request)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	if debug {
		logDebug("Sending Azure OpenAI request", map[string]any{
			"url":        url,
			"deployment": deploymentName,
			"request":    string(jsonData),
		})
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("api-key", apiKey) // Azure OpenAI uses "api-key" header

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	if debug {
		logDebug("Received Azure OpenAI response", map[string]any{
			"status":   resp.Status,
			"response": string(body),
		})
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	var response AzureOpenAIResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return "", fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if response.Error != nil {
		return "", fmt.Errorf("Azure OpenAI API error: %s", response.Error.Message)
	}

	if len(response.Choices) == 0 {
		return "", fmt.Errorf("no choices returned from Azure OpenAI API")
	}

	return response.Choices[0].Message.Content, nil
}

func fetchAnthropic() (string, error) {
	apiKey := os.Getenv("ANTHROPIC_API_KEY")
	if apiKey == "" {
		return "", fmt.Errorf("ANTHROPIC_API_KEY environment variable is not set")
	}

	baseURL := os.Getenv("ANTHROPIC_BASE_URL")
	if baseURL == "" {
		baseURL = "https://api.anthropic.com"
	}

	// Handle different base URL formats
	var url string
	if strings.HasPrefix(baseURL, "http://") || strings.HasPrefix(baseURL, "https://") {
		// Base URL already includes protocol
		baseURL = strings.TrimSuffix(baseURL, "/")
		url = fmt.Sprintf("%s/v1/messages", baseURL)
	} else {
		// Base URL is just hostname, add https protocol
		url = fmt.Sprintf("https://%s/v1/messages", baseURL)
	}

	request := AnthropicRequest{
		Model:     "claude-3-5-sonnet-20241022",
		MaxTokens: 1000,
		System:    systemPrompt,
		Messages: []AnthropicMessage{
			{Role: "user", Content: input},
		},
	}

	jsonData, err := json.Marshal(request)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	if debug {
		logDebug("Sending Anthropic request", map[string]any{
			"url":     url,
			"request": string(jsonData),
		})
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	if debug {
		logDebug("Received Anthropic response", map[string]any{
			"status":   resp.Status,
			"response": string(body),
		})
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	var response AnthropicResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return "", fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if response.Type == "error" || response.Error != nil {
		errorMsg := "unknown error"
		if response.Error != nil {
			errorMsg = response.Error.Message
		}
		return "", fmt.Errorf("Anthropic API error: %s", errorMsg)
	}

	if len(response.Content) == 0 {
		return "", fmt.Errorf("no content returned from Anthropic API")
	}

	return response.Content[0].Text, nil
}

// writeToLogFile writes content to a log file with automatic rotation
func writeToLogFile(logFilePath, content string) error {
	// Check and rotate log file if necessary
	if err := logRotator.CheckAndRotate(logFilePath); err != nil {
		log.Printf("Failed to rotate log file: %v", err)
		// Continue with logging even if rotation fails
	}

	file, err := os.OpenFile(logFilePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("failed to open log file: %w", err)
	}
	defer file.Close()

	_, err = file.WriteString(content + "\n")
	return err
}

func logDebug(message string, data map[string]any) {
	logFilePath := "/tmp/smart-suggestion.log"

	logEntry := map[string]any{
		"date": time.Now().Format(time.RFC3339),
		"log":  message,
	}

	for k, v := range data {
		logEntry[k] = v
	}

	jsonData, err := json.Marshal(logEntry)
	if err != nil {
		log.Printf("Failed to marshal debug log: %v", err)
		return
	}

	if err := writeToLogFile(logFilePath, string(jsonData)); err != nil {
		log.Printf("Failed to write debug log: %v", err)
	}
}

// buildContextInfo builds context information similar to the zsh plugin
func buildContextInfo() (string, error) {
	var contextParts []string

	// Get user information
	currentUser := os.Getenv("USER")
	if currentUser == "" {
		currentUser = "unknown"
	}

	// Get current directory
	currentDir, err := os.Getwd()
	if err != nil {
		currentDir = "unknown"
	}

	// Get shell information
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "unknown"
	}

	// Get terminal information
	term := os.Getenv("TERM")
	if term == "" {
		term = "unknown"
	}

	// Get system information
	systemInfo, err := getSystemInfo()
	if err != nil {
		if debug {
			logDebug("Failed to get system info", map[string]any{
				"error": err.Error(),
			})
		}
		systemInfo = "unknown system"
	}

	// Get user ID information
	userID, err := getUserID()
	if err != nil {
		if debug {
			logDebug("Failed to get user ID", map[string]any{
				"error": err.Error(),
			})
		}
		userID = "unknown"
	}

	// Get uname information
	unameInfo, err := getUnameInfo()
	if err != nil {
		if debug {
			logDebug("Failed to get uname info", map[string]any{
				"error": err.Error(),
			})
		}
		unameInfo = "unknown"
	}

	// Build the basic context
	basicContext := fmt.Sprintf("# Context:\nYou are user %s with id %s in directory %s. Your shell is %s and your terminal is %s running on %s. %s",
		currentUser, userID, currentDir, shell, term, unameInfo, systemInfo)
	contextParts = append(contextParts, basicContext)

	// Get aliases
	aliases, err := getAliases()
	if err != nil {
		if debug {
			logDebug("Failed to get aliases", map[string]any{
				"error": err.Error(),
			})
		}
	} else {
		contextParts = append(contextParts, "\n# This is the alias defined in your shell:\n", aliases)
	}

	shellHistory, err := getShellHistory()
	if err != nil {
		if debug {
			logDebug("Failed to get shell history", map[string]any{
				"error": err.Error(),
			})
		}
	} else {
		contextParts = append(contextParts, "\n# Shell history:\n", shellHistory)
	}

	// Get tmux buffer content if available
	shellBuffer, err := getShellBuffer()
	if err != nil {
		if debug {
			logDebug("Failed to get shell buffer", map[string]any{
				"error": err.Error(),
			})
		}
	} else {
		contextParts = append(contextParts, "\n# Shell buffer:\n", shellBuffer)
	}

	return strings.Join(contextParts, ""), nil
}

// getSystemInfo gets system information similar to the zsh plugin
func getSystemInfo() (string, error) {
	switch runtime.GOOS {
	case "darwin":
		// macOS: use sw_vers command
		cmd := exec.Command("sw_vers")
		output, err := cmd.Output()
		if err != nil {
			return "", fmt.Errorf("failed to run sw_vers: %w", err)
		}

		// Process output similar to: $(sw_vers | xargs | sed 's/ /./g')
		lines := strings.Split(strings.TrimSpace(string(output)), "\n")
		var parts []string
		for _, line := range lines {
			parts = append(parts, strings.ReplaceAll(line, " ", "."))
		}
		return fmt.Sprintf("Your system is %s.", strings.Join(parts, ".")), nil

	default:
		// Linux and others: read /etc/*-release files
		releaseFiles := []string{"/etc/os-release", "/etc/lsb-release", "/etc/redhat-release"}
		var content []string

		for _, file := range releaseFiles {
			data, err := os.ReadFile(file)
			if err == nil {
				content = append(content, string(data))
			}
		}

		if len(content) == 0 {
			return "", fmt.Errorf("no release files found")
		}

		// Process similar to: $(cat /etc/*-release | xargs | sed 's/ /,/g')
		allContent := strings.Join(content, " ")
		processedContent := strings.ReplaceAll(strings.TrimSpace(allContent), " ", ",")
		return fmt.Sprintf("Your system is %s.", processedContent), nil
	}
}

// getUserID gets user ID information
func getUserID() (string, error) {
	cmd := exec.Command("id")
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to run id command: %w", err)
	}
	return strings.TrimSpace(string(output)), nil
}

func fetchGemini() (string, error) {
	apiKey := os.Getenv("GEMINI_API_KEY")
	if apiKey == "" {
		return "", fmt.Errorf("GEMINI_API_KEY environment variable is not set")
	}

	baseURL := os.Getenv("GEMINI_BASE_URL")
	if baseURL == "" {
		baseURL = "https://generativelanguage.googleapis.com"
	}

	model := os.Getenv("GEMINI_MODEL")
	if model == "" {
		model = "gemini-2.5-flash"
	}

	// Handle different base URL formats
	var url string
	if strings.HasPrefix(baseURL, "http://") || strings.HasPrefix(baseURL, "https://") {
		// Base URL already includes protocol
		baseURL = strings.TrimSuffix(baseURL, "/")
		url = fmt.Sprintf("%s/v1beta/models/%s:generateContent?key=%s", baseURL, model, apiKey)
	} else {
		// Base URL is just hostname, add https protocol
		url = fmt.Sprintf("https://%s/v1beta/models/%s:generateContent?key=%s", baseURL, model, apiKey)
	}

	// Gemini API expects a different format - system prompt and user input are combined
	var contents []GeminiContent

	// Add system prompt as user message (Gemini doesn't have separate system role)
	if systemPrompt != "" {
		contents = append(contents, GeminiContent{
			Parts: []GeminiPart{{Text: systemPrompt}},
			Role:  "user",
		})
		contents = append(contents, GeminiContent{
			Parts: []GeminiPart{{Text: "I understand. I'll follow these instructions."}},
			Role:  "model",
		})
	}

	// Add user input
	contents = append(contents, GeminiContent{
		Parts: []GeminiPart{{Text: input}},
		Role:  "user",
	})

	request := GeminiRequest{
		Contents: contents,
	}

	jsonData, err := json.Marshal(request)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	if debug {
		logDebug("Sending Gemini request", map[string]any{
			"url":     url,
			"request": string(jsonData),
		})
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	if debug {
		logDebug("Received Gemini response", map[string]any{
			"status":   resp.Status,
			"response": string(body),
		})
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	var response GeminiResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return "", fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if response.Error != nil {
		return "", fmt.Errorf("Gemini API error: %s", response.Error.Message)
	}

	if len(response.Candidates) == 0 {
		return "", fmt.Errorf("no candidates returned from Gemini API")
	}

	if len(response.Candidates[0].Content.Parts) == 0 {
		return "", fmt.Errorf("no content parts returned from Gemini API")
	}

	return response.Candidates[0].Content.Parts[0].Text, nil
}

// getUnameInfo gets uname information
func getUnameInfo() (string, error) {
	cmd := exec.Command("uname", "-a")
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to run uname command: %w", err)
	}
	return strings.TrimSpace(string(output)), nil
}

// getAliases gets shell aliases
func getAliases() (string, error) {
	// Try to get aliases using the alias command
	cmd := exec.Command("alias")
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get aliases: %w", err)
	}

	// No need to escape quotes here as Go handles JSON encoding properly
	return strings.TrimSpace(string(output)), nil
}

func getShellHistory() (string, error) {
	// Get the number of lines to fetch
	numLinesStr := os.Getenv("SMART_SUGGESTION_HISTORY_LINES")
	if numLinesStr == "" {
		numLinesStr = "10"
	}

	cmd := exec.Command("fc", "-ln", fmt.Sprintf("-%s", numLinesStr))
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to run history command: %w", err)
	}
	return strings.TrimSpace(string(output)), nil
}

// createProcessLock creates a lock file to prevent duplicate processes
func createProcessLock(lockPath string) (*os.File, error) {
	// Create directory if it doesn't exist
	dir := filepath.Dir(lockPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create lock directory: %w", err)
	}

	// Try to create and lock the file
	file, err := os.OpenFile(lockPath, os.O_CREATE|os.O_WRONLY|os.O_EXCL, 0644)
	if err != nil {
		if os.IsExist(err) {
			// Check if the process is still running
			if isProcessRunning(lockPath) {
				return nil, fmt.Errorf("another instance is already running")
			}
			// Remove stale lock file
			os.Remove(lockPath)
			// Try again
			file, err = os.OpenFile(lockPath, os.O_CREATE|os.O_WRONLY|os.O_EXCL, 0644)
			if err != nil {
				return nil, fmt.Errorf("failed to create lock file: %w", err)
			}
		} else {
			return nil, fmt.Errorf("failed to create lock file: %w", err)
		}
	}

	// Try to acquire an exclusive lock
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		file.Close()
		os.Remove(lockPath)
		return nil, fmt.Errorf("failed to acquire lock: %w", err)
	}

	// Write PID to the lock file
	pid := os.Getpid()
	if _, err := file.WriteString(fmt.Sprintf("%d\n", pid)); err != nil {
		file.Close()
		os.Remove(lockPath)
		return nil, fmt.Errorf("failed to write PID to lock file: %w", err)
	}

	if err := file.Sync(); err != nil {
		file.Close()
		os.Remove(lockPath)
		return nil, fmt.Errorf("failed to sync lock file: %w", err)
	}

	return file, nil
}

// isProcessRunning checks if the process in the lock file is still running
func isProcessRunning(lockPath string) bool {
	data, err := os.ReadFile(lockPath)
	if err != nil {
		return false
	}

	pidStr := strings.TrimSpace(string(data))
	pid, err := strconv.Atoi(pidStr)
	if err != nil {
		return false
	}

	// Check if process is running by sending signal 0
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}

	err = process.Signal(syscall.Signal(0))
	return err == nil
}

// cleanupProcessLock removes the lock file and closes the file handle
func cleanupProcessLock(file *os.File, lockPath string) {
	if file != nil {
		syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
		file.Close()
	}
	os.Remove(lockPath)
}

// generateSessionID generates a unique session ID
func generateSessionID() (string, error) {
	// Generate random bytes
	randomBytes := make([]byte, 8)
	if _, err := rand.Read(randomBytes); err != nil {
		return "", fmt.Errorf("failed to generate random bytes: %w", err)
	}

	// Create session ID with PID and timestamp for uniqueness
	pid := os.Getpid()
	timestamp := time.Now().Unix()
	randomHex := hex.EncodeToString(randomBytes)

	sessionID := fmt.Sprintf("%d_%d_%s", pid, timestamp, randomHex)
	return sessionID, nil
}

// getSessionBasedLogFile returns the session-specific log file path
func getSessionBasedLogFile(baseLogFile, sessionID string) string {
	if sessionID == "" {
		return baseLogFile
	}

	// Extract directory and base filename
	dir := filepath.Dir(baseLogFile)
	base := filepath.Base(baseLogFile)

	// Remove extension if present
	ext := filepath.Ext(base)
	if ext != "" {
		base = strings.TrimSuffix(base, ext)
	}

	// Create session-specific filename
	sessionLogFile := fmt.Sprintf("%s.%s%s", base, sessionID, ext)
	return filepath.Join(dir, sessionLogFile)
}

// getSessionBasedLockFile returns the session-specific lock file path
func getSessionBasedLockFile(baseLockFile, sessionID string) string {
	if sessionID == "" {
		return baseLockFile
	}

	dir := filepath.Dir(baseLockFile)
	base := filepath.Base(baseLockFile)
	ext := filepath.Ext(base)
	if ext != "" {
		base = strings.TrimSuffix(base, ext)
	}

	sessionLockFile := fmt.Sprintf("%s.%s%s", base, sessionID, ext)
	return filepath.Join(dir, sessionLockFile)
}

// getCurrentSessionID gets the current session ID from environment or generates one
func getCurrentSessionID() string {
	// Try to get from environment variable first
	if sessionID := os.Getenv("SMART_SUGGESTION_SESSION_ID"); sessionID != "" {
		return sessionID
	}

	// Try to get from TTY device name
	if ttyName := getTTYName(); ttyName != "" {
		return ttyName
	}

	// Generate a new one based on PID
	return fmt.Sprintf("pid_%d", os.Getpid())
}

// getTTYName gets the current TTY device name for session identification
func getTTYName() string {
	// Try to get TTY name from various sources
	if tty := os.Getenv("TTY"); tty != "" {
		// Extract just the device name
		if parts := strings.Split(tty, "/"); len(parts) > 0 {
			return strings.ReplaceAll(parts[len(parts)-1], ".", "_")
		}
	}

	// Try to get from tty command
	cmd := exec.Command("tty")
	output, err := cmd.Output()
	if err == nil {
		ttyPath := strings.TrimSpace(string(output))
		if parts := strings.Split(ttyPath, "/"); len(parts) > 0 {
			deviceName := parts[len(parts)-1]
			// Replace special characters to make it filesystem-safe
			deviceName = strings.ReplaceAll(deviceName, ".", "_")
			deviceName = strings.ReplaceAll(deviceName, ":", "_")
			return deviceName
		}
	}

	return ""
}

// cleanupOldSessionLogs removes old session log files
func cleanupOldSessionLogs(baseLogPath string, maxAge time.Duration) error {
	dir := filepath.Dir(baseLogPath)
	base := filepath.Base(baseLogPath)

	// Remove extension for pattern matching
	ext := filepath.Ext(base)
	if ext != "" {
		base = strings.TrimSuffix(base, ext)
	}

	// Pattern to match session log files
	pattern := fmt.Sprintf("%s.*%s", base, ext)

	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("failed to read directory %s: %w", dir, err)
	}

	cutoff := time.Now().Add(-maxAge)
	var removedFiles []string

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		filename := entry.Name()
		// Check if this looks like a session log file
		if matched, _ := filepath.Match(pattern, filename); !matched {
			continue
		}

		// Skip the base file itself
		if filename == filepath.Base(baseLogPath) {
			continue
		}

		fullPath := filepath.Join(dir, filename)
		info, err := os.Stat(fullPath)
		if err != nil {
			continue
		}

		// Remove if older than cutoff
		if info.ModTime().Before(cutoff) {
			if err := os.Remove(fullPath); err == nil {
				removedFiles = append(removedFiles, filename)
			}
		}
	}

	if len(removedFiles) > 0 && debug {
		logDebug("Cleaned up old session logs", map[string]any{
			"removed_files": removedFiles,
			"base_path":     baseLogPath,
		})
	}

	return nil
}

// runProxy starts the shell proxy mode using PTY
func runProxy(cmd *cobra.Command, args []string) {
	// Check if we're already inside a proxy session to prevent nesting
	if os.Getenv("SMART_SUGGESTION_PROXY_ACTIVE") != "" {
		if debug {
			logDebug("Already inside a proxy session, preventing nesting", map[string]any{
				"existing_proxy_pid": os.Getenv("SMART_SUGGESTION_PROXY_ACTIVE"),
			})
		}
		return
	}

	// Generate or get session ID
	if sessionID == "" {
		// Auto-generate session ID if not provided
		generatedID, err := generateSessionID()
		if err != nil {
			if debug {
				logDebug("Failed to generate session ID", map[string]any{
					"error": err.Error(),
				})
			}
			fmt.Fprintf(os.Stderr, "Failed to generate session ID: %v\n", err)
			os.Exit(1)
		}
		sessionID = generatedID
	}

	// Create session-based file paths
	sessionLogFile := getSessionBasedLogFile(proxyLogFile, sessionID)
	sessionLockFile := getSessionBasedLockFile("/tmp/smart-suggestion-proxy.lock", sessionID)

	// Create a lock file to prevent duplicate proxy processes for this session
	lockFile, err := createProcessLock(sessionLockFile)
	if err != nil {
		if debug {
			logDebug("Failed to create process lock", map[string]any{
				"error":      err.Error(),
				"lock_path":  sessionLockFile,
				"session_id": sessionID,
			})
		}
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	// Ensure cleanup on exit
	defer cleanupProcessLock(lockFile, sessionLockFile)

	// Set environment variables for child processes
	os.Setenv("SMART_SUGGESTION_SESSION_ID", sessionID)
	// Set proxy active flag with current PID to prevent nesting
	os.Setenv("SMART_SUGGESTION_PROXY_ACTIVE", fmt.Sprintf("%d", os.Getpid()))

	// Clean up old session logs (older than 24 hours)
	if err := cleanupOldSessionLogs(proxyLogFile, 24*time.Hour); err != nil {
		if debug {
			logDebug("Failed to cleanup old session logs", map[string]any{
				"error": err.Error(),
			})
		}
		// Continue even if cleanup fails
	}

	if debug {
		logDebug("Starting shell proxy mode with PTY", map[string]any{
			"log_file":   sessionLogFile,
			"lock_file":  sessionLockFile,
			"session_id": sessionID,
			"pid":        os.Getpid(),
		})
	}

	// Get the user's shell, default to bash if not set
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/bash"
	}

	// Create a shell command
	c := exec.Command(shell)

	// Start the shell with a pty
	ptmx, err := pty.Start(c)
	if err != nil {
		if debug {
			logDebug("Failed to start PTY", map[string]any{
				"error": err.Error(),
				"shell": shell,
			})
		}
		fmt.Fprintf(os.Stderr, "Failed to start PTY: %v\n", err)
		os.Exit(1)
	}
	defer func() { _ = ptmx.Close() }()

	// Handle pty size changes
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGWINCH)
	go func() {
		for range ch {
			if err := pty.InheritSize(os.Stdin, ptmx); err != nil {
				if debug {
					logDebug("Error resizing pty", map[string]any{
						"error": err.Error(),
					})
				}
			}
		}
	}()
	ch <- syscall.SIGWINCH // Initial resize
	defer func() { signal.Stop(ch); close(ch) }()

	// Set stdin in raw mode to properly handle terminal input (only if it's a terminal)
	var oldState *term.State
	if term.IsTerminal(int(os.Stdin.Fd())) {
		oldState, err = term.MakeRaw(int(os.Stdin.Fd()))
		if err != nil {
			if debug {
				logDebug("Failed to set raw mode", map[string]any{
					"error": err.Error(),
				})
			}
			fmt.Fprintf(os.Stderr, "Failed to set raw mode: %v\n", err)
			os.Exit(1)
		}
		defer func() {
			if oldState != nil {
				_ = term.Restore(int(os.Stdin.Fd()), oldState)
			}
		}()
	} else {
		if debug {
			logDebug("Stdin is not a terminal, skipping raw mode", map[string]any{
				"stdin_fd": int(os.Stdin.Fd()),
			})
		}
	}

	// try to delete session log file if it exists
	if _, err := os.Stat(sessionLogFile); err == nil {
		if err := os.Remove(sessionLogFile); err != nil {
			if debug {
				logDebug("Failed to delete session log file", map[string]any{
					"error":      err.Error(),
					"log_file":   sessionLogFile,
					"session_id": sessionID,
				})
			}
			fmt.Fprintf(os.Stderr, "Failed to delete session log file: %v\n", err)
			os.Exit(1)
		}
	}

	// Open session log file for writing
	logFile, err := os.OpenFile(sessionLogFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		if debug {
			logDebug("Failed to open session log file", map[string]any{
				"error":      err.Error(),
				"log_file":   sessionLogFile,
				"session_id": sessionID,
			})
		}
		fmt.Fprintf(os.Stderr, "Failed to open session log file: %v\n", err)
		os.Exit(1)
	}
	defer logFile.Close()

	// Create a tee writer to write to both stdout and log file
	teeWriter := io.MultiWriter(os.Stdout, logFile)

	// Handle graceful shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	// Start goroutines for copying data
	done := make(chan struct{})

	// Copy from stdin to pty (user input)
	go func() {
		defer close(done)
		_, err := io.Copy(ptmx, os.Stdin)
		if err != nil && debug {
			logDebug("Error copying stdin to pty", map[string]any{
				"error": err.Error(),
			})
		}
	}()

	// Copy from pty to stdout and log file (shell output)
	go func() {
		_, err := io.Copy(teeWriter, ptmx)
		if err != nil && debug {
			logDebug("Error copying pty to output", map[string]any{
				"error": err.Error(),
			})
		}
		done <- struct{}{}
	}()

	// Wait for either completion or signal
	select {
	case <-done:
		if debug {
			logDebug("PTY session completed", map[string]any{
				"log_file": proxyLogFile,
			})
		}
	case sig := <-sigCh:
		if debug {
			logDebug("Received signal, shutting down", map[string]any{
				"signal":   sig.String(),
				"log_file": proxyLogFile,
			})
		}
	}
}

// getShellBuffer gets terminal buffer content using multiple methods
func getShellBuffer() (string, error) {
	// Try tmux first if available
	if os.Getenv("TMUX") != "" {
		cmd := exec.Command("tmux", "capture-pane", "-pS", "-")
		output, err := cmd.Output()
		if err == nil {
			return strings.TrimSpace(string(output)), nil
		}
		if debug {
			logDebug("Failed to get tmux buffer", map[string]any{
				"error": err.Error(),
			})
		}
	}

	// Try kitty if available
	if os.Getenv("KITTY_LISTEN_ON") != "" {
		cmd := exec.Command("kitten", "@", "get-text", "--extent", "all")
		output, err := cmd.Output()
		if err == nil {
			return strings.TrimSpace(string(output)), nil
		}
		if debug {
			logDebug("Failed to get kitty scrollback buffer", map[string]any{
				"error": err.Error(),
			})
		}
	}

	// Try to read from session-specific proxy log file if it exists
	currentSessionID := getCurrentSessionID()
	if currentSessionID != "" && proxyLogFile != "" {
		sessionLogFile := getSessionBasedLogFile(proxyLogFile, currentSessionID)
		content, err := readLatestProxyContent(sessionLogFile)
		if err == nil {
			return content, nil
		}
		if debug {
			logDebug("Failed to read session proxy log", map[string]any{
				"error":      err.Error(),
				"file":       sessionLogFile,
				"session_id": currentSessionID,
			})
		}
	}

	// Fallback to base proxy log file if session-specific file doesn't exist
	if proxyLogFile != "" {
		content, err := readLatestProxyContent(proxyLogFile)
		if err == nil {
			return content, nil
		}
		if debug {
			logDebug("Failed to read base proxy log", map[string]any{
				"error": err.Error(),
				"file":  proxyLogFile,
			})
		}
	}

	// Try screen if available
	content, err := getScreenBuffer()
	if err == nil {
		return content, nil
	}

	// Try to get terminal buffer using tput if available
	content, err = getTerminalBufferWithTput()
	if err == nil {
		return content, nil
	}

	return "", fmt.Errorf("no terminal buffer available - not in tmux/screen session and no proxy log found")
}

// readLatestProxyContent reads the latest content from proxy log file
func readLatestProxyContent(logFile string) (string, error) {
	file, err := os.Open(logFile)
	if err != nil {
		return "", fmt.Errorf("failed to open proxy log file: %w", err)
	}
	defer file.Close()

	// Read the last N lines from the file
	const maxLines = 50
	scanner := bufio.NewScanner(file)
	var lines []string

	for scanner.Scan() {
		lines = append(lines, scanner.Text())
		// Keep only the last maxLines
		if len(lines) > maxLines {
			lines = lines[1:]
		}
	}

	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("failed to read proxy log file: %w", err)
	}

	return strings.Join(lines, "\n"), nil
}

// getScreenBuffer tries to get buffer from GNU screen
func getScreenBuffer() (string, error) {
	// Check if we're in a screen session
	if os.Getenv("STY") == "" {
		return "", fmt.Errorf("not in a screen session")
	}

	// Try to capture screen buffer
	cmd := exec.Command("screen", "-X", "hardcopy", "/tmp/screen_buffer.txt")
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("failed to capture screen buffer: %w", err)
	}

	// Read the captured buffer
	content, err := os.ReadFile("/tmp/screen_buffer.txt")
	if err != nil {
		return "", fmt.Errorf("failed to read screen buffer: %w", err)
	}

	// Clean up the temporary file
	os.Remove("/tmp/screen_buffer.txt")

	return strings.TrimSpace(string(content)), nil
}

// getTerminalBufferWithTput tries to get terminal content using tput
func getTerminalBufferWithTput() (string, error) {
	// This is a limited approach that only works in some terminals
	// Get terminal size
	rowsCmd := exec.Command("tput", "lines")
	rowsOutput, err := rowsCmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get terminal rows: %w", err)
	}

	rows, err := strconv.Atoi(strings.TrimSpace(string(rowsOutput)))
	if err != nil {
		return "", fmt.Errorf("failed to parse terminal rows: %w", err)
	}

	// This is a very limited approach and may not work in all terminals
	// We can try to use ANSI escape sequences to query the terminal
	// but this is complex and terminal-dependent

	// For now, return an error indicating this method is not implemented
	return "", fmt.Errorf("tput method not fully implemented (terminal has %d rows)", rows)
}

// runRotateLogs handles the rotate-logs command
func runRotateLogs(cmd *cobra.Command, args []string) {
	if proxyLogFile == "" {
		fmt.Fprintf(os.Stderr, "Error: log file path is required\n")
		os.Exit(1)
	}

	if debug {
		logDebug("Starting log rotation", map[string]any{
			"log_file": proxyLogFile,
		})
	}

	// Check if the log file exists
	if _, err := os.Stat(proxyLogFile); os.IsNotExist(err) {
		if debug {
			logDebug("Log file does not exist, nothing to rotate", map[string]any{
				"log_file": proxyLogFile,
			})
		}
		fmt.Printf("Log file %s does not exist, nothing to rotate\n", proxyLogFile)
		return
	}

	// Perform log rotation
	if err := logRotator.ForceRotate(proxyLogFile); err != nil {
		if debug {
			logDebug("Failed to rotate log file", map[string]any{
				"error":    err.Error(),
				"log_file": proxyLogFile,
			})
		}
		fmt.Fprintf(os.Stderr, "Error rotating log file %s: %v\n", proxyLogFile, err)
		os.Exit(1)
	}

	if debug {
		logDebug("Log rotation completed successfully", map[string]any{
			"log_file": proxyLogFile,
		})
	}

	// Get backup files to show what was created
	backups, err := logRotator.GetBackupFiles(proxyLogFile)
	if err != nil {
		fmt.Printf("Log file %s rotated successfully\n", proxyLogFile)
	} else {
		fmt.Printf("Log file %s rotated successfully. Backup files: %v\n", proxyLogFile, backups)
	}
}

func fetchDeepSeek() (string, error) {
	apiKey := os.Getenv("DEEPSEEK_API_KEY")
	if apiKey == "" {
		return "", fmt.Errorf("DEEPSEEK_API_KEY environment variable is not set")
	}

	baseURL := os.Getenv("DEEPSEEK_BASE_URL")
	if baseURL == "" {
		baseURL = "https://api.deepseek.com"
	}

	// Handle different base URL formats
	var url string
	if strings.HasPrefix(baseURL, "http://") || strings.HasPrefix(baseURL, "https://") {
		// Base URL already includes protocol
		baseURL = strings.TrimSuffix(baseURL, "/")
		url = fmt.Sprintf("%s/chat/completions", baseURL)
	} else {
		// Base URL is just hostname, add https protocol
		url = fmt.Sprintf("https://%s/chat/completions", baseURL)
	}

	// Get model from environment or use default
	model := os.Getenv("DEEPSEEK_MODEL")
	if model == "" {
		model = "deepseek-chat" // Default to deepseek-chat which points to DeepSeek-V3-0324
	}

	request := DeepSeekRequest{
		Model: model,
		Messages: []OpenAIMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: input},
		},
	}

	jsonData, err := json.Marshal(request)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	if debug {
		logDebug("Sending DeepSeek request", map[string]any{
			"url":     url,
			"request": string(jsonData),
		})
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	if debug {
		logDebug("Received DeepSeek response", map[string]any{
			"status":   resp.Status,
			"response": string(body),
		})
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	var response DeepSeekResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return "", fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if response.Error != nil {
		return "", fmt.Errorf("DeepSeek API error: %s", response.Error.Message)
	}

	if len(response.Choices) == 0 {
		return "", fmt.Errorf("no choices returned from DeepSeek API")
	}

	return response.Choices[0].Message.Content, nil
}

func runUpdate(cmd *cobra.Command, args []string) {
	checkOnly, _ := cmd.Flags().GetBool("check-only")

	fmt.Println("Checking for updates...")

	// Get current version
	currentVersion := Version
	if currentVersion == "dev" {
		// TO TEST: Comment out this two lines and uncomment the line below to allow updating from development version
		fmt.Println("Cannot update development version. Please install from releases.")
		os.Exit(1)
		// currentVersion = "0.0.0"
	}

	// Check for latest version
	latestVersion, downloadURL, err := getLatestVersion()
	if err != nil {
		fmt.Printf("Failed to check for updates: %v\n", err)
		os.Exit(1)
	}

	if currentVersion == latestVersion {
		fmt.Println("Smart Suggestion is already up to date!")
		if checkOnly {
			os.Exit(0)
		} else {
			return
		}
	} else {
		fmt.Printf("New version available: %s (current: %s)\n", latestVersion, currentVersion)
		if checkOnly {
			os.Exit(1) // Exit with code 1 to indicate update available
		}
	}

	// Download and install update
	if err := downloadAndInstallUpdate(downloadURL); err != nil {
		fmt.Printf("Failed to update: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Successfully updated to version %s!\n", latestVersion)
}

func getLatestVersion() (string, string, error) {
	resp, err := http.Get("https://api.github.com/repos/yetone/smart-suggestion/releases/latest")
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", "", err
	}

	var release GitHubRelease
	if err := json.Unmarshal(body, &release); err != nil {
		return "", "", err
	}

	// Detect platform
	platform := fmt.Sprintf("%s-%s", runtime.GOOS, runtime.GOARCH)

	// Find matching asset
	for _, asset := range release.Assets {
		if strings.Contains(asset.Name, platform) {
			return strings.TrimPrefix(release.TagName, "v"), asset.BrowserDownloadURL, nil
		}
	}

	return "", "", fmt.Errorf("no release found for platform %s", platform)
}

func downloadAndInstallUpdate(downloadURL string) error {
	// Create temporary directory
	tempDir, err := os.MkdirTemp("", "smart-suggestion-update")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tempDir)

	// Download archive
	tempFile := filepath.Join(tempDir, "update.tar.gz")
	if err := downloadFile(downloadURL, tempFile); err != nil {
		return err
	}

	// Extract archive
	extractDir := filepath.Join(tempDir, "extracted")
	if err := extractTarGz(tempFile, extractDir); err != nil {
		return err
	}

	// Get current binary path
	currentBinary, err := os.Executable()
	if err != nil {
		return err
	}

	// Find new binary in extracted files
	newBinary := filepath.Join(extractDir, "smart-suggestion")
	if _, err := os.Stat(newBinary); os.IsNotExist(err) {
		// Try to find in subdirectory
		entries, err := os.ReadDir(extractDir)
		if err != nil {
			return err
		}
		for _, entry := range entries {
			if entry.IsDir() {
				candidate := filepath.Join(extractDir, entry.Name(), "smart-suggestion")
				if _, err := os.Stat(candidate); err == nil {
					newBinary = candidate
					break
				}
			}
		}
	}

	// Backup current binary
	backupPath := currentBinary + ".backup"
	if err := copyFile(currentBinary, backupPath); err != nil {
		return err
	}

	// Replace current binary
	if err := copyFile(newBinary, currentBinary); err != nil {
		// Restore backup on failure
		os.Rename(backupPath, currentBinary)
		return err
	}

	// Make executable
	if err := os.Chmod(currentBinary, 0755); err != nil {
		return err
	}

	// Remove backup
	os.Remove(backupPath)

	return nil
}

// Helper functions
// downloadFile downloads a file from the given URL to the specified filepath with retry logic
// It attempts up to 3 times with exponential backoff (1s, 2s, 4s) between retries
func downloadFile(url, filepath string) error {
	maxRetries := 3
	baseDelay := time.Second

	for attempt := 0; attempt < maxRetries; attempt++ {
		// Attempt to download the file
		err := attemptDownload(url, filepath)
		if err == nil {
			return nil // Success
		}

		// If this is the last attempt, return the error
		if attempt == maxRetries-1 {
			return fmt.Errorf("download failed after %d attempts: %w", maxRetries, err)
		}

		// Calculate delay for exponential backoff: 1s, 2s, 4s
		delay := baseDelay * time.Duration(1<<attempt)
		fmt.Printf("Download attempt %d failed, retrying in %v: %v\n", attempt+1, delay, err)
		time.Sleep(delay)
	}

	return fmt.Errorf("download failed after %d attempts", maxRetries)
}

// attemptDownload performs a single download attempt
func attemptDownload(url, filepath string) error {
	// Create HTTP client with timeout
	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	resp, err := client.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// Check for HTTP errors
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP error: %d %s", resp.StatusCode, resp.Status)
	}

	file, err := os.Create(filepath)
	if err != nil {
		return err
	}
	defer file.Close()

	_, err = io.Copy(file, resp.Body)
	return err
}

func extractTarGz(src, dest string) error {
	file, err := os.Open(src)
	if err != nil {
		return err
	}
	defer file.Close()

	gzr, err := gzip.NewReader(file)
	if err != nil {
		return err
	}
	defer gzr.Close()

	tr := tar.NewReader(gzr)

	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		path := filepath.Join(dest, header.Name)

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(path, 0755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
				return err
			}

			file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, os.FileMode(header.Mode))
			if err != nil {
				return err
			}

			_, err = io.Copy(file, tr)
			file.Close()
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func copyFile(src, dst string) error {
	source, err := os.Open(src)
	if err != nil {
		return err
	}
	defer source.Close()

	destination, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destination.Close()

	_, err = io.Copy(destination, source)
	return err
}
