package github

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

type AppAuth struct {
	appID          int64
	privateKey     *rsa.PrivateKey
	installationID int64

	mu    sync.Mutex
	token string
	expAt time.Time
}

type installationToken struct {
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expires_at"`
}

func NewAppAuth(appID int64, privateKeyPath string, installationID int64) (*AppAuth, error) {
	keyData, err := os.ReadFile(privateKeyPath)
	if err != nil {
		return nil, fmt.Errorf("read private key: %w", err)
	}

	block, _ := pem.Decode(keyData)
	if block == nil {
		return nil, fmt.Errorf("no PEM block found in %s", privateKeyPath)
	}

	key, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse private key: %w", err)
	}

	return &AppAuth{
		appID:          appID,
		privateKey:     key,
		installationID: installationID,
	}, nil
}

func (a *AppAuth) Token(ctx context.Context) (string, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.token != "" && time.Now().Before(a.expAt.Add(-5*time.Minute)) {
		return a.token, nil
	}

	jwt, err := a.generateJWT()
	if err != nil {
		return "", fmt.Errorf("generate jwt: %w", err)
	}

	tok, err := a.exchangeForInstallationToken(ctx, jwt)
	if err != nil {
		return "", fmt.Errorf("get installation token: %w", err)
	}

	a.token = tok.Token
	a.expAt = tok.ExpiresAt
	return a.token, nil
}

func (a *AppAuth) generateJWT() (string, error) {
	now := time.Now()
	header := map[string]string{"alg": "RS256", "typ": "JWT"}
	claims := map[string]any{
		"iat": now.Add(-60 * time.Second).Unix(),
		"exp": now.Add(10 * time.Minute).Unix(),
		"iss": a.appID,
	}

	headerJSON, _ := json.Marshal(header)
	claimsJSON, _ := json.Marshal(claims)

	headerB64 := base64.RawURLEncoding.EncodeToString(headerJSON)
	claimsB64 := base64.RawURLEncoding.EncodeToString(claimsJSON)

	signingInput := headerB64 + "." + claimsB64

	hash := sha256.Sum256([]byte(signingInput))
	sig, err := rsa.SignPKCS1v15(rand.Reader, a.privateKey, crypto.SHA256, hash[:])
	if err != nil {
		return "", err
	}

	sigB64 := base64.RawURLEncoding.EncodeToString(sig)
	return signingInput + "." + sigB64, nil
}

func (a *AppAuth) exchangeForInstallationToken(ctx context.Context, jwt string) (installationToken, error) {
	url := fmt.Sprintf("https://api.github.com/app/installations/%d/access_tokens", a.installationID)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, nil)
	if err != nil {
		return installationToken{}, err
	}
	req.Header.Set("Authorization", "Bearer "+jwt)
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return installationToken{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body := make([]byte, 1024)
		n, _ := resp.Body.Read(body)
		return installationToken{}, fmt.Errorf("installation token: %d: %s", resp.StatusCode, strings.TrimSpace(string(body[:n])))
	}

	var tok installationToken
	if err := json.NewDecoder(resp.Body).Decode(&tok); err != nil {
		return installationToken{}, err
	}
	return tok, nil
}
