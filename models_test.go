package gocivitailib_test

import (
	"context"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	cv "github.com/rm4n0s/gocivitailib"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func fileJSON(name, url string, sizeKB float64, primary bool, sha256 string) map[string]any {
	f := map[string]any{"id": 1, "name": name, "type": "Model", "sizeKB": sizeKB, "primary": primary, "downloadUrl": url}
	if sha256 != "" {
		f["hashes"] = map[string]any{"AutoV2": "ABCDEF1234", "SHA256": sha256}
	}
	return f
}

func versionJSON(id int, files ...map[string]any) map[string]any {
	return map[string]any{
		"id": id, "name": "v" + strings.Repeat("I", id%5), "baseModel": "SDXL 1.0",
		"trainedWords": []string{"trigger one", "trigger two"}, "files": files,
	}
}

func modelJSON(id int, versions ...map[string]any) map[string]any {
	return map[string]any{
		"id": id, "name": "My LoRA", "type": "LORA", "description": "<p>desc</p>",
		"tags": []string{"anime", "style"}, "modelVersions": versions,
	}
}

func TestGetModel(t *testing.T) {
	var path, auth string
	client, _ := newClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path, auth = r.URL.Path, r.Header.Get("Authorization")
		writeJSON(t, w, 200, modelJSON(42,
			versionJSON(101, fileJSON("a.safetensors", "https://civitai.com/api/download/models/101", 144.5, true, "ABC")),
			versionJSON(102),
		))
	}))
	m, err := client.GetModel(context.Background(), 42)
	require.NoError(t, err)
	assert.Equal(t, "/api/v1/models/42", path)
	assert.Equal(t, "Bearer "+testToken, auth)

	assert.Equal(t, 42, m.ID)
	assert.Equal(t, "My LoRA", m.Name)
	assert.Equal(t, "LORA", m.Type)
	assert.Equal(t, "<p>desc</p>", m.Description)
	assert.Equal(t, []string{"anime", "style"}, m.Tags)
	require.Len(t, m.Versions, 2)
	v := m.Versions[0]
	assert.Equal(t, 101, v.ID)
	assert.Equal(t, "SDXL 1.0", v.BaseModel)
	assert.Equal(t, []string{"trigger one", "trigger two"}, v.TrainedWords)
	require.Len(t, v.Files, 1)
	f := v.Files[0]
	assert.Equal(t, "a.safetensors", f.Name)
	assert.Equal(t, 144.5, f.SizeKB)
	assert.Equal(t, "Model", f.Type)
	assert.True(t, f.Primary)
	assert.Equal(t, "https://civitai.com/api/download/models/101", f.DownloadURL)
	assert.Equal(t, "ABC", f.Hashes["SHA256"])
	assert.Equal(t, "abc", f.SHA256())
	assert.Empty(t, cv.ModelFile{}.SHA256())
}

func TestGetModelWorksWithoutToken(t *testing.T) {
	var auth string
	client, _ := newClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth = r.Header.Get("Authorization")
		writeJSON(t, w, 200, modelJSON(1))
	}), cv.WithToken(""))
	_, err := client.GetModel(context.Background(), 1)
	require.NoError(t, err)
	assert.Empty(t, auth)
}

func TestGetModelFailures(t *testing.T) {
	tests := []struct {
		name    string
		status  int
		body    string
		wantIs  error
		status2 int
	}{
		{name: "not found", status: 404, body: `{"error":"Model not found"}`, status2: 404},
		{name: "forbidden", status: 403, body: `nope`, status2: 403},
		{name: "not json", status: 200, body: `<html>login</html>`, wantIs: cv.ErrInvalidResponse},
		{name: "empty body", status: 200, body: ``, wantIs: cv.ErrInvalidResponse},
		{name: "json array", status: 200, body: `[]`, wantIs: cv.ErrInvalidResponse},
		{name: "missing id", status: 200, body: `{"name":"x"}`, wantIs: cv.ErrInvalidResponse},
		{name: "missing name", status: 200, body: `{"id":1}`, wantIs: cv.ErrInvalidResponse},
		{name: "version without id", status: 200, body: `{"id":1,"name":"x","modelVersions":[{"name":"v"}]}`, wantIs: cv.ErrInvalidResponse},
		{name: "wrong field type", status: 200, body: `{"id":"one","name":"x"}`, wantIs: cv.ErrInvalidResponse},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, _ := newClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.status)
				io.WriteString(w, tt.body)
			}))
			m, err := client.GetModel(context.Background(), 1)
			assert.Nil(t, m)
			require.Error(t, err)
			assert.True(t, strings.HasPrefix(err.Error(), "get model: "), err.Error())
			assert.NotContains(t, err.Error(), "get model: get model")
			if tt.wantIs != nil {
				assert.ErrorIs(t, err, tt.wantIs)
			}
			if tt.status2 != 0 {
				var se *cv.StatusError
				require.ErrorAs(t, err, &se)
				assert.Equal(t, tt.status2, se.Status)
				assert.NotErrorIs(t, err, cv.ErrInvalidResponse)
			}
		})
	}
}

