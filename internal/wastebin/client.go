package wastebin

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

// Sentinel errors for the WastebinClient.
var (
	errBothContentAndFilePath     = errors.New("provide either 'content' or 'file_path', not both")
	errNeitherContentNorFilePath  = errors.New("provide either 'content' or 'file_path'")
	errContentEmpty               = errors.New("content cannot be empty")
	errArgsRequired               = errors.New("args is required")
	errContentTooLarge            = errors.New("content exceeds the maximum allowed size")
	errServerRejected             = errors.New("server rejected the request; content may contain disallowed data")
	errContentTooLargeServer      = errors.New("content exceeds the server's maximum allowed size")
	errUnknownHTTP                = errors.New("unknown HTTP error")
	errFileNotText                = errors.New("file is binary or not valid UTF-8 text")
	errConfigRequired             = errors.New("config is required")
	errUnsupportedURLScheme       = errors.New("server URL must use http or https scheme")
	errURLMissingHost             = errors.New("server URL must include a host")
	errTooManyRedirects           = errors.New("stopped after 10 redirects")
	errRedirectDifferentHost      = errors.New("redirect to different host blocked")
	errSandboxTranslationNoMounts = errors.New("sandbox path translation requested but no mounts configured")
	errSandboxTranslationNoMatch  = errors.New("sandbox path does not match any configured mount")
)

// HTTP transport defaults.
const (
	clientTimeout         = 30 * time.Second
	dialTimeout           = 10 * time.Second
	keepAlive             = 30 * time.Second
	tlsHandshakeTimeout   = 10 * time.Second
	responseHeaderTimeout = 30 * time.Second
	idleConnTimeout       = 90 * time.Second
	maxIdleConns          = 100
	maxIdleConnsPerHost   = 10
	maxRedirects          = 10
)

// WastebinClient handles HTTP communication with the Wastebin server.
//
//nolint:revive // stutters as wastebin.WastebinClient, kept for consistency
type WastebinClient struct {
	baseURL    *url.URL
	httpClient *http.Client
	config     *Config
	postURL    string
}

// wastebinRequest is the JSON body sent to the Wastebin API.
type wastebinRequest struct {
	Text      string `json:"text"`
	Extension string `json:"extension,omitempty"`
	Expires   int    `json:"expires,omitempty"`
	Title     string `json:"title,omitempty"`
	//nolint:tagliatelle // intentional, wastebin API uses snake_case
	BurnAfterReading bool   `json:"burn_after_reading,omitempty"`
	Password         string `json:"password,omitempty"`
}

// wastebinResponse is the JSON response from the Wastebin API.
type wastebinResponse struct {
	Path string `json:"path"`
}

// NewWastebinClient creates a new client from Config.
func NewWastebinClient(cfg *Config) (*WastebinClient, error) {
	if cfg == nil {
		return nil, errConfigRequired
	}

	if cfg.ServerURL == "" {
		return nil, errServerURLRequired
	}

	baseURL, err := url.Parse(cfg.ServerURL)
	if err != nil {
		return nil, fmt.Errorf("invalid server URL: %w", err)
	}

	if baseURL.Scheme != "http" && baseURL.Scheme != "https" {
		return nil, errUnsupportedURLScheme
	}

	if baseURL.Host == "" {
		return nil, errURLMissingHost
	}

	httpClient := &http.Client{
		Timeout: clientTimeout,
		Transport: &http.Transport{
			DialContext: (&net.Dialer{
				Timeout:   dialTimeout,
				KeepAlive: keepAlive,
			}).DialContext,
			TLSHandshakeTimeout:   tlsHandshakeTimeout,
			ResponseHeaderTimeout: responseHeaderTimeout,
			IdleConnTimeout:       idleConnTimeout,
			MaxIdleConns:          maxIdleConns,
			MaxIdleConnsPerHost:   maxIdleConnsPerHost,
		},
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= maxRedirects {
				return errTooManyRedirects
			}

			if len(via) > 0 {
				prev := via[len(via)-1]
				if !strings.EqualFold(req.URL.Host, prev.URL.Host) {
					return fmt.Errorf("%w: %s -> %s", errRedirectDifferentHost, prev.URL.Host, req.URL.Host)
				}
			}

			return nil
		},
	}

	// Store a defensive copy of the config to prevent external mutation.
	cfgCopy := *cfg
	cfgCopy.AllowedPaths = slices.Clone(cfg.AllowedPaths)
	cfgCopy.BlockedPaths = slices.Clone(cfg.BlockedPaths)
	cfgCopy.SandboxMounts = slices.Clone(cfg.SandboxMounts)

	return &WastebinClient{
		baseURL:    baseURL,
		httpClient: httpClient,
		config:     &cfgCopy,
		postURL:    baseURL.JoinPath("/").String(),
	}, nil
}

