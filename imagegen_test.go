package gocivitailib_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	cv "github.com/rm4n0s/gocivitailib"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSubmitImageWorkflow(t *testing.T) {
	okBody := `{"id":"wf_1","status":"succeeded","steps":[{"name":"0","$type":"imageGen","status":"succeeded","output":{"images":[{"id":"blob_1","url":"https://example.com/a.jpg"}]}}]}`

	tests := []struct {
		name       string
		status     int
		header     http.Header
		body       string
		wantStatus int
		check      func(t *testing.T, resp *cv.ImageWorkflowResponse, err error)
	}{
		{
			name: "success", status: 200, body: okBody, wantStatus: 200,
			check: func(t *testing.T, resp *cv.ImageWorkflowResponse, err error) {
				require.NoError(t, err)
				require.NotNil(t, resp)
				assert.Equal(t, "wf_1", resp.ID)
				assert.Equal(t, cv.WorkflowStatusSucceeded, resp.Status)
				require.Len(t, resp.Steps, 1)
				require.NotNil(t, resp.Steps[0].Output)
				assert.Equal(t, "https://example.com/a.jpg", resp.Steps[0].Output.Images[0].URL)
			},
		},
		{
			name: "accepted while still processing", status: 202,
			body: `{"id":"wf_2","status":"processing","steps":[]}`, wantStatus: 202,
			check: func(t *testing.T, resp *cv.ImageWorkflowResponse, err error) {
				require.NoError(t, err)
				assert.Equal(t, cv.WorkflowStatusProcessing, resp.Status)
				assert.False(t, cv.IsTerminalStatus(resp.Status))
			},
		},
		{
			name: "bad request", status: 400, body: `{"errors":{"/input/clipSkip":["not allowed"]}}`, wantStatus: 400,
			check: func(t *testing.T, resp *cv.ImageWorkflowResponse, err error) {
				assert.Nil(t, resp)
				var se *cv.StatusError
				require.ErrorAs(t, err, &se)
				assert.Equal(t, 400, se.Status)
				assert.Contains(t, se.Body, "not allowed")
			},
		},
		{
			name: "unauthorized", status: 401, body: `{"error":"Unauthorized"}`, wantStatus: 401,
			check: func(t *testing.T, resp *cv.ImageWorkflowResponse, err error) {
				var se *cv.StatusError
				require.ErrorAs(t, err, &se)
				assert.Equal(t, 401, se.Status)
			},
		},
		{
			name: "rate limited is not retried", status: 429, header: http.Header{"Retry-After": {"7"}}, wantStatus: 429,
			check: func(t *testing.T, resp *cv.ImageWorkflowResponse, err error) {
				var rl *cv.RateLimitedError
				require.ErrorAs(t, err, &rl)
				assert.Equal(t, 7_000_000_000, int(rl.RetryAfter))
				var se *cv.StatusError
				require.ErrorAs(t, err, &se)
				assert.Equal(t, 429, se.Status)
			},
		},
		{
			name: "server error is not retried", status: 503, body: "unavailable", wantStatus: 503,
			check: func(t *testing.T, resp *cv.ImageWorkflowResponse, err error) {
				var se *cv.StatusError
				require.ErrorAs(t, err, &se)
				assert.Equal(t, 503, se.Status)
			},
		},
		{
			name: "huge error body is truncated", status: 500, body: strings.Repeat("x", 100_000), wantStatus: 500,
			check: func(t *testing.T, resp *cv.ImageWorkflowResponse, err error) {
				var se *cv.StatusError
				require.ErrorAs(t, err, &se)
				assert.LessOrEqual(t, len(se.Body), 4096)
				assert.Less(t, len(err.Error()), 400)
			},
		},
		{
			name: "malformed json", status: 200, body: `{"id":`, wantStatus: 200,
			check: func(t *testing.T, resp *cv.ImageWorkflowResponse, err error) {
				assert.Nil(t, resp)
				assert.ErrorIs(t, err, cv.ErrInvalidResponse)
			},
		},
		{
			name: "empty body", status: 200, body: ``, wantStatus: 200,
			check: func(t *testing.T, resp *cv.ImageWorkflowResponse, err error) {
				assert.ErrorIs(t, err, cv.ErrInvalidResponse)
			},
		},
		{
			name: "missing id", status: 200, body: `{"status":"succeeded"}`, wantStatus: 200,
			check: func(t *testing.T, resp *cv.ImageWorkflowResponse, err error) {
				assert.ErrorIs(t, err, cv.ErrInvalidResponse)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var hits atomic.Int32
			client, _ := newClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				hits.Add(1)
				for k, v := range tt.header {
					w.Header()[k] = v
				}
				w.WriteHeader(tt.status)
				io.WriteString(w, tt.body)
			}))
			resp, status, err := client.SubmitImageWorkflow(context.Background(), validRequest())
			assert.Equal(t, tt.wantStatus, status)
			assert.EqualValues(t, 1, hits.Load(), "a submission must be sent exactly once")
			tt.check(t, resp, err)
			if err != nil {
				assert.True(t, strings.HasPrefix(err.Error(), "submit workflow: "), err.Error())
				assert.NotContains(t, err.Error(), "submit workflow: submit workflow")
			}
		})
	}
}

