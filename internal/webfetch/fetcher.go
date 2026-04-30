// Package webfetch provides a secure web fetching and HTML-to-text extraction
// module with SSRF protection, size limits, and URL validation.
package webfetch

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// FetchMeta holds metadata about a fetched page.
type FetchMeta struct {
	SourceURL    string // original requested URL
	FinalURL     string // URL after redirects
	Title        string // extracted page title
	ContentLength int64
	StatusCode   int
	ContentType  string
	Duration     time.Duration
}

// FetchResult holds the complete fetch result.
type FetchResult struct {
	Meta    FetchMeta
	Content string // extracted text content
	HTML    []byte // raw HTML (if needed)
}

// FetcherConfig controls fetch behaviour and security policies.
type FetcherConfig struct {
	Timeout        time.Duration // default 30s
	MaxSize        int64         // max response body, default 5MB
	MaxRedirects   int           // default 5
	UserAgent      string
	RespectRobots  bool // optionally respect robots.txt
}

// DefaultConfig returns safe default fetch settings.
func DefaultConfig() FetcherConfig {
	return FetcherConfig{
		Timeout:      30 * time.Second,
		MaxSize:      5 * 1024 * 1024, // 5MB
		MaxRedirects: 5,
		UserAgent:    "mini-wiki/1.0 (research assistant)",
	}
}

// Fetcher is the web fetching interface.
type Fetcher interface {
	// FetchText fetches a URL and returns extracted text content.
	FetchText(ctx context.Context, rawURL string, cfg FetcherConfig) (*FetchResult, error)

	// ValidateURL checks that a URL is safe to fetch.
	ValidateURL(rawURL string) error

	// ExtractText parses HTML and returns readable text.
	ExtractText(htmlContent []byte) (string, string)
}

// New creates a new Fetcher.
func New() Fetcher {
	return &fetcher{
		client: &http.Client{
			Timeout: 30 * time.Second,
			Transport: &http.Transport{
				DialContext:           ssrfSafeDialContext(),
				TLSHandshakeTimeout:   10 * time.Second,
				ResponseHeaderTimeout: 10 * time.Second,
				MaxIdleConns:          2,
			},
		},
	}
}

type fetcher struct {
	client *http.Client
}

// -- URL validation --

var allowedSchemes = map[string]bool{"http": true, "https": true}
var blockedHosts = map[string]bool{
	"localhost": true, "localhost.localdomain": true, "localhost6": true,
	"127.0.0.1": true, "::1": true, "0.0.0.0": true,
}

func (f *fetcher) ValidateURL(rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("malformed URL: %w", err)
	}

	if !allowedSchemes[u.Scheme] {
		return fmt.Errorf("disallowed scheme: %s (only http/https allowed)", u.Scheme)
	}
	if u.User != nil {
		return fmt.Errorf("URL must not contain credentials")
	}
	host := strings.ToLower(u.Hostname())
	if blockedHosts[host] {
		return fmt.Errorf("URL pointing to localhost is not allowed")
	}

	return nil
}

// -- SSRF-safe dial context --

var privateCIDRs = func() []*net.IPNet {
	cidrs := []string{
		"127.0.0.0/8", "10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16",
		"169.254.0.0/16", "::1/128", "fc00::/7", "fe80::/10",
		"0.0.0.0/8", "100.64.0.0/10", "198.18.0.0/15",
	}
	parsed := make([]*net.IPNet, 0, len(cidrs))
	for _, c := range cidrs {
		_, n, err := net.ParseCIDR(c)
		if err == nil {
			parsed = append(parsed, n)
		}
	}
	return parsed
}()

func ssrfSafeDialContext() func(ctx context.Context, network, addr string) (net.Conn, error) {
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		host, _, err := net.SplitHostPort(addr)
		if err != nil {
			return nil, err
		}

		ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
		if err != nil {
			return nil, fmt.Errorf("DNS resolution failed: %w", err)
		}

		for _, ip := range ips {
			for _, cidr := range privateCIDRs {
				if cidr.Contains(ip.IP) {
					return nil, fmt.Errorf("SSRF blocked: %s resolves to private IP %s", host, ip.IP)
				}
			}
		}

		dialer := &net.Dialer{Timeout: 5 * time.Second}
		return dialer.DialContext(ctx, network, addr)
	}
}

