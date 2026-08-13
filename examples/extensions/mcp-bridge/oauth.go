// oauth.go — MCP OAuth2 flow implementation.
package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/patriceckhart/zot/packages/provider/auth"
)

// MCPAuthStore holds tokens for MCP servers.
type MCPAuthStore struct {
	path string
	mu   sync.Mutex
}

func newMCPAuthStore() *MCPAuthStore {
	return &MCPAuthStore{
		path: filepath.Join(zotHome(), "mcp-auth.json"),
	}
}

func (s *MCPAuthStore) load() (map[string]auth.OAuthToken, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	b, err := os.ReadFile(s.path)
	if os.IsNotExist(err) {
		return make(map[string]auth.OAuthToken), nil
	}
	if err != nil {
		return nil, err
	}
	var tokens map[string]auth.OAuthToken
	if err := json.Unmarshal(b, &tokens); err != nil {
		return nil, err
	}
	return tokens, nil
}

func (s *MCPAuthStore) get(name string) (*auth.OAuthToken, error) {
	tokens, err := s.load()
	if err != nil {
		return nil, err
	}
	if t, ok := tokens[name]; ok {
		return &t, nil
	}
	return nil, nil
}

func (s *MCPAuthStore) save(name string, tok auth.OAuthToken) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	tokens := make(map[string]auth.OAuthToken)
	b, err := os.ReadFile(s.path)
	if err == nil {
		_ = json.Unmarshal(b, &tokens)
	}
	tokens[name] = tok
	data, err := json.MarshalIndent(tokens, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, data, 0o600)
}

// performOAuthFlow executes the loopback OAuth2 flow.
func performOAuthFlow(ctx context.Context, logger *log.Logger, name string, cfg OAuthConfig) (*auth.OAuthToken, error) {
	state, err := randomHex(16)
	if err != nil {
		return nil, err
	}

	// 1. Find a port
	port := cfg.RedirectPort
	if port == 0 {
		l, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			return nil, fmt.Errorf("find port: %w", err)
		}
		port = l.Addr().(*net.TCPAddr).Port
		l.Close()
	}

	redirectURI := fmt.Sprintf("http://127.0.0.1:%d/callback", port)

	// 2. Start callback server
	resultChan := make(chan loginResult, 1)
	server := &http.Server{
		Addr: fmt.Sprintf("127.0.0.1:%d", port),
	}

	http.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		code := q.Get("code")
		pastedState := q.Get("state")

		if code == "" {
			fmt.Fprintf(w, "Error: missing code")
			resultChan <- loginResult{err: fmt.Errorf("missing code")}
			return
		}
		if pastedState != state {
			fmt.Fprintf(w, "Error: state mismatch")
			resultChan <- loginResult{err: fmt.Errorf("state mismatch")}
			return
		}

		fmt.Fprintf(w, "Authentication successful! You can close this window.")
		resultChan <- loginResult{code: code}
	})

	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Printf("callback server error: %v", err)
		}
	}()
	defer server.Shutdown(context.Background())

	// 3. Open browser
	authURL := fmt.Sprintf("%s?client_id=%s&redirect_uri=%s&response_type=code&state=%s",
		cfg.AuthURL, url.QueryEscape(cfg.ClientID), url.QueryEscape(redirectURI), state)
	if cfg.Scope != "" {
		authURL += "&scope=" + url.QueryEscape(cfg.Scope)
	}

	logger.Printf("[%s] opening browser for OAuth: %s", name, authURL)
	if err := openBrowser(authURL); err != nil {
		return nil, fmt.Errorf("open browser: %w", err)
	}

	// 4. Wait for code
	var code string
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case res := <-resultChan:
		if res.err != nil {
			return nil, res.err
		}
		code = res.code
	}

	// 5. Exchange code for token
	return exchangeCode(ctx, cfg, "authorization_code", code, redirectURI, "")
}

type loginResult struct {
	code string
	err  error
}

func refreshOAuthToken(ctx context.Context, cfg OAuthConfig, refreshToken string) (*auth.OAuthToken, error) {
	return exchangeCode(ctx, cfg, "refresh_token", "", "", refreshToken)
}

func exchangeCode(ctx context.Context, cfg OAuthConfig, grantType, code, redirectURI, refreshToken string) (*auth.OAuthToken, error) {
	data := url.Values{}
	data.Set("grant_type", grantType)
	data.Set("client_id", cfg.ClientID)
	data.Set("client_secret", cfg.ClientSecret)

	if grantType == "authorization_code" {
		data.Set("code", code)
		data.Set("redirect_uri", redirectURI)
	} else if grantType == "refresh_token" {
		data.Set("refresh_token", refreshToken)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", cfg.TokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("token exchange failed (status %d): %s", resp.StatusCode, string(body))
	}

	var tok auth.OAuthToken
	if err := json.NewDecoder(resp.Body).Decode(&tok); err != nil {
		return nil, err
	}
	if tok.Expiry.IsZero() && tok.RefreshToken != "" {
		// Default to something if not provided but we have a refresh token
		tok.Expiry = time.Now().Add(1 * time.Hour)
	}
	return &tok, nil
}

func randomHex(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func openBrowser(u string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", u)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", u)
	default:
		cmd = exec.Command("xdg-open", u)
	}
	return cmd.Start()
}
