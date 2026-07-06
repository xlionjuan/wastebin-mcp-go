package wastebin

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
)

// Handler implements the PasteCreator interface by orchestrating the
// create-paste pipeline: resolveContent, checkContentSize, parseExpires,
// buildRequest, sendRequest, buildResponse.
type Handler struct {
	baseURL    *url.URL
	httpClient *http.Client
	config     *Config
	postURL    string
}

// NewHandler creates a Handler with the given configuration.
func NewHandler(cfg *Config) (*Handler, error) {
	baseURL, err := validateServerURL(cfg)
	if err != nil {
		return nil, err
	}

	return &Handler{
		baseURL:    baseURL,
		httpClient: newHTTPClient(),
		config:     cfg,
		postURL:    baseURL.JoinPath("/").String(),
	}, nil
}

// CreatePaste implements PasteCreator. It orchestrates the full pipeline.
func (h *Handler) CreatePaste(ctx context.Context, args *CreatePasteArgs) (*PasteResponse, error) {
	content, ext, err := h.resolveContent(args)
	if err != nil {
		return nil, err
	}

	err = checkContentSize(content, h.config.MaxContentSize)
	if err != nil {
		return nil, err
	}

	expires, err := parseExpires(args.Expires, h.config.DefaultExpires)
	if err != nil {
		return nil, err
	}

	bodyBytes, err := h.buildRequest(args, content, ext, expires)
	if err != nil {
		return nil, err
	}

	wastebinResp, err := h.sendRequest(ctx, bodyBytes)
	if err != nil {
		return nil, err
	}

	return buildPasteResponse(h.baseURL, wastebinResp.Path, ext, args.Password != nil), nil
}

// resolveContent validates args and resolves paste content from either content
// mode or file mode.
//
//nolint:nonamedreturns // content and ext are both string — named disambiguates
func (h *Handler) resolveContent(args *CreatePasteArgs) (content, ext string, err error) {
	if args.Content != nil && args.FilePath != nil {
		return "", "", errBothContentAndFilePath
	}

	if args.Content == nil && args.FilePath == nil {
		return "", "", errNeitherContentNorFilePath
	}

	if args.FilePath != nil && !h.config.FileReadEnabled {
		return "", "", errFileReadDisabled
	}

	if args.Content != nil && *args.Content == "" {
		return "", "", errContentEmpty
	}

	if args.FilePath != nil {
		return h.readFileContent(*args.FilePath, args.TranslateSandboxPath, args.Extension)
	}

	return resolveContentMode(args)
}

// buildRequest assembles the HTTP request body for the Wastebin API.
func (h *Handler) buildRequest(args *CreatePasteArgs, content, ext string, expires int) ([]byte, error) {
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

		if h.baseURL.Scheme == "http" {
			if !isLoopbackHost(h.baseURL.Host) && !h.config.AllowInsecurePassword {
				return nil, errPasswordOverHTTP
			}

			slog.Warn("password is being sent over an unencrypted HTTP connection")
		}
	}

	bodyBytes, err := json.Marshal(reqBody) //nolint:gosec // JSON marshaling is safe; no user-controlled structure
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request body: %w", err)
	}

	return bodyBytes, nil
}

// sendRequest performs the HTTP POST to the Wastebin server and processes the
// response, including error translation and response validation.
func (h *Handler) sendRequest(ctx context.Context, bodyBytes []byte) (*wastebinResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, h.postURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	slog.Debug("sending paste to Wastebin", "url", h.postURL, "size", len(bodyBytes))

	resp, err := h.httpClient.Do(req)
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

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, maxErrorBodyLength)) //nolint:errcheck // read for diagnostics
		//nolint:errcheck // Best-effort drain; bounded to prevent OOM
		_, _ = io.CopyN(io.Discard, resp.Body, drainLimit)

		return nil, translateHTTPError(resp.StatusCode, string(bodyBytes))
	}

	var wastebinResp wastebinResponse

	err = json.NewDecoder(resp.Body).Decode(&wastebinResp)
	if err != nil {
		return nil, fmt.Errorf("failed to parse Wastebin response: %w", err)
	}

	err = validateWastebinResponse(wastebinResp)
	if err != nil {
		return nil, err
	}

	_, _ = io.Copy(io.Discard, resp.Body) //nolint:errcheck // Best-effort drain of body for connection reuse

	return &wastebinResp, nil
}

