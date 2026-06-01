package tests

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path"
	"testing"
	"time"

	"github.com/joho/godotenv"
	"github.com/rm4n0s/errors"
	cv "github.com/rm4n0s/gocivitailib"
	"github.com/stretchr/testify/assert"
)

func TestImageGenera(t *testing.T) {
	err := godotenv.Load("../.env")
	assert.NoError(t, err)
	ctx := context.TODO()
	client := cv.NewClient()
	w := cv.ImageWorkflowRequest{
		Steps: []cv.ImageGenStep{
			{
				Type: cv.TypeImageGen,
				Input: cv.ImageGenInput{
					Engine:          cv.EngineSdCpp,
					Ecosystem:       cv.EcosystemSDXL,
					Operation:       cv.OperationCreateImage,
					Model:           "urn:air:sdxl:checkpoint:civitai:2940478",
					Prompt:          "masterpiece, best quality, portrait, 1boy, warrior , ornate armor",
					NegativePrompt:  "worst quality, low quality, blurry",
					Scheduler:       cv.SdCppSampleEulerA,
					Steps:           25,
					CFGScale:        7.5,
					Width:           1024,
					Height:          1024,
					Seed:            1790693947,
					ClipSkip:        2,
					DenoiseStrength: 0.8,
				},
			},
		},
	}
	resp, statusCode, err := client.SubmitImageWorkflow(ctx, w)
	log.Println(statusCode)
	assert.NoError(t, err)
	fmt.Println(err.(*errors.Error).Tag)
	assert.NotNil(t, resp)
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		resp, err = client.GetImageWorkflow(ctx, resp.ID)
		if !assert.NoError(t, err) {
			break
		}
		fmt.Println(resp.Status)
		if resp.Status == cv.WorkflowStatusPending || resp.Status == cv.WorkflowStatusRunning {
			continue
		} else {
			assert.Equal(t, resp.Status, cv.WorkflowStatusSucceeded)
			break
		}
	}
	dir := os.TempDir()
	for _, step := range resp.Steps {
		if assert.NotNil(t, step.Output) {

			for i, img := range step.Output.Images {
				resp, err := http.Get(img.URL)
				assert.NoError(t, err)
				defer resp.Body.Close()
				destPath := path.Join(dir, fmt.Sprintf("%d.jpg", i))
				f, err := os.Create(destPath)
				assert.NoError(t, err)
				_, err = io.Copy(f, resp.Body)
				assert.NoError(t, err)
				f.Close()
				fmt.Println(destPath)
			}
		}
	}

	t.Error()

}
