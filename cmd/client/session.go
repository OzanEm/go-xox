package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

const refreshMargin = time.Minute

type tokenPairResponse struct {
	AccessToken      string    `json:"access_token"`
	AccessExpiresAt  time.Time `json:"access_expires_at"`
	RefreshToken     string    `json:"refresh_token"`
	RefreshExpiresAt time.Time `json:"refresh_expires_at"`
}

type session struct {
	mu              sync.Mutex
	access          string
	refresh         string
	accessExpiresAt time.Time
}

func newSession(p *tokenPairResponse) *session {
	s := &session{}
	s.update(p)
	return s
}

func (s *session) token() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.access
}

func (s *session) update(p *tokenPairResponse) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.access = p.AccessToken
	s.refresh = p.RefreshToken
	s.accessExpiresAt = p.AccessExpiresAt
}

func (s *session) snapshot() (refreshToken string, accessExpiresAt time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.refresh, s.accessExpiresAt
}

func (s *session) keepFresh(server string, stop <-chan struct{}) {
	for {
		refreshToken, expiresAt := s.snapshot()

		wait := time.Until(expiresAt) - refreshMargin
		if wait < 0 {
			wait = 0
		}
		timer := time.NewTimer(wait)

		select {
		case <-stop:
			timer.Stop()
			return
		case <-timer.C:
		}

		pair, err := refresh(server, refreshToken)
		if err != nil {
			fmt.Fprintln(os.Stderr, "token refresh failed:", err)
			return
		}
		s.update(pair)
	}
}

func register(server, username, password string) error {
	resp, err := postJSON(server+"/register", map[string]string{"username": username, "password": password})
	if err != nil {
		return fmt.Errorf("register request: %w", err)
	}
	defer resp.Body.Close()
	// 201 = created, 409 = already registered, both are ok
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusConflict {
		return fmt.Errorf("register failed: %s", statusBody(resp))
	}
	return nil
}

func login(server, username, password string) (*tokenPairResponse, error) {
	resp, err := postJSON(server+"/login", map[string]string{"username": username, "password": password})
	if err != nil {
		return nil, fmt.Errorf("login request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("login failed: %s", statusBody(resp))
	}
	return decodeTokenPair(resp)
}

func refresh(server, refreshToken string) (*tokenPairResponse, error) {
	resp, err := postJSON(server+"/refresh", map[string]string{"refresh_token": refreshToken})
	if err != nil {
		return nil, fmt.Errorf("refresh request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("refresh failed: %s", statusBody(resp))
	}
	return decodeTokenPair(resp)
}

func decodeTokenPair(resp *http.Response) (*tokenPairResponse, error) {
	var body tokenPairResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, fmt.Errorf("decode token response: %w", err)
	}
	if body.AccessToken == "" {
		return nil, fmt.Errorf("token response carried no access token")
	}
	return &body, nil
}

func postJSON(url string, body any) (*http.Response, error) {
	data, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	return http.Post(url, "application/json", bytes.NewReader(data))
}

func statusBody(resp *http.Response) string {
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<10))
	msg := strings.TrimSpace(string(b))
	if msg == "" {
		return resp.Status
	}
	return fmt.Sprintf("%s: %s", resp.Status, msg)
}
