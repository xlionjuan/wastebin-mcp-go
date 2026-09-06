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

	passwordSet := args.Password != nil && *args.Password != ""

	return buildPasteResponse(h.baseURL, wastebinResp.Path, ext, passwordSet), nil
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
		if *args.Password == "" {
			return nil, errPasswordEmpty
		}

		reqBody.Password = *args.Password

		if h.baseURL.Scheme == schemeHTTP {
			if !isLoopbackHost(h.baseURL.Host) && !h.config.AllowInsecurePassword {
				return nil, errPasswordOverHTTP
			}

			slog.Warn("password is being sent over an unencrypted HTTP connection")
		}
	}

	bodyBytes, err := json.Marshal(reqBody) //nolint:gosec // JSON marshaling is safe; no user-controlled structure
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request body; ask the user to check the request parameters: %w", err)
	}

	return bodyBytes, nil
}

// sendRequest performs the HTTP POST to the Wastebin server and processes the
// response, including error translation and response validation.
//
//nolint:funlen // Sequential request/response pipeline; inlining keeps error contexts local
func (h *Handler) sendRequest(ctx context.Context, bodyBytes []byte) (*wastebinResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, h.postURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP request; ask the user to check the request parameters: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	slog.Debug("sending paste to Wastebin", "url", h.postURL, "size", len(bodyBytes))

	resp, err := h.httpClient.Do(req)
	if err != nil {
		if isDNSError(err) {
			return nil, fmt.Errorf("cannot resolve the server hostname; "+
				"verify WASTEBIN_SERVER_URL points to a resolvable host: %w", err)
		}

		if isConnectionError(err) {
			return nil, fmt.Errorf("cannot connect to Wastebin server; verify the server is running: %w", err)
		}

		return nil, translateRequestError(err)
	}
	defer closeResponseBody(resp)

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, maxErrorBodyLength)) //nolint:errcheck // read for diagnostics
		//nolint:errcheck // Best-effort drain; bounded to prevent OOM
		_, _ = io.CopyN(io.Discard, resp.Body, drainLimit)

		return nil, translateHTTPError(resp.StatusCode, string(bodyBytes))
	}

	// Read through a LimitReader to prevent unbounded allocation from
	// oversized successful response bodies.
	limited := io.LimitReader(resp.Body, maxResponseBodyLength+1)

	rawBody, err := io.ReadAll(limited)
	if err != nil {
		return nil, fmt.Errorf("failed to read Wastebin response; ask the user to check the server configuration: %w", err)
	}

	if len(rawBody) > maxResponseBodyLength {
		return nil, errResponseTooLarge
	}

	var wastebinResp wastebinResponse

	decoder := json.NewDecoder(bytes.NewReader(rawBody))

	err = decoder.Decode(&wastebinResp)
	if err != nil {
		return nil, fmt.Errorf("failed to parse Wastebin response; "+
			"ask the user to check the server configuration: %w", err)
	}

	// Reject trailing non-whitespace content after the JSON object.
	_, tokErr := decoder.Token()
	if tokErr != io.EOF {
		return nil, fmt.Errorf("%w: unexpected content after JSON response", errInvalidWastebinResponse)
	}

	err = validateWastebinResponse(wastebinResp)
	if err != nil {
		return nil, err
	}

	// Best-effort bounded drain for connection reuse.
	_, _ = io.CopyN(io.Discard, resp.Body, drainLimit) //nolint:errcheck // Bound prevents OOM

	return &wastebinResp, nil
}

// wrapFilePathCannotBeUsed wraps the errFilePathCannotBeUsed sentinel with an
// underlying open/stat/read error, preserving both in the error chain for
// errors.Is matching. The wrapping format is produced here so readFileContent's
// call sites and the docs-contract fixture share one source of truth.
func wrapFilePathCannotBeUsed(underlying error) error {
	return fmt.Errorf("%w: %w", errFilePathCannotBeUsed, underlying)
}

// readFileContent reads file content from the given path, handling sandbox
// translation, path validation, text detection, and extension detection.
//
//nolint:cyclop,funlen,gocognit,gocyclo,nonamedreturns // Security pipeline; named returns disambiguate strings
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
			return "", "", newBlockedComponentError(reason)
		}

		translator := NewTranslator(h.config.SandboxMounts)

		translated, ok := translator.Translate(resolvedPath)
		if !ok {
			return "", "", newSandboxNoMatchError(filePath)
		}

		if !isUnderMountHost(translated, h.config.SandboxMounts) {
			return "", "", errPathTraversal
		}

		slog.Debug("translated sandbox path", "from", resolvedPath, "to", translated)
		resolvedPath = translated
	}

	// 2. Validate path through the six-stage pipeline.
	resolvedPath, err = validateFilePath(resolvedPath, h.config)
	if err != nil {
		return "", "", err
	}

	// 3. Open via fd-based symlink-safe traversal, stat the fd, and read
	//    through LimitReader.
	file, openErr := openFileResolved(resolvedPath, h.config.AllowedPaths)
	if openErr != nil {
		return "", "", wrapFilePathCannotBeUsed(openErr)
	}
	defer file.Close() //nolint:errcheck // Read-only file; close error non-critical

	fi, statErr := file.Stat()
	if statErr != nil {
		return "", "", wrapFilePathCannotBeUsed(statErr)
	}

	if !fi.Mode().IsRegular() {
		return "", "", errFilePathCannotBeUsed
	}

	if fi.Size() > h.config.MaxContentSize {
		return "", "", newContentTooLargeError(fi.Size(), h.config.MaxContentSize)
	}

	data, readErr := io.ReadAll(io.LimitReader(file, h.config.MaxContentSize+1))
	if readErr != nil {
		return "", "", wrapFilePathCannotBeUsed(readErr)
	}

	if int64(len(data)) > h.config.MaxContentSize {
		return "", "", newContentTooLargeError(int64(len(data)), h.config.MaxContentSize)
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
					return errRedirectDifferentHost
				}

				if prev.URL.Scheme == schemeHTTPS && req.URL.Scheme == schemeHTTP {
					return errRedirectSchemeDowngrade
				}
			}

			return nil
		},
	}
}

// translateRequestError converts transport-layer failures into
// "<problem>; <next-step guidance>" messages with dynamic values appended as
// ": <value>" suffixes. net/http wraps CheckRedirect failures in a *url.Error
// that prefixes the request URL ("Post <url>: ..."), which would place URL
// details before the problem-and-guidance text; the URL is appended as a
// ": <value>" suffix instead.
func translateRequestError(err error) error {
	var urlErr *url.Error
	if errors.As(err, &urlErr) && isRedirectError(urlErr.Err) {
		return fmt.Errorf("%w: %s", urlErr.Err, urlErr.URL)
	}

	return fmt.Errorf("HTTP request failed; ask the user to check the server URL and the network connection: %w", err)
}

// isRedirectError reports whether err is one of the redirect policy failures
// returned by the HTTP client's CheckRedirect.
func isRedirectError(err error) bool {
	return errors.Is(err, errTooManyRedirects) ||
		errors.Is(err, errRedirectDifferentHost) ||
		errors.Is(err, errRedirectSchemeDowngrade)
}
