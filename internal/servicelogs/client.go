package servicelogs

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"time"
)

func UnixClient(socket string) *http.Client {
	return &http.Client{Timeout: 15 * time.Second, Transport: &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "unix", socket)
	}, MaxIdleConns: 2, IdleConnTimeout: 30 * time.Second}, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}
}

type Client struct{ http *http.Client }

func NewClient(socket string) *Client { return &Client{http: UnixClient(socket)} }
func (c *Client) Read(ctx context.Context, values url.Values) (Page, error) {
	var page Page
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://log-reader/logs?"+values.Encode(), nil)
	if err != nil {
		return page, err
	}
	response, err := c.http.Do(req)
	if err != nil {
		return page, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return page, fmt.Errorf("log reader HTTP %d", response.StatusCode)
	}
	err = json.NewDecoder(io.LimitReader(response.Body, 16<<20)).Decode(&page)
	return page, err
}
