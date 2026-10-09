package gocivitailib

import (
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewClientURLs(t *testing.T) {
	tests := []struct {
		name          string
		env           string
		opts          []Option
		wantSite      string
		wantOrchestra string
	}{
		{name: "defaults", wantSite: "https://civitai.com", wantOrchestra: "https://orchestration.civitai.com"},
		{name: "domain from env", env: "civitai.green", wantSite: "https://civitai.green", wantOrchestra: "https://orchestration.civitai.green"},
		{name: "WithDomain beats env", env: "civitai.green", opts: []Option{WithDomain("example.org")}, wantSite: "https://example.org", wantOrchestra: "https://orchestration.example.org"},
		{name: "blank domain falls back", opts: []Option{WithDomain("  ")}, wantSite: "https://civitai.com", wantOrchestra: "https://orchestration.civitai.com"},
		{name: "explicit urls", opts: []Option{WithSiteURL("http://127.0.0.1:1/"), WithOrchestrationURL("http://127.0.0.1:2//")}, wantSite: "http://127.0.0.1:1", wantOrchestra: "http://127.0.0.1:2"},
		{name: "explicit site url only", opts: []Option{WithSiteURL("http://s")}, wantSite: "http://s", wantOrchestra: "https://orchestration.civitai.com"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(EnvDomain, tt.env)
			c := NewClient(tt.opts...)
			assert.Equal(t, tt.wantSite, c.siteURL)
			assert.Equal(t, tt.wantOrchestra, c.orchestrationURL)
		})
	}
}

func TestNewClientToken(t *testing.T) {
	t.Setenv(EnvToken, "from-env")
	assert.NoError(t, NewClient().requireToken())
	assert.Equal(t, "explicit", NewClient(WithToken("explicit")).token)
	assert.ErrorIs(t, NewClient(WithToken("")).requireToken(), ErrMissingToken)
	assert.ErrorIs(t, NewClient(WithToken(" \t")).requireToken(), ErrMissingToken)
}

func TestDefaultHTTPClientTimeouts(t *testing.T) {
	c := NewClient().client
	assert.Zero(t, c.Timeout, "no overall timeout: the context bounds the call")
	tr, ok := c.Transport.(*http.Transport)
	require.True(t, ok)
	assert.NotNil(t, tr.DialContext)
	assert.Greater(t, tr.TLSHandshakeTimeout, time.Duration(0))
	// Must outlast a submission with wait=60.
	assert.Greater(t, tr.ResponseHeaderTimeout, 60*time.Second)
}

func TestWithHttpClientIsCopied(t *testing.T) {
	custom := &http.Client{Timeout: time.Minute}
	c := NewClient(WithHttpClient(custom))
	assert.NotSame(t, custom, c.client)
	assert.Nil(t, custom.CheckRedirect, "the caller's client must not be modified")
	assert.Equal(t, time.Minute, c.client.Timeout)
}

func TestParseRetryAfter(t *testing.T) {
	now := time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		in     string
		want   time.Duration
		wantOK bool
	}{
		{"", 0, false},
		{"abc", 0, false},
		{"-5", 0, false},
		{"0", 0, true},
		{"7", 7 * time.Second, true},
		{" 7 ", 7 * time.Second, true},
		{"99999999999999", 24 * time.Hour, true},
		{now.Add(90 * time.Second).UTC().Format(http.TimeFormat), 90 * time.Second, true},
		{now.Add(-time.Hour).UTC().Format(http.TimeFormat), 0, true},
	}
	for _, tt := range tests {
		got, ok := parseRetryAfter(tt.in, now)
		assert.Equal(t, tt.wantOK, ok, tt.in)
		assert.Equal(t, tt.want, got, tt.in)
	}
}

func TestBackoffIsBounded(t *testing.T) {
	c := NewClient(WithRetryBackoff(100*time.Millisecond, time.Second))
	for attempt, ceiling := range []time.Duration{100, 200, 400, 800, 1000, 1000, 1000} {
		ceiling *= time.Millisecond
		for range 50 {
			d := c.backoff(attempt)
			assert.GreaterOrEqual(t, d, ceiling/2, "attempt %d", attempt)
			assert.LessOrEqual(t, d, ceiling, "attempt %d", attempt)
		}
	}
	assert.Zero(t, NewClient(WithRetryBackoff(0, 0)).backoff(3))
	assert.NotPanics(t, func() { NewClient(WithRetryBackoff(1, 1)).backoff(60) })
}