// CreatePaste sends a paste to the Wastebin server.
// It handles content mode (Content field set) or file mode (FilePath set),
// including file reading, path validation, sandbox translation,
// expiration parsing, and response construction.
func (c *WastebinClient) CreatePaste(ctx context.Context, args *CreatePasteArgs) (*PasteResponse, error) {
	if args == nil {
		return nil, errArgsRequired
	}

	// Mutual exclusivity check.
	if args.Content != nil && args.FilePath != nil {
		return nil, errBothContentAndFilePath
	}

	if args.Content == nil && args.FilePath == nil {
		return nil, errNeitherContentNorFilePath
	}

	// Reject empty content in content mode.
	if args.Content != nil && *args.Content == "" {
		return nil, errContentEmpty
	}

	var (
		content string
		ext     string
	)

	if args.FilePath != nil {
		var err error

		content, ext, err = c.readFileContent(*args.FilePath, args.TranslateSandboxPath, args.Extension)
		if err != nil {
			return nil, err
		}
	} else {
		// Content mode.
		content = *args.Content
		if args.Extension != nil {
			ext = *args.Extension
		}
	}

	// Content size pre-check.
	if int64(len(content)) > c.config.MaxContentSize {
		return nil, fmt.Errorf("%w: %d bytes exceeds limit of %d bytes",
			errContentTooLarge, len(content), c.config.MaxContentSize)
	}

	// Parse expiration.
	expires := c.config.DefaultExpires
	if args.Expires != nil && *args.Expires != "" {
		parsed, err := ParseExpiration(*args.Expires, c.config.DefaultExpires)
		if err != nil {
			return nil, fmt.Errorf("invalid expiration: %w", err)
		}

		expires = parsed
	}

	// Build request body.
	reqBody := wastebinRequest{
		Text:      content,
		Extension: ext,
		Expires:   expires,
	}
	if args.Title != nil {
		reqBody.Title = *args.Title
	}

	if args.BurnAfterReading != nil {
		reqBody.BurnAfterReading = *args.BurnAfterReading
	}

	if args.Password != nil {
		reqBody.Password = *args.Password

		if c.baseURL.Scheme == "http" {
			slog.Warn("password is being sent over an unencrypted HTTP connection")
		}
	}

	bodyBytes, err := json.Marshal(reqBody) //nolint:gosec // JSON marshaling is safe; no user-controlled structure
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request body: %w", err)
	}

	// HTTP POST to Wastebin.
	reqURL := c.postURL

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, reqURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	slog.Debug("sending paste to Wastebin", "url", reqURL, "size", len(bodyBytes))

	resp, err := c.httpClient.Do(req)
	if err != nil {
		if isDNSError(err) {
			return nil, fmt.Errorf("cannot resolve the server hostname: %w", err)
		}

		if isConnectionError(err) {
			return nil, fmt.Errorf("cannot connect to Wastebin server; verify the server is running: %w", err)
		}

		return nil, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer closeResponseBody(resp)

	// Handle error status codes.
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body) //nolint:errcheck // Best-effort read of error body for diagnostics
		msg := strings.TrimSpace(string(body))

		return nil, translateHTTPError(resp.StatusCode, msg)
	}

	// Parse response body.
	var wastebinResp wastebinResponse

	err = json.NewDecoder(resp.Body).Decode(&wastebinResp)
	if err != nil {
		return nil, fmt.Errorf("failed to parse Wastebin response: %w", err)
	}

	// Drain response body to EOF for HTTP connection reuse.
	_, _ = io.Copy(io.Discard, resp.Body) //nolint:errcheck // Best-effort drain of body for connection reuse

	// Build PasteResponse.
	return buildPasteResponse(c.baseURL, wastebinResp.Path, ext, args.Password != nil), nil
}

// shouldTranslateSandboxPath determines whether sandbox path translation should
// be applied. Translation is enabled when:
//   - the config has SandboxTransparent set (automatic translation), OR
//   - the caller explicitly set TranslateSandboxPath to true (opt-in).
//
// If no sandbox mounts are configured, translation is never enabled.
func shouldTranslateSandboxPath(cfg *Config, requested *bool) bool {
	if len(cfg.SandboxMounts) == 0 {
		return false
	}

	return cfg.SandboxTransparent || (requested != nil && *requested)
}