func TestSubmitImageWorkflowRequest(t *testing.T) {
	var got struct {
		method, path, query, auth, contentType string
		body                                   cv.ImageWorkflowRequest
	}
	client, _ := newClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got.method, got.path, got.query = r.Method, r.URL.Path, r.URL.RawQuery
		got.auth, got.contentType = r.Header.Get("Authorization"), r.Header.Get("Content-Type")
		assert.NoError(t, json.NewDecoder(r.Body).Decode(&got.body))
		writeJSON(t, w, 200, map[string]any{"id": "wf_1", "status": "succeeded"})
	}))
	req := validRequest()
	_, _, err := client.SubmitImageWorkflow(context.Background(), req)
	require.NoError(t, err)
	assert.Equal(t, "POST", got.method)
	assert.Equal(t, "/v2/consumer/workflows", got.path)
	assert.Equal(t, "whatif=false&wait=60", got.query)
	assert.Equal(t, "Bearer "+testToken, got.auth)
	assert.Equal(t, "application/json", got.contentType)
	assert.Equal(t, req, got.body)
}

func TestSubmitImageWorkflowNeverSendsInvalidOrTokenlessRequests(t *testing.T) {
	bad := validRequest()
	bad.Steps[0].Input.ClipSkip = 2 // SDXL

	tests := []struct {
		name    string
		token   string
		req     cv.ImageWorkflowRequest
		wantIs  error
		wantAs  func(t *testing.T, err error)
		wantMsg string
	}{
		{name: "missing token", token: "", req: validRequest(), wantIs: cv.ErrMissingToken, wantMsg: "submit workflow: missing api token"},
		{name: "blank token", token: "  ", req: validRequest(), wantIs: cv.ErrMissingToken},
		{
			name: "invalid input", token: testToken, req: bad,
			wantAs: func(t *testing.T, err error) {
				var ve *cv.ValidationError
				require.ErrorAs(t, err, &ve)
				assert.Equal(t, "steps[0].clipSkip", ve.Field)
				assert.Equal(t, cv.RuleNotAllowed, ve.Rule)
			},
		},
		{
			name: "no steps", token: testToken, req: cv.ImageWorkflowRequest{},
			wantAs: func(t *testing.T, err error) {
				var ve *cv.ValidationError
				require.ErrorAs(t, err, &ve)
				assert.Equal(t, cv.RuleRequired, ve.Rule)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var hits atomic.Int32
			client, _ := newClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				hits.Add(1)
			}), cv.WithToken(tt.token))
			resp, status, err := client.SubmitImageWorkflow(context.Background(), tt.req)
			require.Error(t, err)
			assert.Nil(t, resp)
			assert.Zero(t, status)
			assert.Zero(t, hits.Load(), "no request may be sent")
			if tt.wantIs != nil {
				assert.ErrorIs(t, err, tt.wantIs)
			}
			if tt.wantAs != nil {
				tt.wantAs(t, err)
			}
			if tt.wantMsg != "" {
				assert.Equal(t, tt.wantMsg, err.Error())
			}
		})
	}
}

func TestSubmitImageWorkflowContextCancelled(t *testing.T) {
	client, _ := newClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, status, err := client.SubmitImageWorkflow(ctx, validRequest())
	assert.ErrorIs(t, err, context.Canceled)
	assert.Zero(t, status)
	assert.NotContains(t, err.Error(), "http://", "the url must not leak into errors")
}

