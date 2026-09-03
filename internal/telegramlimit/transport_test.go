package telegramlimit

import (
	"context"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (function roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return function(request)
}

func TestTransportLimitsEveryOutboundRequest(t *testing.T) {
	var mutex sync.Mutex
	var started []time.Time
	base := roundTripFunc(func(*http.Request) (*http.Response, error) {
		mutex.Lock()
		started = append(started, time.Now())
		mutex.Unlock()
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"ok":true}`)), Header: make(http.Header)}, nil
	})
	transport := NewTransport(base, New(25*time.Millisecond, 0))
	for range 3 {
		request, err := http.NewRequestWithContext(context.Background(), http.MethodPost, "https://api.telegram.org/bot-token/sendMessage", nil)
		if err != nil {
			t.Fatal(err)
		}
		response, err := transport.RoundTrip(request)
		if err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
	}
	if elapsed := started[len(started)-1].Sub(started[0]); elapsed < 40*time.Millisecond {
		t.Fatalf("requests were not globally spaced: %v", elapsed)
	}
}

func TestTransportDoesNotChargeGetUpdatesToOutboundBudget(t *testing.T) {
	base := roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"ok":true}`)), Header: make(http.Header)}, nil
	})
	transport := NewTransport(base, New(time.Second, 0))
	request, _ := http.NewRequest(http.MethodPost, "https://api.telegram.org/bot-token/sendMessage", nil)
	response, err := transport.RoundTrip(request)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	pollRequest, _ := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.telegram.org/bot-token/getUpdates", nil)
	response, err = transport.RoundTrip(pollRequest)
	if err != nil {
		t.Fatalf("getUpdates consumed outbound budget: %v", err)
	}
	response.Body.Close()
}

func TestTransportPreservesFloodBodyAndBlocksFollowingRequests(t *testing.T) {
	body := `{"ok":false,"error_code":429,"description":"Too Many Requests","parameters":{"retry_after":0}}`
	calls := 0
	base := roundTripFunc(func(*http.Request) (*http.Response, error) {
		calls++
		return &http.Response{StatusCode: http.StatusTooManyRequests, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
	})
	transport := NewTransport(base, New(0, 0))
	request, _ := http.NewRequest(http.MethodPost, "https://api.telegram.org/bot-token/sendMessage", nil)
	response, err := transport.RoundTrip(request)
	if err != nil {
		t.Fatal(err)
	}
	preserved, err := io.ReadAll(response.Body)
	response.Body.Close()
	if err != nil || string(preserved) != body {
		t.Fatalf("response body changed: %q, %v", preserved, err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	request, _ = http.NewRequestWithContext(ctx, http.MethodPost, "https://api.telegram.org/bot-token/sendMessage", nil)
	if _, err = transport.RoundTrip(request); err == nil {
		t.Fatal("following request ignored flood backoff")
	}
	if calls != 1 {
		t.Fatalf("blocked request reached base transport: %d calls", calls)
	}
}

func TestTransportLeavesMultipartBodyStreaming(t *testing.T) {
	bodyRead := false
	base := roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if bodyRead {
			t.Fatal("multipart body was consumed before base transport")
		}
		_, err := io.Copy(io.Discard, request.Body)
		if err != nil {
			return nil, err
		}
		bodyRead = true
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"ok":true}`)), Header: make(http.Header)}, nil
	})
	transport := NewTransport(base, New(0, 0))
	request, _ := http.NewRequest(http.MethodPost, "https://api.telegram.org/bot-token/sendPhoto", strings.NewReader("multipart-stream"))
	request.Header.Set("Content-Type", "multipart/form-data; boundary=test")
	response, err := transport.RoundTrip(request)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if !bodyRead {
		t.Fatal("base transport did not receive multipart body")
	}
}
