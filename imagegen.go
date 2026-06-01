package gocivitailib

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"

	"github.com/rm4n0s/errors"
)

func (c *Client) SubmitImageWorkflow(ctx context.Context, req ImageWorkflowRequest) (*ImageWorkflowResponse, int, error) {
	link := c.orchestrationBaseUrl() + "/v2/consumer/workflows?whatif=false&wait=60"
	fmt.Println(link)
	body, err := json.Marshal(req)
	if err != nil {
		return nil, 0, errors.NewFromErr("FailedJsonMarshal", "failed to marshal image generation's request to json", err)
	}
	r, err := http.NewRequestWithContext(ctx, "POST", link, bytes.NewReader(body))
	if err != nil {
		return nil, 0, errors.NewFromErr("FailedRequestCreation", "failed to create request for image generation", err)
	}
	r.Header.Set("Authorization", "Bearer "+c.token)
	r.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(r)
	if err != nil {
		return nil, 0, errors.NewFromErr("FailedRequestExecution", "failed to execute request for image generation", err)
	}
	defer resp.Body.Close()

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, 0, errors.NewFromErr("FailedReadAllBody", "failed to execute request for image generation", err)
	}
	log.Println(string(b))
	var result *ImageWorkflowResponse
	err = json.Unmarshal(b, result)
	if err != nil {
		return nil, resp.StatusCode, errors.NewFromErr("FailedJsonDecode", "failed to decode json to response from image generation", err)
	}
	return result, resp.StatusCode, nil
}

func (c *Client) GetImageWorkflow(ctx context.Context, workflowID string) (*ImageWorkflowResponse, error) {
	r, err := http.NewRequestWithContext(ctx, "GET", c.orchestrationBaseUrl()+"/v2/consumer/workflows/"+workflowID, nil)
	if err != nil {
		return nil, errors.NewFromErr("FailedRequestCreation", "failed to create request to get image workflow", err)
	}
	r.Header.Set("Authorization", "Bearer "+c.token)

	resp, err := c.client.Do(r)
	if err != nil {
		return nil, errors.NewFromErr("FailedRequestExecution", "failed to execute request for image generation", err)
	}
	defer resp.Body.Close()

	var wf ImageWorkflowResponse
	err = json.NewDecoder(resp.Body).Decode(&wf)
	if err != nil {
		return nil, errors.NewFromErr("FailedJsonDecode", "failed to decode json to response from image workflow", err)
	}
	return &wf, nil
}
