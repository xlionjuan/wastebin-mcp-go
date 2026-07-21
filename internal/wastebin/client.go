package wastebin

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"
)

// Sentinel errors are defined in errors.go.

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
	maxErrorBodyLength    = 200

	// drainLimit is the maximum number of bytes read from error response
	// bodies. Reading more than this is unnecessary since the body is not
	// used for diagnostics, and a large body could cause OOM.
	drainLimit = 4096
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
	baseURL, err := validateServerURL(cfg)
	if err != nil {
		return nil, err
	}

	httpClient := newHTTPClient()

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
func (c *WastebinClient) CreatePaste(ctx context.Context, args *CreatePasteArgs) (*PasteResponse, error) {
	h := &Handler{
		baseURL:    c.baseURL,
		httpClient: c.httpClient,
		config:     c.config,
		postURL:    c.postURL,
	}

	return h.CreatePaste(ctx, args)
}

// checkContentSize verifies that the content does not exceed the maximum
// allowed size.
func checkContentSize(content string, maxSize int64) error {
	if int64(len(content)) > maxSize {
		return fmt.Errorf("%w: %d bytes exceeds limit of %d bytes",
			errContentTooLarge, len(content), maxSize)
	}

	return nil
}

// parseExpires parses and validates an optional expiration string, falling back
// to the configured default.
func parseExpires(expiresStr *string, defaultExpires int) (int, error) {
	expires := defaultExpires

	if expiresStr != nil && *expiresStr != "" {
		parsed, err := ParseExpiration(*expiresStr, defaultExpires)
		if err != nil {
			return 0, fmt.Errorf("invalid expiration: %w", err)
		}

		expires = parsed
	}

	ve := ValidateExpiration(expires)
	if ve != nil {
		return 0, fmt.Errorf("invalid expiration: %w", ve)
	}

	return expires, nil
}

// resolveContentMode extracts content and normalizes the extension for content
// mode pastes.
func resolveContentMode(args *CreatePasteArgs) (string, string, error) {
	content := *args.Content

	if args.Extension != nil {
		ext, err := normalizeExtension(*args.Extension)
		if err != nil {
			return "", "", err
		}

		return content, ext, nil
	}

	return content, "", nil
}

// normalizeExtension normalizes a caller-provided extension for use in Wastebin
// URLs. It trims whitespace, removes leading dots, lowercases, and validates that
// the extension does not contain characters that would create malformed URLs.
//
// Returns an empty string (no error) when the result is empty after normalization.
func normalizeExtension(ext string) (string, error) {
	normalized := strings.TrimSpace(ext)
	normalized = strings.TrimLeft(normalized, ".")
	normalized = strings.ToLower(normalized)

	if strings.ContainsAny(normalized, "/\\?#") {
		return "", fmt.Errorf("%w: %q", errInvalidExtension, ext)
	}

	return normalized, nil
}

// validateWastebinResponse checks that the Wastebin API response contains a
// valid paste path before we use it to build a PasteResponse.
func validateWastebinResponse(resp wastebinResponse) error {
	path := resp.Path

	if path == "" {
		return fmt.Errorf("%w: empty path", errInvalidWastebinResponse)
	}

	if !strings.HasPrefix(path, "/") {
		return fmt.Errorf("%w: path must be relative, got %q", errInvalidWastebinResponse, path)
	}

	clean := strings.TrimPrefix(path, "/")
	if clean == "" {
		return fmt.Errorf("%w: path is missing paste ID", errInvalidWastebinResponse)
	}

	return nil
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
			"(Replace YOUR_PASSWORD with the actual password.)"
	}

	return resp
}

// translateHTTPError maps HTTP status codes to user-friendly error messages.
func translateHTTPError(statusCode int, body string) error {
	switch statusCode {
	case http.StatusForbidden:
		return errServerRejected
	case http.StatusRequestEntityTooLarge:
		return errContentTooLargeServer
	case http.StatusUnprocessableEntity:
		if body == "" {
			return errServerValidation
		}

		if len(body) > maxErrorBodyLength {
			body = body[:maxErrorBodyLength] + "..."
		}

		return fmt.Errorf("%w: %s", errServerValidation, body)
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

// isLoopbackHost checks whether the given host (with optional port) refers
// to a loopback address. Returns true for "localhost", "127.0.0.1", "::1",
// "[::1]", and any IP in the loopback range.
func isLoopbackHost(hostPort string) bool {
	host := hostPort

	h, _, err := net.SplitHostPort(hostPort)
	if err == nil {
		host = h
	}

	// Strip brackets from bracketed IPv6 without port, e.g. "[::1]" -> "::1".
	host = strings.Trim(host, "[]")

	if strings.EqualFold(host, "localhost") {
		return true
	}

	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}

	return ip.IsLoopback()
}

// validateServerURL validates the configuration and parses the server URL,
// returning the parsed URL or an appropriate sentinel error.
func validateServerURL(cfg *Config) (*url.URL, error) {
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

	if baseURL.User != nil {
		return nil, errURLUserInfo
	}

	if baseURL.RawQuery != "" || baseURL.ForceQuery {
		return nil, errURLQuery
	}

	if baseURL.Fragment != "" || baseURL.RawFragment != "" {
		return nil, errURLFragment
	}

	return baseURL, nil
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
