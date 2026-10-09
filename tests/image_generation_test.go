package tests

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	cv "github.com/rm4n0s/gocivitailib"
	"github.com/stretchr/testify/require"
)

// TestImageGeneration talks to the real Civitai API and spends Buzz. It runs
// only when CIVITAI_TOKEN is set.
func TestImageGeneration(t *testing.T) {
	if os.Getenv(cv.EnvToken) == "" {
		t.Skipf("%s is not set", cv.EnvToken)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	client := cv.NewClient()
	w := cv.ImageWorkflowRequest{
		Steps: []cv.ImageGenStep{
			{
				Type: cv.TypeImageGen,
				Input: cv.ImageGenInput{
					Engine:         cv.EngineSdCpp,
					Ecosystem:      cv.EcosystemSDXL,
					Operation:      cv.OperationCreateImage,
					Model:          "urn:air:sdxl:checkpoint:civitai:2940478",
					Prompt:         "masterpiece, best quality, portrait, 1boy, warrior , ornate armor",
					NegativePrompt: "worst quality, low quality, blurry",
					SampleMethod:   cv.SdCppSampleEulerA,
					Steps:          25,
					CFGScale:       7.5,
					Width:          1024,
					Height:         1024,
					Seed:           1790693947,
				},
			},
		},
	}
	resp, statusCode, err := client.SubmitImageWorkflow(ctx, w)
	require.NoError(t, err)
	t.Logf("submit status: %d", statusCode)
	require.NotNil(t, resp)

	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()
	for !cv.IsTerminalStatus(resp.Status) {
		select {
		case <-ctx.Done():
			t.Fatalf("workflow %s did not finish: %v", resp.ID, ctx.Err())
		case <-ticker.C:
		}
		resp, err = client.GetImageWorkflow(ctx, resp.ID)
		require.NoError(t, err)
		t.Logf("workflow status: %s", resp.Status)
	}
	require.Equal(t, cv.WorkflowStatusSucceeded, resp.Status)

	dir := t.TempDir()
	for _, step := range resp.Steps {
		require.NotNil(t, step.Output)
		for i, img := range step.Output.Images {
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, img.URL, nil)
			require.NoError(t, err)
			r, err := http.DefaultClient.Do(req)
			require.NoError(t, err)
			dest := filepath.Join(dir, fmt.Sprintf("%d.jpg", i))
			f, err := os.Create(dest)
			require.NoError(t, err)
			_, err = io.Copy(f, r.Body)
			r.Body.Close()
			require.NoError(t, f.Close())
			require.NoError(t, err)
			t.Logf("saved %s", dest)
		}
	}
}
