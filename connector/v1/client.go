package v1

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type Client struct {
	BaseURL     string
	ConnectorID string
	KeyID       string
	PrivateKey  ed25519.PrivateKey
	HTTPClient  *http.Client
	Compress    bool
	MaxAttempts int
	BaseDelay   time.Duration
}

func NewClient(baseURL, connectorID, keyID, privateKey string) (*Client, error) {
	key, err := DecodePrivateKey(privateKey)
	if err != nil {
		return nil, err
	}
	parsed, err := url.Parse(strings.TrimRight(baseURL, "/"))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return nil, fmt.Errorf("connector base URL must be absolute")
	}
	return &Client{
		BaseURL: strings.TrimRight(baseURL, "/"), ConnectorID: connectorID,
		KeyID: keyID, PrivateKey: key, HTTPClient: &http.Client{Timeout: 30 * time.Second},
		Compress: true, MaxAttempts: 4, BaseDelay: 500 * time.Millisecond,
	}, nil
}

func (c *Client) Submit(ctx context.Context, snapshot Snapshot) (*SubmissionResponse, error) {
	if err := Validate(snapshot); err != nil {
		return nil, err
	}
	raw, err := json.Marshal(snapshot)
	if err != nil {
		return nil, fmt.Errorf("encode snapshot: %w", err)
	}
	path := "/api/v1/connectors/" + url.PathEscape(c.ConnectorID) + "/snapshots"
	requestBody := raw
	contentEncoding := ""
	if c.Compress && len(raw) >= 4096 {
		var buffer bytes.Buffer
		writer := gzip.NewWriter(&buffer)
		if _, err = writer.Write(raw); err != nil {
			return nil, err
		}
		if err = writer.Close(); err != nil {
			return nil, err
		}
		requestBody = buffer.Bytes()
		contentEncoding = "gzip"
	}
	headers := make(http.Header)
	headers.Set("Content-Type", "application/json")
	if contentEncoding != "" {
		headers.Set("Content-Encoding", contentEncoding)
	}
	headers.Set("Idempotency-Key", snapshot.SnapshotID)
	response, err := c.doSigned(ctx, http.MethodPost, path, requestBody, headers)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	return decodeResponse[SubmissionResponse](response)
}

func (c *Client) Run(ctx context.Context, runID string) (*RunStatus, error) {
	path := "/api/v1/connectors/" + url.PathEscape(c.ConnectorID) + "/runs/" + url.PathEscape(runID)
	response, err := c.doSigned(ctx, http.MethodGet, path, nil, nil)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	return decodeResponse[RunStatus](response)
}

func (c *Client) Heartbeat(ctx context.Context) (*HeartbeatResponse, error) {
	path := "/api/v1/connectors/" + url.PathEscape(c.ConnectorID) + "/heartbeat"
	headers := make(http.Header)
	headers.Set("Content-Type", "application/json")
	response, err := c.doSigned(ctx, http.MethodPost, path, []byte("{}"), headers)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	return decodeResponse[HeartbeatResponse](response)
}

func (c *Client) doSigned(
	ctx context.Context,
	method, path string,
	body []byte,
	headers http.Header,
) (*http.Response, error) {
	attempts := c.MaxAttempts
	if attempts < 1 {
		attempts = 1
	}
	baseDelay := c.BaseDelay
	if baseDelay <= 0 {
		baseDelay = 500 * time.Millisecond
	}
	var lastErr error
	for attempt := 0; attempt < attempts; attempt++ {
		request, err := http.NewRequestWithContext(ctx, method, c.BaseURL+path, bytes.NewReader(body))
		if err != nil {
			return nil, err
		}
		request.Header = headers.Clone()
		if request.Header == nil {
			request.Header = make(http.Header)
		}
		now := time.Now().UTC().Format(time.RFC3339)
		nonce, nonceErr := randomNonce()
		if nonceErr != nil {
			return nil, nonceErr
		}
		digest := PayloadDigest(body)
		request.Header.Set(HeaderKeyID, c.KeyID)
		request.Header.Set(HeaderTimestamp, now)
		request.Header.Set(HeaderNonce, nonce)
		request.Header.Set(HeaderPayload, digest)
		request.Header.Set(HeaderSignature, SignRequest(c.PrivateKey, method, path, now, nonce, body))

		response, requestErr := c.httpClient().Do(request)
		retryable := requestErr != nil || response.StatusCode == http.StatusTooManyRequests || response.StatusCode >= 500
		if !retryable || attempt == attempts-1 {
			if requestErr != nil {
				return nil, requestErr
			}
			return response, nil
		}
		lastErr = requestErr
		delay := retryDelay(baseDelay, attempt, response)
		if response != nil {
			_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 64<<10))
			_ = response.Body.Close()
		}
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
	return nil, lastErr
}

func retryDelay(base time.Duration, attempt int, response *http.Response) time.Duration {
	if response != nil {
		if seconds, err := time.ParseDuration(strings.TrimSpace(response.Header.Get("Retry-After")) + "s"); err == nil && seconds > 0 {
			return seconds
		}
	}
	delay := base << attempt
	var jitter [1]byte
	if _, err := rand.Read(jitter[:]); err == nil {
		delay += time.Duration(jitter[0]%100) * time.Millisecond
	}
	if delay > 5*time.Second {
		return 5 * time.Second
	}
	return delay
}

func (c *Client) httpClient() *http.Client {
	if c.HTTPClient != nil {
		return c.HTTPClient
	}
	return &http.Client{Timeout: 30 * time.Second}
}

func randomNonce() (string, error) {
	value := make([]byte, 18)
	if _, err := rand.Read(value); err != nil {
		return "", fmt.Errorf("generate request nonce: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(value), nil
}

func decodeResponse[T any](response *http.Response) (*T, error) {
	body, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		var payload struct {
			Error string `json:"error"`
		}
		_ = json.Unmarshal(body, &payload)
		if payload.Error == "" {
			payload.Error = strings.TrimSpace(string(body))
		}
		return nil, fmt.Errorf("connector API HTTP %d: %s", response.StatusCode, payload.Error)
	}
	var result T
	if err = json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("decode connector API response: %w", err)
	}
	return &result, nil
}
