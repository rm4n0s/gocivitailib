package gocivitailib

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// DownloadFile streams the file at downloadURL into w and returns the number
// of bytes written. The bearer token is sent only when downloadURL is on the
// site host (see WithSiteURL), and is dropped again if a redirect leads to
// another host. A 429 or 5xx response is retried before streaming starts; once
// bytes have been written to w, a failure is not retried.
func (c *Client) DownloadFile(ctx context.Context, downloadURL string, w io.Writer) (int64, error) {
	n, err := c.download(ctx, downloadURL, w)
	if err != nil {
		return n, fmt.Errorf("download file: %w", err)
	}
	return n, nil
}

// DownloadModelVersion streams the primary file of a model version into w
// and returns the number of bytes written. It needs a token.
func (c *Client) DownloadModelVersion(ctx context.Context, versionID int, w io.Writer) (int64, error) {
	if versionID <= 0 {
		return 0, fmt.Errorf("download model version: %w", positiveID("versionID", versionID))
	}
	n, err := c.download(ctx, c.versionDownloadURL(versionID), w)
	if err != nil {
		return n, fmt.Errorf("download model version: %w", err)
	}
	return n, nil
}

func (c *Client) versionDownloadURL(versionID int) string {
	return c.siteURL + "/api/download/models/" + strconv.Itoa(versionID)
}

// download is DownloadFile without the operation prefix.
func (c *Client) download(ctx context.Context, downloadURL string, w io.Writer) (int64, error) {
	u, err := url.Parse(downloadURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return 0, &ValidationError{Field: "downloadURL", Rule: RuleRequired, Message: "download url must be an absolute http or https url"}
	}
	// Civitai wants the token for every download, but another host must never
	// get it.
	auth := c.isSiteHost(u)
	if auth {
		if err := c.requireToken(); err != nil {
			return 0, err
		}
	}

	resp, err := c.do(ctx, request{method: http.MethodGet, url: downloadURL, auth: auth, retry: true})
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	// A login or error page is not a model.
	if ct := strings.ToLower(resp.Header.Get("Content-Type")); strings.HasPrefix(ct, "text/html") {
		return 0, invalidResponse("download returned %s instead of a file", ct)
	}

	n, err := io.Copy(w, resp.Body)
	if err != nil {
		return n, fmt.Errorf("copy body: %w", dropURL(err))
	}
	return n, nil
}

func (c *Client) isSiteHost(u *url.URL) bool {
	site, err := url.Parse(c.siteURL)
	return err == nil && strings.EqualFold(site.Host, u.Host)
}

// DownloadOption customises DownloadModel.
type DownloadOption func(*downloadOptions)

type downloadOptions struct {
	versionID int
}

// WithVersionID selects a model version instead of the newest.
func WithVersionID(versionID int) DownloadOption {
	return func(o *downloadOptions) { o.versionID = versionID }
}

// DownloadedModel is the result of DownloadModel.
type DownloadedModel struct {
	ModelPath    string       // <dir>/<modelID>.safetensors
	MetadataPath string       // <dir>/<modelID>.json
	Info         *ModelInfo   // the model as fetched
	Version      ModelVersion // the version that was chosen
	File         ModelFile    // the file that was chosen
	Size         int64        // size of ModelPath in bytes
	Skipped      bool         // the file was already in place, nothing was downloaded
}

// ModelMetadata is the content of the <modelID>.json file written next to
// the model.
type ModelMetadata struct {
	Name         string   `json:"name"`
	BaseModel    string   `json:"baseModel"`
	TriggerWords []string `json:"triggerWords"`
	Tags         []string `json:"tags"`
	Description  string   `json:"description"`
	VersionID    int      `json:"versionId"`
}

// DownloadModel fetches a model's metadata and stores one version of it in
// dir as <modelID>.safetensors, next to <modelID>.json (see ModelMetadata).
//
// The newest version (the one with the highest id) is used unless WithVersionID
// says otherwise, and from it the primary .safetensors file, falling back to
// the first .safetensors file of type "Model" when none is marked primary.
// ErrNoDownloadableFile is returned when there is none. An existing
// <modelID>.safetensors of the expected size is kept and not downloaded again.
// Otherwise the file is streamed to a temporary file in dir, checked against the
// SHA256 of the metadata when there is one (ErrHashMismatch), and renamed into
// place. Temporary files are removed on every failure.
func (c *Client) DownloadModel(ctx context.Context, modelID int, dir string, opts ...DownloadOption) (*DownloadedModel, error) {
	var o downloadOptions
	for _, opt := range opts {
		opt(&o)
	}
	res, err := c.downloadModel(ctx, modelID, dir, o)
	if err != nil {
		return nil, fmt.Errorf("download model: %w", err)
	}
	return res, nil
}