func TestGetImageWorkflow(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		var path, auth string
		client, _ := newClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			path, auth = r.URL.EscapedPath(), r.Header.Get("Authorization")
			writeJSON(t, w, 200, map[string]any{"id": "wf_9", "status": "processing"})
		}))
		wf, err := client.GetImageWorkflow(context.Background(), "wf_9")
		require.NoError(t, err)
		assert.Equal(t, "wf_9", wf.ID)
		assert.Equal(t, "/v2/consumer/workflows/wf_9", path)
		assert.Equal(t, "Bearer "+testToken, auth)
	})

	t.Run("id is escaped", func(t *testing.T) {
		var path string
		client, _ := newClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			path = r.URL.EscapedPath()
			writeJSON(t, w, 200, map[string]any{"id": "x"})
		}))
		_, err := client.GetImageWorkflow(context.Background(), "a/b?c")
		require.NoError(t, err)
		assert.Equal(t, "/v2/consumer/workflows/a%2Fb%3Fc", path)
	})

	failures := []struct {
		name   string
		status int
		body   string
		check  func(t *testing.T, err error)
	}{
		{"not found", 404, `{"title":"Not Found"}`, func(t *testing.T, err error) {
			var se *cv.StatusError
			require.ErrorAs(t, err, &se)
			assert.Equal(t, 404, se.Status)
			assert.Contains(t, se.Body, "Not Found")
		}},
		{"malformed json", 200, `<html>`, func(t *testing.T, err error) {
			assert.ErrorIs(t, err, cv.ErrInvalidResponse)
		}},
		{"missing id", 200, `{}`, func(t *testing.T, err error) {
			assert.ErrorIs(t, err, cv.ErrInvalidResponse)
		}},
	}
	for _, tt := range failures {
		t.Run(tt.name, func(t *testing.T) {
			client, _ := newClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.status)
				io.WriteString(w, tt.body)
			}))
			wf, err := client.GetImageWorkflow(context.Background(), "wf_1")
			assert.Nil(t, wf)
			require.Error(t, err)
			tt.check(t, err)
			assert.True(t, strings.HasPrefix(err.Error(), "get workflow: "), err.Error())
		})
	}

	t.Run("cancelled context", func(t *testing.T) {
		client, _ := newClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			<-r.Context().Done()
		}))
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		_, err := client.GetImageWorkflow(ctx, "wf_1")
		assert.ErrorIs(t, err, context.Canceled)
	})

	t.Run("missing token", func(t *testing.T) {
		client, _ := newClient(t, http.NotFoundHandler(), cv.WithToken(""))
		_, err := client.GetImageWorkflow(context.Background(), "wf_1")
		assert.ErrorIs(t, err, cv.ErrMissingToken)
	})

	t.Run("empty id", func(t *testing.T) {
		client, _ := newClient(t, http.NotFoundHandler())
		_, err := client.GetImageWorkflow(context.Background(), "")
		var ve *cv.ValidationError
		require.ErrorAs(t, err, &ve)
		assert.Equal(t, cv.RuleRequired, ve.Rule)
	})

	t.Run("retries a 503 then succeeds", func(t *testing.T) {
		var hits atomic.Int32
		client, _ := newClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if hits.Add(1) == 1 {
				w.WriteHeader(503)
				return
			}
			writeJSON(t, w, 200, map[string]any{"id": "wf_1"})
		}))
		wf, err := client.GetImageWorkflow(context.Background(), "wf_1")
		require.NoError(t, err)
		assert.Equal(t, "wf_1", wf.ID)
		assert.EqualValues(t, 2, hits.Load())
	})
}

func TestLoggerNeverSeesBodiesOrSecrets(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	client, _ := newClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(t, w, 200, map[string]any{
			"id": "wf_1",
			"steps": []any{map[string]any{"output": map[string]any{
				"images": []any{map[string]any{"url": "https://signed.example/secret-signature"}},
			}}},
		})
	}), cv.WithLogger(logger))
	_, _, err := client.SubmitImageWorkflow(context.Background(), validRequest())
	require.NoError(t, err)
	_, err = client.GetImageWorkflow(context.Background(), "wf_1")
	require.NoError(t, err)

	logged := buf.String()
	assert.Contains(t, logged, "level=DEBUG")
	assert.Contains(t, logged, "status=200")
	assert.NotContains(t, logged, "secret-signature")
	assert.NotContains(t, logged, testToken)
	assert.NotContains(t, logged, "a cat", "request bodies must not be logged")
}

func TestStatusConstants(t *testing.T) {
	assert.Equal(t, "processing", cv.WorkflowStatusRunning)
	assert.Equal(t, "canceled", cv.WorkflowStatusCancelled)
	for _, s := range []string{"succeeded", "failed", "expired", "canceled"} {
		assert.True(t, cv.IsTerminalStatus(s), s)
	}
	for _, s := range []string{"unassigned", "preparing", "scheduled", "processing", "pending", ""} {
		assert.False(t, cv.IsTerminalStatus(s), s)
	}
}
