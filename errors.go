package gocivitailib

import (
	"errors"
	"fmt"
	"time"
)

var (
	// ErrMissingToken is returned, before any request is sent, when an
	// operation needs an API token and the client has none.
	ErrMissingToken = errors.New("missing api token")

	// ErrNoDownloadableFile is returned when a model has no version, or the
	// chosen version has no primary .safetensors file with a download URL.
	ErrNoDownloadableFile = errors.New("no downloadable file")

	// ErrInvalidResponse is returned when a response body is not valid JSON,
	// or lacks a field the client requires.
	ErrInvalidResponse = errors.New("invalid response")

	// ErrHashMismatch is returned when a downloaded file's SHA256 differs from
	// the one in the model metadata.
	ErrHashMismatch = errors.New("hash mismatch")
)

// maxErrorBody is the most response body kept in a StatusError.
const maxErrorBody = 4096

// StatusError is returned for a non-2xx HTTP response.
type StatusError struct {
	Status int
	Body   string // truncated to a few KiB
}

func (e *StatusError) Error() string {
	body := e.Body
	if len(body) > 200 {
		body = body[:200] + "..."
	}
	if body == "" {
		return fmt.Sprintf("unexpected http status %d", e.Status)
	}
	return fmt.Sprintf("unexpected http status %d: %s", e.Status, body)
}

// RateLimitedError is returned for an HTTP 429 response, after any retries
// are exhausted. RetryAfter is the server's Retry-After hint, or zero when it
// sent none. It also unwraps to the underlying *StatusError.
type RateLimitedError struct {
	RetryAfter time.Duration

	status *StatusError
}

func (e *RateLimitedError) Error() string {
	if e.RetryAfter > 0 {
		return fmt.Sprintf("rate limited: retry after %s", e.RetryAfter)
	}
	return "rate limited"
}

func (e *RateLimitedError) Unwrap() error {
	if e.status == nil {
		return nil
	}
	return e.status
}

// Rules reported in ValidationError.Rule.
const (
	RuleRequired     = "required"       // a mandatory field is empty
	RuleNotAllowed   = "not_allowed"    // field is not valid for the chosen engine or ecosystem
	RuleMultipleOf16 = "multiple_of_16" // dimension is not divisible by 16
	RuleOutOfRange   = "out_of_range"   // value is outside the permitted range
)

// ValidationError describes one rule broken by a request, before it was sent.
type ValidationError struct {
	Field   string // JSON name of the offending field, such as "clipSkip"
	Rule    string // one of the Rule constants
	Message string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("invalid %s: %s", e.Field, e.Message)
}

// invalidResponse wraps ErrInvalidResponse with a reason.
func invalidResponse(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalidResponse, fmt.Sprintf(format, args...))
}

// asStatusError finds the *StatusError in err, if any.
func asStatusError(err error) (*StatusError, bool) {
	var se *StatusError
	ok := errors.As(err, &se)
	return se, ok
}
