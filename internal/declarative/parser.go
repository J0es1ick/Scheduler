package declarative

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	connector "github.com/J0es1ick/Scheduler/connector/v1"
	"github.com/J0es1ick/Scheduler/internal/managedparser"
	"github.com/J0es1ick/Scheduler/internal/scraper"
	managed "github.com/J0es1ick/Scheduler/parser/v1"
)

const AdapterType = "declarative_snapshot"

type Config struct {
	URL                string `json:"url"`
	UniversityID       string `json:"university_id"`
	UniversityName     string `json:"university_name"`
	UniversityFullName string `json:"university_full_name"`
	ScheduleURL        string `json:"schedule_url"`
	Timezone           string `json:"timezone"`
	Locale             string `json:"locale"`
}

func AdapterFactory(raw string) (scraper.SourceAdapter, error) {
	var config Config
	if err := json.Unmarshal([]byte(raw), &config); err != nil {
		return nil, fmt.Errorf("decode declarative source config: %w", err)
	}
	if err := ValidateConfig(config); err != nil {
		return nil, err
	}
	return managedparser.Factory(func() managed.Parser { return newParser(config) })(raw)
}

func ValidateConfig(config Config) error {
	parsed, err := url.ParseRequestURI(strings.TrimSpace(config.URL))
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil {
		return fmt.Errorf("declarative endpoint must be an absolute HTTPS URL without userinfo")
	}
	if strings.TrimSpace(config.UniversityID) == "" || strings.TrimSpace(config.UniversityName) == "" {
		return fmt.Errorf("declarative source university identity is required")
	}
	if _, err = time.LoadLocation(config.Timezone); err != nil {
		return fmt.Errorf("declarative source timezone: %w", err)
	}
	if forbiddenHostname(parsed.Hostname()) {
		return fmt.Errorf("declarative endpoint points to a local or private host")
	}
	return nil
}

type parser struct {
	config Config
	client *http.Client
	mu     sync.RWMutex
	groups map[string]connector.Group
}

func newParser(config Config) *parser {
	return &parser{config: config, client: safeClient(), groups: make(map[string]connector.Group)}
}

func (p *parser) Manifest() managed.Manifest {
	return managed.NormalizeManifest(managed.Manifest{
		ContractVersion: managed.ContractVersion,
		ParserID:        "declarative-" + p.config.UniversityID,
		Version:         "1.0.0",
		DisplayName:     p.config.UniversityName + " · JSON pull",
		Description:     "Scheduler загружает готовый Schedule Snapshot v1 по HTTPS.",
		Institution: connector.Institution{
			ExternalID: p.config.UniversityID, Name: p.config.UniversityName,
			FullName: p.config.UniversityFullName, ScheduleURL: p.config.ScheduleURL,
			Timezone: p.config.Timezone, Locale: p.config.Locale,
		},
		UpdateInterval: time.Hour,
	})
}

func (p *parser) FetchGroups(ctx context.Context) ([]managed.Group, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, p.config.URL, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("User-Agent", "Scheduler-Declarative-Pull/1.0")
	response, err := p.client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("fetch declarative snapshot: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 64<<10))
		return nil, fmt.Errorf("fetch declarative snapshot: HTTP %d", response.StatusCode)
	}
	body, err := scraper.ReadLimitedBody(response.Body, 16<<20)
	if err != nil {
		return nil, err
	}
	var snapshot connector.Snapshot
	if err = json.Unmarshal(body, &snapshot); err != nil {
		return nil, fmt.Errorf("decode declarative snapshot: %w", err)
	}
	if err = connector.Validate(snapshot); err != nil {
		return nil, err
	}
	if snapshot.Institution.ExternalID != p.config.UniversityID {
		return nil, fmt.Errorf("snapshot institution %q does not match configured university %q", snapshot.Institution.ExternalID, p.config.UniversityID)
	}
	groups := make([]managed.Group, 0, len(snapshot.Groups))
	lookup := make(map[string]connector.Group, len(snapshot.Groups))
	for _, group := range snapshot.Groups {
		lookup[group.ExternalID] = group
		groups = append(groups, managed.Group{ExternalID: group.ExternalID, Name: group.Name, Metadata: group.Metadata})
	}
	p.mu.Lock()
	p.groups = lookup
	p.mu.Unlock()
	return groups, nil
}

func (p *parser) FetchSchedule(_ context.Context, externalGroupID string) ([]managed.Lesson, error) {
	p.mu.RLock()
	group, ok := p.groups[externalGroupID]
	p.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("declarative snapshot does not contain group %q", externalGroupID)
	}
	return append([]managed.Lesson(nil), group.Lessons...), nil
}

func safeClient() *http.Client {
	dialer := &net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}
	transport := &http.Transport{
		Proxy:                 nil,
		TLSClientConfig:       &tls.Config{MinVersion: tls.VersionTLS12},
		ResponseHeaderTimeout: 20 * time.Second,
		DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(address)
			if err != nil {
				return nil, err
			}
			addresses, err := net.DefaultResolver.LookupNetIP(ctx, "ip", host)
			if err != nil {
				return nil, err
			}
			for _, address := range addresses {
				if forbiddenIP(net.IP(address.AsSlice())) {
					continue
				}
				return dialer.DialContext(ctx, network, net.JoinHostPort(address.String(), port))
			}
			return nil, fmt.Errorf("host %q resolves only to local or private addresses", host)
		},
	}
	return &http.Client{Timeout: 30 * time.Second, Transport: transport, CheckRedirect: scraper.SameHostRedirectPolicy(5)}
}

func forbiddenHostname(host string) bool {
	host = strings.TrimSuffix(strings.ToLower(strings.TrimSpace(host)), ".")
	if host == "localhost" || strings.HasSuffix(host, ".localhost") || host == "localhost.localdomain" {
		return true
	}
	if address := net.ParseIP(host); address != nil {
		return forbiddenIP(address)
	}
	return false
}

func forbiddenIP(address net.IP) bool {
	return address.IsLoopback() || address.IsPrivate() || address.IsLinkLocalUnicast() ||
		address.IsLinkLocalMulticast() || address.IsUnspecified() || address.IsMulticast()
}
