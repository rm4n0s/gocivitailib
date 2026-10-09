package gocivitailib

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
)

// SubmitImageWorkflow validates and submits an image generation workflow,
// waiting up to 60 seconds for it to finish. The returned int is the HTTP
// status code (200 when finished, 202 when still running), or 0 when no
// response was received. A non-2xx response is a *StatusError, or a
// *RateLimitedError for 429. Submissions are never retried, because a retry
// could spend Buzz twice.
func (c *Client) SubmitImageWorkflow(ctx context.Context, req ImageWorkflowRequest) (*ImageWorkflowResponse, int, error) {
	if err := c.requireToken(); err != nil {
		return nil, 0, fmt.Errorf("submit workflow: %w", err)
	}
	if err := req.Validate(); err != nil {
		return nil, 0, fmt.Errorf("submit workflow: %w", err)
	}
	body, err := json.Marshal(req)
	if err != nil {
		return nil, 0, fmt.Errorf("submit workflow: encode request: %w", err)
	}

	resp, err := c.do(ctx, request{
		method:      http.MethodPost,
		url:         c.orchestrationURL + "/v2/consumer/workflows?whatif=false&wait=60",
		body:        body,
		contentType: "application/json",
		auth:        true,
	})
	if err != nil {
		return nil, statusOf(err), fmt.Errorf("submit workflow: %w", err)
	}
	defer resp.Body.Close()

	result, err := decodeWorkflow(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, fmt.Errorf("submit workflow: %w", err)
	}
	return result, resp.StatusCode, nil
}

// GetImageWorkflow fetches a workflow by id. A 429 or 5xx response is retried,
// see WithMaxRetries.
func (c *Client) GetImageWorkflow(ctx context.Context, workflowID string) (*ImageWorkflowResponse, error) {
	if err := c.requireToken(); err != nil {
		return nil, fmt.Errorf("get workflow: %w", err)
	}
	if workflowID == "" {
		return nil, fmt.Errorf("get workflow: %w", &ValidationError{
			Field: "workflowID", Rule: RuleRequired, Message: "workflow id is required",
		})
	}

	resp, err := c.do(ctx, request{
		method: http.MethodGet,
		url:    c.orchestrationURL + "/v2/consumer/workflows/" + url.PathEscape(workflowID),
		auth:   true,
		retry:  true,
	})
	if err != nil {
		return nil, fmt.Errorf("get workflow: %w", err)
	}
	defer resp.Body.Close()

	wf, err := decodeWorkflow(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("get workflow: %w", err)
	}
	return wf, nil
}

func decodeWorkflow(r io.Reader) (*ImageWorkflowResponse, error) {
	var wf ImageWorkflowResponse
	if err := json.NewDecoder(r).Decode(&wf); err != nil {
		return nil, invalidResponse("decode json: %v", err)
	}
	if wf.ID == "" {
		return nil, invalidResponse("workflow has no id")
	}
	return &wf, nil
}

// statusOf returns the HTTP status carried by err, or 0.
func statusOf(err error) int {
	if se, ok := asStatusError(err); ok {
		return se.Status
	}
	return 0
}
