package gocivitailib_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	cv "github.com/rm4n0s/gocivitailib"
)

const testToken = "test-token"

// newClient starts a server running h and returns a client whose site and
// orchestration URLs both point at it, with fast retries.
func newClient(t *testing.T, h http.Handler, opts ...cv.Option) (*cv.Client, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	base := []cv.Option{
		cv.WithToken(testToken),
		cv.WithSiteURL(srv.URL),
		cv.WithOrchestrationURL(srv.URL),
		cv.WithRetryBackoff(time.Millisecond, 20*time.Millisecond),
	}
	return cv.NewClient(append(base, opts...)...), srv
}

func writeJSON(t *testing.T, w http.ResponseWriter, status int, v any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		t.Errorf("write json: %v", err)
	}
}

// validInput is an ImageGenInput that passes Validate.
func validInput() cv.ImageGenInput {
	return cv.ImageGenInput{
		Engine:    cv.EngineSdCpp,
		Ecosystem: cv.EcosystemSDXL,
		Operation: cv.OperationCreateImage,
		Model:     "urn:air:sdxl:checkpoint:civitai:1@2",
		Prompt:    "a cat",
		Width:     1024,
		Height:    1024,
	}
}

func validRequest() cv.ImageWorkflowRequest {
	return cv.ImageWorkflowRequest{Steps: []cv.ImageGenStep{{Type: cv.TypeImageGen, Input: validInput()}}}
}