// -- Fetch implementation --

func (f *fetcher) FetchText(ctx context.Context, rawURL string, cfg FetcherConfig) (*FetchResult, error) {
	if err := f.ValidateURL(rawURL); err != nil {
		return nil, err
	}

	// Apply config overrides
	client := f.client
	if cfg.Timeout > 0 {
		client = &http.Client{
			Timeout:          cfg.Timeout,
			Transport:        client.Transport,
			CheckRedirect:    client.CheckRedirect,
		}
	}
	if cfg.MaxRedirects > 0 {
		client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
			if len(via) >= cfg.MaxRedirects {
				return fmt.Errorf("too many redirects")
			}
			// Validate redirect URL too
			return f.ValidateURL(req.URL.String())
		}
	}

	if cfg.UserAgent == "" {
		cfg.UserAgent = "mini-wiki/1.0"
	}
	if cfg.MaxSize == 0 {
		cfg.MaxSize = 5 * 1024 * 1024
	}

	start := time.Now()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("User-Agent", cfg.UserAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch failed: %w", err)
	}
	defer resp.Body.Close()

	// Limit response size
	limited := io.LimitReader(resp.Body, cfg.MaxSize+1)
	body, err := io.ReadAll(limited)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}
	if int64(len(body)) > cfg.MaxSize {
		return nil, fmt.Errorf("response too large: max %d bytes", cfg.MaxSize)
	}

	// Extract text content
	text, title := f.ExtractText(body)

	finalURL := rawURL
	if resp.Request != nil && resp.Request.URL != nil {
		finalURL = resp.Request.URL.String()
	}

	return &FetchResult{
		Meta: FetchMeta{
			SourceURL:     rawURL,
			FinalURL:      finalURL,
			Title:         title,
			ContentLength: int64(len(body)),
			StatusCode:    resp.StatusCode,
			ContentType:   resp.Header.Get("Content-Type"),
			Duration:      time.Since(start),
		},
		Content: text,
		HTML:    body,
	}, nil
}

// -- HTML to text extraction --

func (f *fetcher) ExtractText(htmlContent []byte) (string, string) {
	doc, err := html.Parse(strings.NewReader(string(htmlContent)))
	if err != nil {
		return string(htmlContent), ""
	}

	title := extractTitle(doc)
	text := extractText(doc)
	return text, title
}

// extractText recursively walks HTML and extracts readable text.
func extractText(n *html.Node) string {
	var buf strings.Builder
	var f func(*html.Node)
	f = func(node *html.Node) {
		if node.Type == html.TextNode {
			text := strings.TrimSpace(node.Data)
			if text != "" {
				if buf.Len() > 0 {
					buf.WriteString(" ")
				}
				buf.WriteString(text)
			}
		}
		if node.Type == html.ElementNode {
			switch node.DataAtom {
			case atom.Script, atom.Style, atom.Noscript, atom.Iframe:
				return // skip these elements entirely
			case atom.P, atom.Div, atom.Br, atom.H1, atom.H2, atom.H3,
				atom.H4, atom.H5, atom.H6, atom.Li, atom.Tr, atom.Td,
				atom.Pre, atom.Blockquote, atom.Section, atom.Article:
				if buf.Len() > 0 {
					s := buf.String()
					if len(s) > 0 && s[len(s)-1] != '\n' {
						buf.WriteString("\n")
					}
				}
			}
		}
		for c := node.FirstChild; c != nil; c = c.NextSibling {
			f(c)
		}
	}
	f(n)
	return strings.TrimSpace(buf.String())
}

// extractTitle extracts the <title> tag content.
func extractTitle(doc *html.Node) string {
	var title string
	var f func(*html.Node)
	f = func(n *html.Node) {
		if n.Type == html.ElementNode && n.DataAtom == atom.Title {
			if n.FirstChild != nil && n.FirstChild.Type == html.TextNode {
				title = strings.TrimSpace(n.FirstChild.Data)
			}
			return
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			f(c)
		}
	}
	f(doc)
	return title
}
