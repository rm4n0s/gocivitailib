package gocivitailib_test

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	cv "github.com/rm4n0s/gocivitailib"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Every exported error must stay reachable through any number of %w wraps.
func TestErrorsAreReachableThroughWrapping(t *testing.T) {
	wrap := func(err error) error {
		return fmt.Errorf("outer: %w", fmt.Errorf("inner: %w", err))
	}

	for name, sentinel := range map[string]error{
		"ErrMissingToken":       cv.ErrMissingToken,
		"ErrNoDownloadableFile": cv.ErrNoDownloadableFile,
		"ErrInvalidResponse":    cv.ErrInvalidResponse,
		"ErrHashMismatch":       cv.ErrHashMismatch,
	} {
		t.Run(name, func(t *testing.T) {
			assert.ErrorIs(t, wrap(sentinel), sentinel)
			assert.ErrorIs(t, wrap(fmt.Errorf("%w: detail", sentinel)), sentinel)
			msg := sentinel.Error()
			assert.Equal(t, strings.ToLower(msg), msg, "messages are lower case")
			assert.False(t, strings.HasSuffix(msg, ".") || strings.HasSuffix(msg, "!"), "no trailing punctuation")
		})
	}

	t.Run("StatusError", func(t *testing.T) {
		var se *cv.StatusError
		require.ErrorAs(t, wrap(&cv.StatusError{Status: 418, Body: "teapot"}), &se)
		assert.Equal(t, 418, se.Status)
		assert.Equal(t, "teapot", se.Body)
		assert.Equal(t, "unexpected http status 418: teapot", se.Error())
		assert.Equal(t, "unexpected http status 500", (&cv.StatusError{Status: 500}).Error())
	})

	t.Run("RateLimitedError", func(t *testing.T) {
		var rl *cv.RateLimitedError
		require.ErrorAs(t, wrap(&cv.RateLimitedError{RetryAfter: 3 * time.Second}), &rl)
		assert.Equal(t, 3*time.Second, rl.RetryAfter)
		assert.Equal(t, "rate limited: retry after 3s", rl.Error())
		assert.Equal(t, "rate limited", (&cv.RateLimitedError{}).Error())
		assert.Nil(t, errors.Unwrap(&cv.RateLimitedError{}), "a hand-built value unwraps to nil, not a typed nil")
	})

	t.Run("ValidationError", func(t *testing.T) {
		var ve *cv.ValidationError
		in := &cv.ValidationError{Field: "width", Rule: cv.RuleMultipleOf16, Message: "width must be divisible by 16"}
		require.ErrorAs(t, wrap(in), &ve)
		assert.Same(t, in, ve)
		assert.Equal(t, "invalid width: width must be divisible by 16", ve.Error())
	})

	t.Run("joined ValidationErrors", func(t *testing.T) {
		a := &cv.ValidationError{Field: "a", Rule: cv.RuleRequired}
		b := &cv.ValidationError{Field: "b", Rule: cv.RuleNotAllowed}
		joined := wrap(errors.Join(a, b))
		var ve *cv.ValidationError
		require.ErrorAs(t, joined, &ve)
		assert.Same(t, a, ve)
		assert.ErrorIs(t, joined, b)
	})
}

func TestRulesAreDistinctStrings(t *testing.T) {
	rules := []string{cv.RuleRequired, cv.RuleNotAllowed, cv.RuleMultipleOf16, cv.RuleOutOfRange}
	seen := map[string]bool{}
	for _, r := range rules {
		assert.NotEmpty(t, r)
		assert.False(t, seen[r], r)
		seen[r] = true
	}
}