func (c *Client) downloadModel(ctx context.Context, modelID int, dir string, o downloadOptions) (*DownloadedModel, error) {
	if modelID <= 0 {
		return nil, positiveID("modelID", modelID)
	}
	if dir == "" {
		return nil, &ValidationError{Field: "dir", Rule: RuleRequired, Message: "directory is required"}
	}
	if err := c.requireToken(); err != nil {
		return nil, err
	}

	info, err := c.GetModel(ctx, modelID)
	if err != nil {
		return nil, err
	}
	version, file, err := chooseFile(info, o.versionID)
	if err != nil {
		return nil, err
	}

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create directory: %w", err)
	}
	res := &DownloadedModel{
		ModelPath:    filepath.Join(dir, fmt.Sprintf("%d.safetensors", modelID)),
		MetadataPath: filepath.Join(dir, fmt.Sprintf("%d.json", modelID)),
		Info:         info,
		Version:      version,
		File:         file,
	}

	if st, err := os.Stat(res.ModelPath); err == nil && st.Mode().IsRegular() && sizeMatches(st.Size(), file.SizeKB) {
		res.Skipped = true
		res.Size = st.Size()
	} else if err := c.fetchInto(ctx, res, modelID, dir); err != nil {
		return nil, err
	}

	meta := ModelMetadata{
		Name:         info.Name,
		BaseModel:    version.BaseModel,
		TriggerWords: nonNil(version.TrainedWords),
		Tags:         nonNil(info.Tags),
		Description:  info.Description,
		VersionID:    version.ID,
	}
	if err := writeJSONFile(res.MetadataPath, meta); err != nil {
		return nil, fmt.Errorf("write metadata: %w", err)
	}
	return res, nil
}

// fetchInto downloads res.File to a temporary file in dir, verifies it and
// moves it to res.ModelPath.
func (c *Client) fetchInto(ctx context.Context, res *DownloadedModel, modelID int, dir string) (err error) {
	tmp, err := os.CreateTemp(dir, fmt.Sprintf(".%d-*.safetensors.tmp", modelID))
	if err != nil {
		return fmt.Errorf("create temporary file: %w", err)
	}
	defer func() {
		if err != nil {
			tmp.Close()
			os.Remove(tmp.Name())
		}
	}()

	h := sha256.New()
	n, err := c.download(ctx, res.File.DownloadURL, io.MultiWriter(tmp, h))
	if err != nil {
		return err
	}
	if want := res.File.SHA256(); want != "" {
		if got := hex.EncodeToString(h.Sum(nil)); got != want {
			return fmt.Errorf("%w: expected sha256 %s, got %s", ErrHashMismatch, want, got)
		}
	}
	if err := tmp.Sync(); err != nil {
		return fmt.Errorf("sync temporary file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temporary file: %w", err)
	}
	if err := os.Chmod(tmp.Name(), 0o644); err != nil {
		return fmt.Errorf("set file mode: %w", err)
	}
	if err := os.Rename(tmp.Name(), res.ModelPath); err != nil {
		return fmt.Errorf("move file into place: %w", err)
	}
	res.Size = n
	return nil
}

// chooseFile picks the version (the given one, or the newest) and its file.
func chooseFile(info *ModelInfo, versionID int) (ModelVersion, ModelFile, error) {
	var version ModelVersion
	found := false
	for _, v := range info.Versions {
		var pick bool
		if versionID != 0 {
			pick = v.ID == versionID
		} else {
			pick = !found || v.ID > version.ID
		}
		if pick {
			version, found = v, true
		}
	}
	if !found {
		if versionID != 0 {
			return version, ModelFile{}, fmt.Errorf("%w: model %d has no version %d", ErrNoDownloadableFile, info.ID, versionID)
		}
		return version, ModelFile{}, fmt.Errorf("%w: model %d has no versions", ErrNoDownloadableFile, info.ID)
	}

	isSafetensors := func(f ModelFile) bool {
		return strings.HasSuffix(strings.ToLower(f.Name), ".safetensors") && f.DownloadURL != ""
	}
	for _, f := range version.Files {
		if f.Primary && isSafetensors(f) {
			return version, f, nil
		}
	}
	for _, f := range version.Files {
		if f.Type == "Model" && isSafetensors(f) {
			return version, f, nil
		}
	}
	return version, ModelFile{}, fmt.Errorf("%w: version %d has no .safetensors file", ErrNoDownloadableFile, version.ID)
}

// sizeMatches compares a size in bytes with the sizeKB of the metadata. That
// is a float that Civitai may round, so a few bytes of difference are fine.
func sizeMatches(size int64, sizeKB float64) bool {
	return sizeKB > 0 && math.Abs(float64(size)-sizeKB*1024) < 16
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

// writeJSONFile writes v to path atomically.
func writeJSONFile(path string, v any) (err error) {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".meta-*.json.tmp")
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			tmp.Close()
			os.Remove(tmp.Name())
		}
	}()
	if _, err = tmp.Write(append(b, '\n')); err != nil {
		return err
	}
	if err = tmp.Close(); err != nil {
		return err
	}
	if err = os.Chmod(tmp.Name(), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}
