package telegramlimit

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"path"
	"strings"
	"time"
)

const maxTelegramErrorBody = 64 << 10

type Transport struct {
	base    http.RoundTripper
	limiter *Limiter
}

type replayReadCloser struct {
	io.Reader
	closer io.Closer
}

func (body *replayReadCloser) Close() error {
	return body.closer.Close()
}

func NewTransport(base http.RoundTripper, limiter *Limiter) *Transport {
	if base == nil {
		base = http.DefaultTransport
	}
	return &Transport{base: base, limiter: limiter}
}

func (t *Transport) RoundTrip(request *http.Request) (*http.Response, error) {
	if t.limiter != nil {
		if telegramMethod(request) == "getUpdates" {
			if err := t.limiter.wait(request.Context(), false, ""); err != nil {
				return nil, err
			}
		} else if err := t.limiter.WaitGlobal(request.Context()); err != nil {
			return nil, err
		}
	}

	response, err := t.base.RoundTrip(request)
	if err != nil || response == nil || response.StatusCode != http.StatusTooManyRequests || t.limiter == nil {
		return response, err
	}
	t.observeFloodResponse(response)
	return response, nil
}

func telegramMethod(request *http.Request) string {
	if request == nil || request.URL == nil {
		return ""
	}
	return path.Base(strings.TrimSuffix(request.URL.Path, "/"))
}

func (t *Transport) observeFloodResponse(response *http.Response) {
	if response.Body == nil {
		t.limiter.block(time.Minute)
		return
	}
	original := response.Body
	prefix, err := io.ReadAll(io.LimitReader(original, maxTelegramErrorBody))
	response.Body = &replayReadCloser{Reader: io.MultiReader(bytes.NewReader(prefix), original), closer: original}
	if err != nil {
		t.limiter.block(time.Minute)
		return
	}
	var payload struct {
		Parameters *struct {
			RetryAfter int `json:"retry_after"`
		} `json:"parameters"`
	}
	if json.Unmarshal(prefix, &payload) == nil && payload.Parameters != nil && payload.Parameters.RetryAfter >= 0 {
		t.limiter.block(time.Duration(payload.Parameters.RetryAfter+1) * time.Second)
		return
	}
	if retryAfter, parseErr := time.ParseDuration(response.Header.Get("Retry-After") + "s"); parseErr == nil {
		t.limiter.block(retryAfter + time.Second)
		return
	}
	t.limiter.block(time.Minute)
}
