package gocivitailib

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

// ModelInfo is a model as returned by GET /api/v1/models/{id}.
type ModelInfo struct {
	ID          int            `json:"id"`
	Name        string         `json:"name"`
	Type        string         `json:"type"` // "LORA", "Checkpoint", ...
	Tags        []string       `json:"tags"`
	Description string         `json:"description"`
	Versions    []ModelVersion `json:"modelVersions"`
}

// ModelVersion is one version of a model.
type ModelVersion struct {
	ID           int         `json:"id"`
	Name         string      `json:"name"`
	BaseModel    string      `json:"baseModel"`
	TrainedWords []string    `json:"trainedWords"`
	Files        []ModelFile `json:"files"`
}

// ModelFile is one downloadable file of a model version.
type ModelFile struct {
	Name        string            `json:"name"`
	SizeKB      float64           `json:"sizeKB"`
	Type        string            `json:"type"` // "Model", "Training Data", ...
	Primary     bool              `json:"primary"`
	DownloadURL string            `json:"downloadUrl"`
	Hashes      map[string]string `json:"hashes"` // AutoV2, SHA256, BLAKE3, ...
}

// SHA256 returns the file's SHA256 hash in lower case, or "" when the
// metadata has none.
func (f ModelFile) SHA256() string {
	for k, v := range f.Hashes {
		if strings.EqualFold(k, "sha256") {
			return strings.ToLower(strings.TrimSpace(v))
		}
	}
	return ""
}

// GetModel fetches a model and its versions. The endpoint is public, so no
// token is required; one is sent when the client has it. A 429 or 5xx response
// is retried, see WithMaxRetries.
func (c *Client) GetModel(ctx context.Context, modelID int) (*ModelInfo, error) {
	if modelID <= 0 {
		return nil, fmt.Errorf("get model: %w", positiveID("modelID", modelID))
	}
	resp, err := c.do(ctx, request{
		method: http.MethodGet,
		url:    c.siteURL + "/api/v1/models/" + strconv.Itoa(modelID),
		auth:   true,
		retry:  true,
	})
	if err != nil {
		return nil, fmt.Errorf("get model: %w", err)
	}
	defer resp.Body.Close()

	var m ModelInfo
	if err := json.NewDecoder(resp.Body).Decode(&m); err != nil {
		return nil, fmt.Errorf("get model: %w", invalidResponse("decode json: %v", err))
	}
	if m.ID == 0 || m.Name == "" {
		return nil, fmt.Errorf("get model: %w", invalidResponse("model has no id or name"))
	}
	for _, v := range m.Versions {
		if v.ID == 0 {
			return nil, fmt.Errorf("get model: %w", invalidResponse("model version has no id"))
		}
	}
	return &m, nil
}

func positiveID(field string, v int) error {
	return &ValidationError{Field: field, Rule: RuleOutOfRange, Message: fmt.Sprintf("%s must be positive, got %d", field, v)}
}
