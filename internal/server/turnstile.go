package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// turnstileVerifyURL is Cloudflare's siteverify endpoint. Kept as a package
// var so tests can swap it for a local stub.
var turnstileVerifyURL = "https://challenges.cloudflare.com/turnstile/v0/siteverify"

// errTurnstileFailed signals that the token was rejected by Cloudflare. The
// caller maps this to a 403 in the auth flow.
var errTurnstileFailed = errors.New("turnstile: token rejected")

// turnstileResponse is the subset of fields documented in the Cloudflare
// Turnstile API we actually consume.
type turnstileResponse struct {
	Success    bool     `json:"success"`
	ErrorCodes []string `json:"error-codes"`
}

// verifyTurnstile contacts Cloudflare to validate the token. When the secret
// is empty, verification is skipped (treated as "not configured" — used in
// dev mode). Any network error is bubbled up so the caller can fail closed
// (returning 503) instead of letting unverified requests through.
func verifyTurnstile(ctx context.Context, secret, token, remoteIP string) error {
	if secret == "" {
		return nil
	}
	if token == "" {
		return errTurnstileFailed
	}
	form := url.Values{
		"secret":   {secret},
		"response": {token},
	}
	if ip := stripPort(remoteIP); ip != "" {
		form.Set("remoteip", ip)
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, turnstileVerifyURL, strings.NewReader(form.Encode()))
	if err != nil {
		return fmt.Errorf("turnstile: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("turnstile: http: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("turnstile: unexpected status %d", resp.StatusCode)
	}
	var body turnstileResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return fmt.Errorf("turnstile: decode: %w", err)
	}
	if !body.Success {
		return errTurnstileFailed
	}
	return nil
}

// stripPort takes "1.2.3.4:5678" or "[::1]:7" and returns just the host part
// because Cloudflare expects an IP without the port in remoteip.
func stripPort(addr string) string {
	if addr == "" {
		return ""
	}
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return addr
	}
	return host
}
