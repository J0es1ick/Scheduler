package ivgpu

import (
	"io"
	"net/http"
	"strings"
	"testing"
)

type fixtureTransport struct{ body string }

func (t fixtureTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(t.body))}, nil
}
func newFixtureClient(t *testing.T, body string) *http.Client {
	t.Helper()
	return &http.Client{Transport: fixtureTransport{body: body}}
}