// readFileContent reads file content from the given path, handling sandbox
// translation, path validation, text detection, and extension detection.
//
//nolint:nonamedreturns // Both returns are string — named disambiguates.
func (h *Handler) readFileContent(
	filePath string, translateSandboxPath *bool, extArg *string,
) (content, ext string, err error) {
	resolvedPath := filePath

	// 1. Sandbox path translation (if applicable).
	if translateSandboxPath != nil && *translateSandboxPath && len(h.config.SandboxMounts) == 0 {
		return "", "", errSandboxTranslationNoMounts
	}

	if shouldTranslateSandboxPath(h.config, translateSandboxPath) {
		if hasPathTraversal(resolvedPath) {
			return "", "", errPathTraversal
		}

		if reason, blocked := hasComponentBlocked(resolvedPath); blocked && !h.config.DisableBuiltinBlocklist {
			return "", "", fmt.Errorf("%w (%s)", errBuiltinBlockedComponent, reason)
		}

		translator := NewTranslator(h.config.SandboxMounts)

		translated, ok := translator.Translate(resolvedPath)
		if !ok {
			return "", "", fmt.Errorf("%w: %s", errSandboxTranslationNoMatch, filePath)
		}

		if !isUnderMountHost(translated, h.config.SandboxMounts) {
			return "", "", errPathTraversal
		}

		slog.Debug("translated sandbox path", "from", resolvedPath, "to", translated)
		resolvedPath = translated
	}

	// 2. Validate path through the five-stage pipeline.
	resolvedPath, err = validateFilePath(resolvedPath, h.config)
	if err != nil {
		return "", "", err
	}

	// 3. Open via fd-based symlink-safe traversal, stat the fd, and read
	//    through LimitReader.
	f, openErr := openFileResolved(resolvedPath, h.config.AllowedPaths)
	if openErr != nil {
		return "", "", fmt.Errorf("%w: %w", errFilePathCannotBeUsed, openErr)
	}
	defer f.Close() //nolint:errcheck // Read-only file; close error non-critical

	fi, statErr := f.Stat()
	if statErr != nil {
		return "", "", fmt.Errorf("%w: %w", errFilePathCannotBeUsed, statErr)
	}

	if !fi.Mode().IsRegular() {
		return "", "", errFilePathCannotBeUsed
	}

	if fi.Size() > h.config.MaxContentSize {
		return "", "", fmt.Errorf("%w: file size %d bytes exceeds limit of %d bytes",
			errContentTooLarge, fi.Size(), h.config.MaxContentSize)
	}

	data, readErr := io.ReadAll(io.LimitReader(f, h.config.MaxContentSize+1))
	if readErr != nil {
		return "", "", fmt.Errorf("%w: %w", errFilePathCannotBeUsed, readErr)
	}

	if int64(len(data)) > h.config.MaxContentSize {
		return "", "", fmt.Errorf("%w: file size %d bytes exceeds limit of %d bytes",
			errContentTooLarge, len(data), h.config.MaxContentSize)
	}

	if !IsLikelyText(data) {
		return "", "", errFileNotText
	}

	content = string(data)

	// 4. Extension.
	if extArg != nil && *extArg != "" {
		ext, err = normalizeExtension(*extArg)
		if err != nil {
			return "", "", err
		}
	} else {
		ext = strings.TrimPrefix(filepath.Ext(filePath), ".")
		ext = strings.ToLower(ext)
	}

	return content, ext, nil
}

// shouldTranslateSandboxPath determines whether sandbox path translation should
// be applied.
func shouldTranslateSandboxPath(cfg *Config, requested *bool) bool {
	if len(cfg.SandboxMounts) == 0 {
		return false
	}

	return cfg.SandboxTransparent || (requested != nil && *requested)
}

// newHTTPClient creates an HTTP client with secure defaults.
func newHTTPClient() *http.Client {
	return &http.Client{
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

				if prev.URL.Scheme == "https" && req.URL.Scheme == "http" {
					return fmt.Errorf("%w: %s (%s -> %s)", errRedirectSchemeDowngrade, req.URL.Host, prev.URL.Scheme, req.URL.Scheme)
				}
			}

			return nil
		},
	}
}
