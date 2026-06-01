package gocivitailib

import "net/http"

const (
	EnvToken  = "CIVITAI_TOKEN"
	EnvDomain = "CIVITAI_DOMAIN"
)

type options struct {
	token      string
	domain     string
	httpClient *http.Client
}
type Option func(*options)

func WithToken(token string) Option {
	return func(opts *options) {
		opts.token = token
	}
}

func WithDomain(domain string) Option {
	return func(opts *options) {
		opts.domain = domain
	}
}

func WithHttpClient(client *http.Client) Option {
	return func(opts *options) {
		opts.httpClient = client
	}
}
