package gocivitailib

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math/rand/v2"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	TypeImageGen = "imageGen"
)

type Client struct {
	token            string
	client           *http.Client
	domain           string
	siteURL          string
	orchestrationURL string
	logger           *slog.Logger
	maxRetries       int
	backoffBase      time.Duration
	backoffMax       time.Duration
}

func NewClient(opts ...Option) *Client {
	options := &options{
		token:       os.Getenv(EnvToken),
		domain:      os.Getenv(EnvDomain),
		maxRetries:  defaultMaxRetries,
		backoffBase: defaultBackoffBase,
		backoffMax:  defaultBackoffMax,
	}

	for _, opt := range opts {
		opt(options)
	}

	domain := strings.TrimSpace(options.domain)
	if domain == "" {
		domain = DefaultDomain
	}
	siteURL := strings.TrimRight(options.siteURL, "/")
	if siteURL == "" {
		siteURL = "https://" + domain
	}
	orchestrationURL := strings.TrimRight(options.orchestrationURL, "/")
	if orchestrationURL == "" {
		orchestrationURL = "https://orchestration." + domain
	}
	logger := options.logger
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}

	return &Client{
		token:            strings.TrimSpace(options.token),
		client:           withSafeRedirects(options.httpClient),
		domain:           domain,
		siteURL:          siteURL,
		orchestrationURL: orchestrationURL,
		logger:           logger,
		maxRetries:       max(options.maxRetries, 0),
		backoffBase:      options.backoffBase,
		backoffMax:       options.backoffMax,
	}
}

// newDefaultHTTPClient has dial and response-header timeouts but no overall
// timeout: callers bound a request, or a long download, with the context.
func newDefaultHTTPClient() *http.Client {
	return &http.Client{
		Transport: &http.Transport{
			Proxy: http.ProxyFromEnvironment,
			DialContext: (&net.Dialer{
				Timeout:   10 * time.Second,
				KeepAlive: 30 * time.Second,
			}).DialContext,
			ForceAttemptHTTP2:     true,
			TLSHandshakeTimeout:   10 * time.Second,
			IdleConnTimeout:       90 * time.Second,
			MaxIdleConns:          10,
			ExpectContinueTimeout: time.Second,
			// A workflow submitted with wait=60 may hold the response for up
			// to the server's 100 second request timeout.
			ResponseHeaderTimeout: 120 * time.Second,
		},
	}
}

// withSafeRedirects returns a copy of base (or a default client) whose
// redirect policy drops the Authorization header on any hop to a host other
// than the one first requested. net/http compares host names only, so by
// itself it would still forward the token to another port or subdomain.
func withSafeRedirects(base *http.Client) *http.Client {
	var c http.Client
	if base == nil {
		c = *newDefaultHTTPClient()
	} else {
		c = *base
	}
	inner := c.CheckRedirect
	c.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) > 0 && req.URL.Host != via[0].URL.Host {
			req.Header.Del("Authorization")
		}
		if inner != nil {
			return inner(req, via)
		}
		if len(via) >= 10 {
			return errors.New("stopped after 10 redirects")
		}
		return nil
	}
	return &c
}

func (c *Client) requireToken() error {
	if c.token == "" {
		return ErrMissingToken
	}
	return nil
}

// request describes one HTTP call made by do.
type request struct {
	method      string
	url         string
	body        []byte
	contentType string
	// auth sends the bearer token, when the client has one.
	auth bool
	// retry repeats the call after a 429 or 5xx. Only for idempotent calls.
	retry bool
}

