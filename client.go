package gocivitailib

import (
	"fmt"
	"net/http"
	"os"
)

const orchBaseURL = "https://orchestration.%s"

const (
	TypeImageGen = "imageGen"
)

type Client struct {
	token  string
	client *http.Client
	domain string
}

func NewClient(opts ...Option) *Client {
	options := &options{
		token:      os.Getenv(EnvToken),
		domain:     os.Getenv(EnvDomain),
		httpClient: &http.Client{},
	}

	for _, opt := range opts {
		opt(options)
	}
	return &Client{token: options.token, domain: options.domain, client: options.httpClient}
}

func (c *Client) orchestrationBaseUrl() string {
	return fmt.Sprintf(orchBaseURL, c.domain)
}