// readFileContent reads file content from the given path, handling sandbox
// translation, path validation, text detection, and extension detection.
//
//nolint:nonamedreturns // Both returns are string — named disambiguates.
func (c *WastebinClient) readFileContent(
	filePath string, translateSandboxPath *bool, extArg *string,
) (content, ext string, err error) {
	resolvedPath := filePath

	// 1. Sandbox path translation (if applicable).
	if translateSandboxPath != nil && *translateSandboxPath && len(c.config.SandboxMounts) == 0 {
		return "", "", errSandboxTranslationNoMounts
	}

	if shouldTranslateSandboxPath(c.config, translateSandboxPath) {
		// Check path traversal on the original sandbox path BEFORE any
		// translation occurs. Translate uses filepath.Join which normalizes
		// ".." out of the result, so we must catch traversal here first.
		if hasPathTraversal(resolvedPath) {
			return "", "", errPathTraversal
		}

		translator := NewTranslator(c.config.SandboxMounts)

		translated, ok := translator.Translate(resolvedPath)
		if !ok {
			return "", "", fmt.Errorf("%w: %s", errSandboxTranslationNoMatch, filePath)
		}

		// Defense-in-depth: verify the translated path is still under
		// a configured mount's host root. Prevents any filepath.Join
		// normalization from escaping the intended sandbox scope.
		if !isUnderMountHost(translated, c.config.SandboxMounts) {
			return "", "", errPathTraversal
		}

		slog.Debug("translated sandbox path", "from", resolvedPath, "to", translated)
		resolvedPath = translated
	}

	// 2. Validate path through the four-stage pipeline.
	resolvedPath, err = validateFilePath(resolvedPath, c.config)
	if err != nil {
		return "", "", err
	}

	// 3. Pre-check file size before reading (OOM prevention).
	fi, statErr := os.Stat(resolvedPath)
	if statErr != nil {
		return "", "", errFilePathCannotBeUsed
	}

	if fi.Size() > c.config.MaxContentSize {
		return "", "", fmt.Errorf("%w: file size %d bytes exceeds limit of %d bytes",
			errContentTooLarge, fi.Size(), c.config.MaxContentSize)
	}

	// 4. Read file content.
	data, readErr := os.ReadFile(resolvedPath) //nolint:gosec // Path already validated through validateFilePath pipeline
	if readErr != nil {
		return "", "", errFilePathCannotBeUsed
	}

	// 5. IsLikelyText check on the read data (single read to avoid TOCTOU).
	if !IsLikelyText(data) {
		return "", "", errFileNotText
	}

	content = string(data)

	// 6. Extension: extArg takes priority, otherwise detect from original file path.
	if extArg != nil && *extArg != "" {
		ext = *extArg
	} else {
		ext = strings.TrimPrefix(filepath.Ext(filePath), ".")
	}

	return content, ext, nil
}

// buildPasteResponse constructs a PasteResponse from the Wastebin API response path.
func buildPasteResponse(baseURL *url.URL, wastebinPath, ext string, passwordSet bool) *PasteResponse {
	cleanPath := strings.TrimPrefix(wastebinPath, "/")

	// Extract the ID from the path (strip the trailing extension).
	id := cleanPath
	if idx := strings.Index(cleanPath, "."); idx > 0 {
		id = cleanPath[:idx]
	}

	hostname := strings.TrimRight(baseURL.String(), "/")

	resp := &PasteResponse{
		Hostname: hostname,
		ID:       id,
		URL:      wastebinPath,
		Raw:      "/raw/" + cleanPath,
	}

	// Add markdown_rendered if extension is .md or .markdown.
	if ext == "md" || ext == "markdown" {
		resp.MarkdownRendered = "/md/" + cleanPath
	}

	// Add hint if extension is unknown (not provided).
	if ext == "" {
		resp.Hint = "Extension not detected; syntax highlighting may not apply"
	}

	// Add password hint if the paste is password-protected.
	if passwordSet {
		resp.PasswordHint = "This paste is password-protected. " +
			"Retrieve raw content via the Wastebin-Password header:\n" +
			"  curl -H 'Wastebin-Password: YOUR_PASSWORD' " + hostname + "/raw/" + cleanPath + "\n" +
			"Or as a query parameter:\n" +
			"  curl '" + hostname + "/raw/" + cleanPath + "?password=YOUR_PASSWORD'\n" +
			"(Replace YOUR_PASSWORD with the actual password.)"
	}

	return resp
}

// translateHTTPError maps HTTP status codes to user-friendly error messages.
func translateHTTPError(statusCode int, _ string) error {
	switch statusCode {
	case http.StatusForbidden:
		return errServerRejected
	case http.StatusRequestEntityTooLarge:
		return errContentTooLargeServer
	default:
		return fmt.Errorf("%w: HTTP %d", errUnknownHTTP, statusCode)
	}
}

// isConnectionError checks if the error is a connection-level error
// (connection refused, timeout, etc.).
func isConnectionError(err error) bool {
	var opErr *net.OpError
	if errors.As(err, &opErr) {
		return opErr.Op == "dial"
	}

	return false
}

// isDNSError checks if the error is a DNS resolution failure.
func isDNSError(err error) bool {
	var dnsErr *net.DNSError

	return errors.As(err, &dnsErr)
}

// closeResponseBody closes the response body with debug logging on failure.
func closeResponseBody(resp *http.Response) {
	if resp == nil || resp.Body == nil {
		return
	}

	err := resp.Body.Close()
	if err != nil {
		slog.Debug("failed to close response body", "error", err)
	}
}
