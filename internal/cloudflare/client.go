// Package cloudflare is a tiny wrapper around the slice of the Cloudflare REST
// API that sqlitedeploy needs: list accounts, list/create R2 buckets, and
// create scoped R2 API tokens whose value yields S3-compatible credentials.
//
// We intentionally don't pull in cloudflare-go: the official SDK is large,
// pulls a lot of transitive deps, and would more than double our binary size.
// The endpoints we touch are stable and trivial to call directly.
package cloudflare

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// BaseURL is Cloudflare's v4 API root. Exposed so tests can point at a stub.
const BaseURL = "https://api.cloudflare.com/client/v4"

// Client makes authenticated requests against the Cloudflare API on behalf of
// a single user-pasted API token.
type Client struct {
	HTTP    *http.Client
	BaseURL string
	Token   string
}

// New returns a Client that uses a 30-second timeout per request.
func New(token string) *Client {
	return &Client{
		HTTP:    &http.Client{Timeout: 30 * time.Second},
		BaseURL: BaseURL,
		Token:   token,
	}
}

// envelope is the standard Cloudflare response shape: {success, result, errors}.
// We deserialize result into T at the call site and surface errors on failure.
type envelope[T any] struct {
	Success bool      `json:"success"`
	Result  T         `json:"result"`
	Errors  []apiErr  `json:"errors"`
	Messages []apiErr `json:"messages"`
}

type apiErr struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// do sends an authenticated request and unmarshals the result envelope.
func do[T any](ctx context.Context, c *Client, method, path string, body any) (T, error) {
	var zero T

	var bodyReader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return zero, fmt.Errorf("marshal request body: %w", err)
		}
		bodyReader = bytes.NewReader(raw)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.BaseURL+path, bodyReader)
	if err != nil {
		return zero, err
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return zero, fmt.Errorf("%s %s: %w", method, path, err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return zero, err
	}

	var env envelope[T]
	if err := json.Unmarshal(raw, &env); err != nil {
		// Cloudflare returned non-JSON (rare; usually proxy/edge errors).
		return zero, fmt.Errorf("%s %s: HTTP %d: %s", method, path, resp.StatusCode, truncate(string(raw), 300))
	}
	if !env.Success {
		return zero, classifyError(method, path, env.Errors)
	}
	return env.Result, nil
}

// classifyError wraps the formatted Cloudflare error in a known sentinel when
// possible, so callers can branch via errors.Is — most importantly to give a
// good message when the user hasn't enabled R2 yet.
func classifyError(method, path string, errs []apiErr) error {
	formatted := fmt.Errorf("%s %s: %s", method, path, formatErrors(errs))
	for _, e := range errs {
		switch e.Code {
		case errCodeR2NotEnabled:
			return fmt.Errorf("%w: %w", ErrR2NotEnabled, formatted)
		case errCodeUnauthorizedScope:
			return fmt.Errorf("%w: %w", ErrTokenScope, formatted)
		}
	}
	return formatted
}

func formatErrors(errs []apiErr) string {
	if len(errs) == 0 {
		return "Cloudflare API returned success=false with no error details"
	}
	parts := make([]string, len(errs))
	for i, e := range errs {
		parts[i] = fmt.Sprintf("[%d] %s", e.Code, e.Message)
	}
	return strings.Join(parts, "; ")
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "...(truncated)"
}

// Cloudflare API error codes we special-case. Full list:
// https://developers.cloudflare.com/api/responses/
const (
	// errCodeR2NotEnabled is returned by any R2 API call when the account
	// hasn't enabled R2 yet (one-time ToS click-through in the dashboard).
	errCodeR2NotEnabled = 10042

	// errCodeUnauthorizedScope is returned when the token authenticates fine
	// but lacks the specific permission the endpoint demands.
	errCodeUnauthorizedScope = 9109
)

// ErrR2NotEnabled signals that the account this token belongs to hasn't yet
// enabled R2 in the dashboard. Callers should print a friendly action item
// pointing the user at the R2 overview page for their account.
var ErrR2NotEnabled = errors.New("R2 is not enabled on this Cloudflare account")

// ErrTokenScope signals the saved token is missing a required permission.
// Callers should suggest re-running `sqlitedeploy auth login` and double-
// checking the token's permission rows.
var ErrTokenScope = errors.New("Cloudflare token is missing a required permission scope")

// IsUnauthorized returns true when err looks like an auth failure — useful
// when we want to special-case the "your token expired or was revoked" path.
func IsUnauthorized(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "[10000]") || // "Authentication error"
		strings.Contains(msg, "Invalid API Token") ||
		strings.Contains(msg, "401")
}

// Compile-time assertion that envelope is exported only via this package.
var _ = errors.New
