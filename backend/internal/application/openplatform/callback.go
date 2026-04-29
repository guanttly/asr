package openplatform

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	openplatformdomain "github.com/lgt/asr/internal/domain/openplatform"
)

func (s *Service) DispatchOpenCallback(ctx context.Context, app *openplatformdomain.App, callbackURL string, payload any, extraHeaders map[string]string) ([]byte, uint16, error) {
	return s.dispatchOpenCallbackWithBackoffs(ctx, app, callbackURL, payload, extraHeaders, s.callbackRetryBackoffs)
}

func (s *Service) dispatchOpenCallbackWithBackoffs(ctx context.Context, app *openplatformdomain.App, callbackURL string, payload any, extraHeaders map[string]string, backoffs []time.Duration) ([]byte, uint16, error) {
	trimmedURL := strings.TrimSpace(callbackURL)
	if app == nil || trimmedURL == "" {
		return nil, 0, fmt.Errorf("callback target is required")
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, 0, err
	}
	secret, err := s.decryptSecret(app.AppSecretCiphertext)
	if err != nil {
		return nil, 0, err
	}
	signature := s.signCallbackBody(secret, body)
	if len(backoffs) == 0 {
		backoffs = []time.Duration{0}
	}

	var lastBody []byte
	var lastStatus uint16
	var lastErr error
	for attempt, backoff := range backoffs {
		if attempt > 0 && backoff > 0 {
			timer := time.NewTimer(backoff)
			select {
			case <-ctx.Done():
				timer.Stop()
				return lastBody, lastStatus, ctx.Err()
			case <-timer.C:
			}
		}

		request, reqErr := http.NewRequestWithContext(ctx, http.MethodPost, trimmedURL, strings.NewReader(string(body)))
		if reqErr != nil {
			return lastBody, lastStatus, reqErr
		}
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("X-OpenAPI-Signature", "hmac-sha256="+signature)
		for key, value := range extraHeaders {
			trimmedValue := strings.TrimSpace(value)
			if trimmedValue == "" {
				continue
			}
			request.Header.Set(key, trimmedValue)
		}

		response, reqErr := s.callbackHTTPClient.Do(request)
		if reqErr != nil {
			lastErr = reqErr
			continue
		}
		lastStatus = uint16(response.StatusCode)
		lastBody, _ = io.ReadAll(response.Body)
		response.Body.Close()
		if response.StatusCode >= 200 && response.StatusCode < 300 {
			return lastBody, lastStatus, nil
		}
		lastErr = fmt.Errorf("callback returned status %d", response.StatusCode)
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("callback dispatch failed")
	}
	return lastBody, lastStatus, lastErr
}

func (s *Service) encryptSecret(secret string) (string, error) {
	block, err := aes.NewCipher(s.encryptionKey())
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	sealed := gcm.Seal(nil, nonce, []byte(secret), nil)
	payload := append(nonce, sealed...)
	return base64.RawStdEncoding.EncodeToString(payload), nil
}

func (s *Service) decryptSecret(ciphertext string) (string, error) {
	trimmed := strings.TrimSpace(ciphertext)
	if trimmed == "" {
		return "", fmt.Errorf("app secret ciphertext missing")
	}
	encoded, err := base64.RawStdEncoding.DecodeString(trimmed)
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(s.encryptionKey())
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonceSize := gcm.NonceSize()
	if len(encoded) <= nonceSize {
		return "", fmt.Errorf("invalid app secret ciphertext")
	}
	plaintext, err := gcm.Open(nil, encoded[:nonceSize], encoded[nonceSize:], nil)
	if err != nil {
		return "", err
	}
	return string(plaintext), nil
}

func (s *Service) encryptionKey() []byte {
	sum := sha256.Sum256([]byte(s.platformSecret))
	return sum[:]
}

func (s *Service) signCallbackBody(secret string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}