// do sends rq and returns the response only when its status is 2xx; the
// caller must close the body. Any other status becomes a *StatusError, or a
// *RateLimitedError for 429. Errors carry no operation prefix: the public
// method that called do adds it once.
func (c *Client) do(ctx context.Context, rq request) (*http.Response, error) {
	retries := 0
	if rq.retry {
		retries = c.maxRetries
	}
	for attempt := 0; ; attempt++ {
		resp, err := c.attempt(ctx, rq, attempt)
		if err != nil {
			return nil, err
		}
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			return resp, nil
		}

		body, _ := io.ReadAll(io.LimitReader(resp.Body, maxErrorBody))
		resp.Body.Close()
		statusErr := &StatusError{Status: resp.StatusCode, Body: string(body)}
		retryAfter, hasRetryAfter := parseRetryAfter(resp.Header.Get("Retry-After"), time.Now())

		var final error = statusErr
		if resp.StatusCode == http.StatusTooManyRequests {
			final = &RateLimitedError{RetryAfter: retryAfter, status: statusErr}
		}
		retryable := resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500
		if !retryable || attempt >= retries {
			return nil, final
		}
		wait := c.backoff(attempt)
		if hasRetryAfter {
			if retryAfter > c.backoffMax {
				return nil, final
			}
			wait = retryAfter
		}
		c.logger.Debug("retrying request", "method", rq.method, "path", pathOf(rq.url),
			"status", resp.StatusCode, "attempt", attempt+1, "wait", wait)
		if err := sleep(ctx, wait); err != nil {
			return nil, err
		}
	}
}

func (c *Client) attempt(ctx context.Context, rq request, attempt int) (*http.Response, error) {
	var body io.Reader
	if rq.body != nil {
		body = bytes.NewReader(rq.body)
	}
	r, err := http.NewRequestWithContext(ctx, rq.method, rq.url, body)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", dropURL(err))
	}
	if rq.contentType != "" {
		r.Header.Set("Content-Type", rq.contentType)
	}
	if rq.auth && c.token != "" {
		r.Header.Set("Authorization", "Bearer "+c.token)
	}

	start := time.Now()
	resp, err := c.client.Do(r)
	if err != nil {
		c.logger.Debug("request failed", "method", rq.method, "path", r.URL.Path,
			"attempt", attempt+1, "elapsed", time.Since(start))
		return nil, fmt.Errorf("send request: %w", dropURL(err))
	}
	c.logger.Debug("response received", "method", rq.method, "path", r.URL.Path,
		"status", resp.StatusCode, "attempt", attempt+1, "elapsed", time.Since(start))
	return resp, nil
}

// backoff returns the wait before retry number attempt+1: the base doubled
// each time, capped at backoffMax, with the upper half randomised.
func (c *Client) backoff(attempt int) time.Duration {
	d := c.backoffBase
	for i := 0; i < attempt && d < c.backoffMax; i++ {
		d *= 2
	}
	d = min(d, c.backoffMax)
	if d <= 1 {
		return d
	}
	half := d / 2
	return half + rand.N(half+1)
}

func sleep(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return ctx.Err()
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// parseRetryAfter reads a Retry-After header: delay-seconds or an HTTP date.
func parseRetryAfter(v string, now time.Time) (time.Duration, bool) {
	v = strings.TrimSpace(v)
	if v == "" {
		return 0, false
	}
	if secs, err := strconv.ParseInt(v, 10, 64); err == nil {
		if secs < 0 {
			return 0, false
		}
		if secs > int64(24*time.Hour/time.Second) {
			secs = int64(24 * time.Hour / time.Second)
		}
		return time.Duration(secs) * time.Second, true
	}
	if t, err := http.ParseTime(v); err == nil {
		return max(t.Sub(now), 0), true
	}
	return 0, false
}

// dropURL strips the request URL from a *url.Error. Redirect targets can be
// signed URLs, which must not end up in error messages or logs. The cause,
// such as context.Canceled, stays reachable.
func dropURL(err error) error {
	var ue *url.Error
	if errors.As(err, &ue) {
		return ue.Err
	}
	return err
}

func pathOf(raw string) string {
	if u, err := url.Parse(raw); err == nil {
		return u.Path
	}
	return ""
}
