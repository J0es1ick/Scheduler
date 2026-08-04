package scraper

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

const MaxResponseBodyBytes int64 = 8 << 20

var ErrResponseTooLarge = errors.New("source response exceeds size limit")

func ReadLimitedBody(reader io.Reader, limit int64) ([]byte, error) {
	if limit <= 0 {
		return nil, fmt.Errorf("invalid response size limit: %d", limit)
	}
	body, err := io.ReadAll(io.LimitReader(reader, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(body)) > limit {
		return body[:limit], fmt.Errorf("%w (%d bytes)", ErrResponseTooLarge, limit)
	}
	return body, nil
}

func SameHostRedirectPolicy(maxRedirects int) func(*http.Request, []*http.Request) error {
	return func(req *http.Request, via []*http.Request) error {
		if len(via) >= maxRedirects {
			return fmt.Errorf("too many redirects (maximum %d)", maxRedirects)
		}
		if len(via) == 0 {
			return nil
		}
		origin := via[0].URL
		if req.URL.User != nil || !strings.EqualFold(req.URL.Hostname(), origin.Hostname()) {
			return fmt.Errorf("redirect to a different host is not allowed")
		}
		if strings.EqualFold(origin.Scheme, "https") && !strings.EqualFold(req.URL.Scheme, "https") {
			return fmt.Errorf("HTTPS downgrade redirect is not allowed")
		}
		return nil
	}
}
