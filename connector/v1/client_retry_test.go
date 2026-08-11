package v1

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

func TestClientRetriesTransientResponsesWithFreshNonce(t *testing.T) {
	var mutex sync.Mutex
	requests := 0
	nonces := map[string]bool{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mutex.Lock()
		defer mutex.Unlock()
		requests++
		nonces[r.Header.Get(HeaderNonce)] = true
		if requests < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	defer server.Close()

	publicKey, privateKey, err := GenerateKeyPair()
	if err != nil || publicKey == "" {
		t.Fatalf("generate key: %v", err)
	}
	client, err := NewClient(server.URL, "connector-test", "key-test", privateKey)
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	client.BaseDelay = time.Millisecond
	response, err := client.doSigned(context.Background(), http.MethodGet, "/test", nil, nil)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	response.Body.Close()
	if requests != 3 || len(nonces) != 3 {
		t.Fatalf("requests=%d unique nonces=%d", requests, len(nonces))
	}
}