func TestGetModelInvalidID(t *testing.T) {
	client, _ := newClient(t, http.NotFoundHandler())
	for _, id := range []int{0, -3} {
		_, err := client.GetModel(context.Background(), id)
		var ve *cv.ValidationError
		require.ErrorAs(t, err, &ve)
		assert.Equal(t, "modelID", ve.Field)
		assert.Equal(t, cv.RuleOutOfRange, ve.Rule)
	}
}

func TestGetModelRateLimiting(t *testing.T) {
	t.Run("429 followed by success", func(t *testing.T) {
		var hits atomic.Int32
		client, _ := newClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if hits.Add(1) == 1 {
				w.Header().Set("Retry-After", "0")
				w.WriteHeader(429)
				return
			}
			writeJSON(t, w, 200, modelJSON(1))
		}))
		m, err := client.GetModel(context.Background(), 1)
		require.NoError(t, err)
		assert.Equal(t, 1, m.ID)
		assert.EqualValues(t, 2, hits.Load())
	})

	t.Run("429 exhausts the retries", func(t *testing.T) {
		var hits atomic.Int32
		client, _ := newClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			hits.Add(1)
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(429)
			io.WriteString(w, "slow down")
		}), cv.WithMaxRetries(2))
		_, err := client.GetModel(context.Background(), 1)
		var rl *cv.RateLimitedError
		require.ErrorAs(t, err, &rl)
		var se *cv.StatusError
		require.ErrorAs(t, err, &se, "a rate limit also unwraps to its status error")
		assert.Equal(t, 429, se.Status)
		assert.Equal(t, "slow down", se.Body)
		assert.EqualValues(t, 3, hits.Load(), "one attempt plus two retries")
		assert.Contains(t, err.Error(), "rate limited")
	})

	t.Run("Retry-After is honoured and reported", func(t *testing.T) {
		var hits atomic.Int32
		client, _ := newClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			hits.Add(1)
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(429)
		}), cv.WithMaxRetries(1))
		start := time.Now()
		_, err := client.GetModel(context.Background(), 1)
		elapsed := time.Since(start)
		// The default test back-off cap is 20ms, so a 1s Retry-After is not
		// waited for: the error comes back at once, carrying the hint.
		var rl *cv.RateLimitedError
		require.ErrorAs(t, err, &rl)
		assert.Equal(t, time.Second, rl.RetryAfter)
		assert.EqualValues(t, 1, hits.Load())
		assert.Less(t, elapsed, 900*time.Millisecond)
	})

	t.Run("Retry-After within the cap is waited for", func(t *testing.T) {
		var hits atomic.Int32
		var firstAt, secondAt time.Time
		client, _ := newClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if hits.Add(1) == 1 {
				firstAt = time.Now()
				w.Header().Set("Retry-After", "1")
				w.WriteHeader(429)
				return
			}
			secondAt = time.Now()
			writeJSON(t, w, 200, modelJSON(1))
		}), cv.WithRetryBackoff(time.Millisecond, 5*time.Second))
		_, err := client.GetModel(context.Background(), 1)
		require.NoError(t, err)
		assert.GreaterOrEqual(t, secondAt.Sub(firstAt), 950*time.Millisecond)
	})

	t.Run("5xx is retried then reported as a status error", func(t *testing.T) {
		var hits atomic.Int32
		client, _ := newClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			hits.Add(1)
			w.WriteHeader(502)
		}), cv.WithMaxRetries(2))
		_, err := client.GetModel(context.Background(), 1)
		var se *cv.StatusError
		require.ErrorAs(t, err, &se)
		assert.Equal(t, 502, se.Status)
		var rl *cv.RateLimitedError
		assert.NotErrorAs(t, err, &rl)
		assert.EqualValues(t, 3, hits.Load())
	})

	t.Run("4xx other than 429 is not retried", func(t *testing.T) {
		var hits atomic.Int32
		client, _ := newClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			hits.Add(1)
			w.WriteHeader(404)
		}))
		_, err := client.GetModel(context.Background(), 1)
		require.Error(t, err)
		assert.EqualValues(t, 1, hits.Load())
	})

	t.Run("WithMaxRetries(0) disables retrying", func(t *testing.T) {
		var hits atomic.Int32
		client, _ := newClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			hits.Add(1)
			w.WriteHeader(429)
		}), cv.WithMaxRetries(0))
		_, err := client.GetModel(context.Background(), 1)
		var rl *cv.RateLimitedError
		require.ErrorAs(t, err, &rl)
		assert.Zero(t, rl.RetryAfter)
		assert.EqualValues(t, 1, hits.Load())
	})

	t.Run("context cancelled while backing off", func(t *testing.T) {
		var hits atomic.Int32
		client, _ := newClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			hits.Add(1)
			w.WriteHeader(503)
		}), cv.WithRetryBackoff(10*time.Second, 10*time.Second), cv.WithMaxRetries(5))
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()
		start := time.Now()
		_, err := client.GetModel(ctx, 1)
		assert.ErrorIs(t, err, context.DeadlineExceeded)
		assert.Less(t, time.Since(start), 5*time.Second)
		assert.EqualValues(t, 1, hits.Load())
	})
}
