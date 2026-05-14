// Package zitadel wraps the Zitadel API endpoints the reconciler needs:
// JWT-bearer token exchange, user lookup, PAT minting, and PAT
// validation. The client is intentionally minimal — we only call what
// the reconciler needs, not a generic SDK.
package zitadel

import (
	"bytes"
	"context"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// MachineKey is the JSON shape Zitadel's setup-job writes for a service
// account machine key.
type MachineKey struct {
	Type     string `json:"type"`
	KeyID    string `json:"keyId"`
	Key      string `json:"key"`      // PEM-encoded RSA private key
	UserID   string `json:"userId"`
	ClientID string `json:"clientId"`
}

// Client is a minimal HTTP client for Zitadel's APIs.
type Client struct {
	BaseURL string
	HTTP    *http.Client
	token   string
}

// New returns a Client targeting baseURL.
func New(baseURL string) *Client {
	return &Client{
		BaseURL: baseURL,
		HTTP:    &http.Client{Timeout: 30 * time.Second},
	}
}

// WaitReady polls /debug/ready (Zitadel's readiness probe) until it
// returns 200 or the context expires.
func (c *Client) WaitReady(ctx context.Context) error {
	tick := time.NewTicker(2 * time.Second)
	defer tick.Stop()

	for {
		req, err := http.NewRequestWithContext(ctx, "GET", c.BaseURL+"/debug/ready", nil)
		if err != nil {
			return err
		}
		resp, err := c.HTTP.Do(req)
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == 200 {
				return nil
			}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-tick.C:
		}
	}
}

// AuthWithMachineKey signs a JWT with the machine key's RSA private key
// and exchanges it for an access token via the JWT-bearer grant. The
// resulting token is cached on the Client and used by subsequent calls.
func (c *Client) AuthWithMachineKey(ctx context.Context, mk *MachineKey, scope string) error {
	block, _ := pem.Decode([]byte(mk.Key))
	if block == nil {
		return fmt.Errorf("machine key: failed to decode PEM block")
	}
	key, err := parseRSAPrivateKey(block.Bytes)
	if err != nil {
		return fmt.Errorf("machine key: parse private key: %w", err)
	}

	now := time.Now()
	claims := jwt.MapClaims{
		"iss": mk.UserID,
		"sub": mk.UserID,
		"aud": c.BaseURL,
		"iat": now.Unix(),
		"exp": now.Add(1 * time.Hour).Unix(),
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	tok.Header["kid"] = mk.KeyID
	signed, err := tok.SignedString(key)
	if err != nil {
		return fmt.Errorf("machine key: sign JWT: %w", err)
	}

	if scope == "" {
		scope = "openid urn:zitadel:iam:org:project:id:zitadel:aud"
	}

	form := url.Values{}
	form.Set("grant_type", "urn:ietf:params:oauth:grant-type:jwt-bearer")
	form.Set("assertion", signed)
	form.Set("scope", scope)

	req, err := http.NewRequestWithContext(ctx, "POST", c.BaseURL+"/oauth/v2/token", bytes.NewBufferString(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return fmt.Errorf("token exchange failed (HTTP %d): %s", resp.StatusCode, body)
	}
	var tokenResp struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return fmt.Errorf("decode token response: %w", err)
	}
	if tokenResp.AccessToken == "" {
		return fmt.Errorf("token response had no access_token: %s", body)
	}
	c.token = tokenResp.AccessToken
	return nil
}

// ValidatePAT calls a lightweight authenticated endpoint with the
// given PAT and returns nil if it's accepted (200), an error otherwise.
// 401 specifically returns ErrPATInvalid so callers can decide to mint.
func (c *Client) ValidatePAT(ctx context.Context, pat string) error {
	req, err := http.NewRequestWithContext(ctx, "GET", c.BaseURL+"/auth/v1/users/me", nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+pat)
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == 200 {
		return nil
	}
	if resp.StatusCode == 401 || resp.StatusCode == 403 {
		return ErrPATInvalid
	}
	body, _ := io.ReadAll(resp.Body)
	return fmt.Errorf("validate PAT: HTTP %d: %s", resp.StatusCode, body)
}

// ErrPATInvalid is returned by ValidatePAT when the token is rejected.
var ErrPATInvalid = fmt.Errorf("PAT invalid")

// FindUserByLoginName looks up a Zitadel user (machine or human) by
// login username. Returns the user's ID.
func (c *Client) FindUserByLoginName(ctx context.Context, loginName string) (string, error) {
	// Zitadel v2 API: POST /v2/users/search with a loginName filter.
	body := map[string]any{
		"queries": []map[string]any{
			{"loginNameQuery": map[string]any{"loginName": loginName, "method": "TEXT_QUERY_METHOD_EQUALS"}},
		},
	}
	var resp struct {
		Result []struct {
			UserId string `json:"userId"`
		} `json:"result"`
	}
	if err := c.do(ctx, "POST", "/v2/users", body, &resp); err != nil {
		return "", err
	}
	if len(resp.Result) == 0 {
		return "", fmt.Errorf("no user found with loginName %q", loginName)
	}
	return resp.Result[0].UserId, nil
}

// MintPAT issues a new PAT for the given user and returns its token
// string. Expiration is set 10 years out (we treat them as
// re-mintable on demand, not as cryptographic time-bombs).
func (c *Client) MintPAT(ctx context.Context, userID string) (string, error) {
	expiration := time.Now().AddDate(10, 0, 0).UTC().Format(time.RFC3339)
	body := map[string]any{
		"expirationDate": expiration,
	}
	var resp struct {
		Token string `json:"token"`
	}
	path := fmt.Sprintf("/management/v1/users/%s/pats", userID)
	if err := c.do(ctx, "POST", path, body, &resp); err != nil {
		return "", err
	}
	if resp.Token == "" {
		return "", fmt.Errorf("PAT mint response had no token")
	}
	return resp.Token, nil
}

func (c *Client) do(ctx context.Context, method, path string, body any, out any) error {
	var reqBody io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reqBody = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.BaseURL+path, reqBody)
	if err != nil {
		return err
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return fmt.Errorf("%s %s: HTTP %d: %s", method, path, resp.StatusCode, raw)
	}
	if out != nil && len(raw) > 0 {
		if err := json.Unmarshal(raw, out); err != nil {
			return fmt.Errorf("decode response: %w: body=%s", err, raw)
		}
	}
	return nil
}

func parseRSAPrivateKey(der []byte) (*rsa.PrivateKey, error) {
	// PKCS#1
	if key, err := x509.ParsePKCS1PrivateKey(der); err == nil {
		return key, nil
	}
	// PKCS#8
	if key, err := x509.ParsePKCS8PrivateKey(der); err == nil {
		rsaKey, ok := key.(*rsa.PrivateKey)
		if !ok {
			return nil, fmt.Errorf("PKCS#8 key is not RSA (got %T)", key)
		}
		return rsaKey, nil
	}
	return nil, fmt.Errorf("not a recognized RSA private key encoding")
}
