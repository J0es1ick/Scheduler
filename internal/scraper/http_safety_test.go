package scraper

import (
	"errors"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func TestReadLimitedBodyRejectsOversizedResponse(t *testing.T) {
	body, err := ReadLimitedBody(strings.NewReader("12345"), 4)
	if !errors.Is(err, ErrResponseTooLarge) {
		t.Fatalf("error = %v, want ErrResponseTooLarge", err)
	}
	if string(body) != "1234" {
		t.Fatalf("body = %q, want bounded preview", body)
	}
}

func TestSameHostRedirectPolicy(t *testing.T) {
	policy := SameHostRedirectPolicy(5)
	origin := &http.Request{URL: mustURL(t, "http://schedule.example/path")}

	if err := policy(&http.Request{URL: mustURL(t, "https://schedule.example/next")}, []*http.Request{origin}); err != nil {
		t.Fatalf("same-host HTTPS upgrade rejected: %v", err)
	}
	if err := policy(&http.Request{URL: mustURL(t, "https://attacker.example/next")}, []*http.Request{origin}); err == nil {
		t.Fatal("cross-host redirect was allowed")
	}

	secureOrigin := &http.Request{URL: mustURL(t, "https://schedule.example/path")}
	if err := policy(&http.Request{URL: mustURL(t, "http://schedule.example/next")}, []*http.Request{secureOrigin}); err == nil {
		t.Fatal("HTTPS downgrade redirect was allowed")
	}
}

func mustURL(t *testing.T, raw string) *url.URL {
	t.Helper()
	parsed, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	return parsed
}
