package gocivitailib

import (
	"log/slog"
	"net/http"
	"time"
)

const (
	EnvToken  = "CIVITAI_TOKEN"
	EnvDomain = "CIVITAI_DOMAIN"
)

// DefaultDomain is used when neither WithDomain nor CIVITAI_DOMAIN is set.
const DefaultDomain = "civitai.com"

const (
	defaultMaxRetries  = 3
	defaultBackoffBase = 500 * time.Millisecond
	defaultBackoffMax  = 30 * time.Second
)

type options struct {
	token            string
	domain           string
	siteURL          string
	orchestrationURL string
	httpClient       *http.Client
	logger           *slog.Logger
	maxRetries       int
	backoffBase      time.Duration
	backoffMax       time.Duration
}
type Option func(*options)

func WithToken(token string) Option {
	return func(opts *options) {
		opts.token = token
	}
}

// WithDomain sets the Civitai domain, such as "civitai.com". The site and
// orchestration URLs are derived from it unless set explicitly.
func WithDomain(domain string) Option {
	return func(opts *options) {
		opts.domain = domain
	}
}

// WithHttpClient replaces the default HTTP client. The client is copied, and
// its redirect policy wrapped so that the Authorization header is never
// forwarded to another host.
func WithHttpClient(client *http.Client) Option {
	return func(opts *options) {
		opts.httpClient = client
	}
}

// WithSiteURL overrides the base URL of the Civitai site API, which defaults
// to https://<domain>. Meant for tests.
func WithSiteURL(url string) Option {
	return func(opts *options) {
		opts.siteURL = url
	}
}

// WithOrchestrationURL overrides the base URL of the orchestration API, which
// defaults to https://orchestration.<domain>. Meant for tests.
func WithOrchestrationURL(url string) Option {
	return func(opts *options) {
		opts.orchestrationURL = url
	}
}

// WithLogger enables debug logging of requests (method, path, status and
// duration; never bodies, query strings or redirect targets).
func WithLogger(logger *slog.Logger) Option {
	return func(opts *options) {
		opts.logger = logger
	}
}

// WithMaxRetries sets how many times a GET is retried after a 429 or 5xx
// response. The default is 3; zero or less disables retrying.
func WithMaxRetries(n int) Option {
	return func(opts *options) {
		opts.maxRetries = n
	}
}

// WithRetryBackoff sets the first and the largest wait between retries. The
// wait doubles on every attempt, with jitter. The defaults are 500ms and 30s.
// A Retry-After longer than maxWait stops the retrying.
func WithRetryBackoff(base, maxWait time.Duration) Option {
	return func(opts *options) {
		opts.backoffBase = base
		opts.backoffMax = maxWait
	}
}
