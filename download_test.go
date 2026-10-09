package gocivitailib_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	cv "github.com/rm4n0s/gocivitailib"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const largeSize = 64 << 20

// serveBytes answers with n bytes of the test pattern.
func serveBytes(n int64) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Length", strconv.FormatInt(n, 10))
		io.Copy(w, newPattern(n))
	}
}

// singleFile is a version list with one primary file of the given size.
func singleFile(name string, size int64, sha string) func(base string) []map[string]any {
	return func(base string) []map[string]any {
		return []map[string]any{versionJSON(7, fileJSON(name, base+"/files/model", float64(size)/1024, true, sha))}
	}
}

// modelServer serves model 42's metadata and, for every other path, files.
// hits counts the requests that reached files.
func modelServer(t *testing.T, versions func(base string) []map[string]any, files http.Handler, opts ...cv.Option) (client *cv.Client, hits *atomic.Int32) {
	t.Helper()
	hits = new(atomic.Int32)
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/models/42", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(t, w, 200, modelJSON(42, versions("http://"+r.Host)...))
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		files.ServeHTTP(w, r)
	})
	client, _ = newClient(t, mux, opts...)
	return client, hits
}

func entries(t *testing.T, dir string) []string {
	t.Helper()
	des, err := os.ReadDir(dir)
	require.NoError(t, err)
	var names []string
	for _, de := range des {
		names = append(names, de.Name())
	}
	return names
}

func sha256Hex(b []byte) string {
	s := sha256.Sum256(b)
	return strings.ToUpper(hex.EncodeToString(s[:]))
}

// allocatedDuring returns the bytes allocated (in total, not retained) by f.
func allocatedDuring(f func()) uint64 {
	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	f()
	runtime.ReadMemStats(&after)
	return after.TotalAlloc - before.TotalAlloc
}

func TestDownloadModelVersion(t *testing.T) {
	var path, auth string
	client, _ := newClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path, auth = r.URL.Path, r.Header.Get("Authorization")
		io.WriteString(w, "lora-bytes")
	}))
	var sb strings.Builder
	n, err := client.DownloadModelVersion(context.Background(), 123, &sb)
	require.NoError(t, err)
	assert.EqualValues(t, 10, n)
	assert.Equal(t, "lora-bytes", sb.String())
	assert.Equal(t, "/api/download/models/123", path)
	assert.Equal(t, "Bearer "+testToken, auth)
}

func TestDownloadModelVersionFailures(t *testing.T) {
	t.Run("missing token", func(t *testing.T) {
		client, _ := newClient(t, http.NotFoundHandler(), cv.WithToken(""))
		n, err := client.DownloadModelVersion(context.Background(), 1, io.Discard)
		assert.ErrorIs(t, err, cv.ErrMissingToken)
		assert.Zero(t, n)
		assert.True(t, strings.HasPrefix(err.Error(), "download model version: "), err.Error())
	})
	t.Run("invalid id", func(t *testing.T) {
		client, _ := newClient(t, http.NotFoundHandler())
		_, err := client.DownloadModelVersion(context.Background(), 0, io.Discard)
		var ve *cv.ValidationError
		require.ErrorAs(t, err, &ve)
		assert.Equal(t, "versionID", ve.Field)
	})
	t.Run("unauthorized", func(t *testing.T) {
		client, _ := newClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(401)
			io.WriteString(w, `{"error":"Unauthorized"}`)
		}))
		var sb strings.Builder
		_, err := client.DownloadModelVersion(context.Background(), 1, &sb)
		var se *cv.StatusError
		require.ErrorAs(t, err, &se)
		assert.Equal(t, 401, se.Status)
		assert.Empty(t, sb.String(), "an error body must not be written to the destination")
	})
	t.Run("html page instead of a file", func(t *testing.T) {
		client, _ := newClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			io.WriteString(w, "<html>please log in</html>")
		}))
		var sb strings.Builder
		_, err := client.DownloadModelVersion(context.Background(), 1, &sb)
		assert.ErrorIs(t, err, cv.ErrInvalidResponse)
		assert.Empty(t, sb.String())
	})
	t.Run("429 followed by success", func(t *testing.T) {
		var hits atomic.Int32
		client, _ := newClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if hits.Add(1) == 1 {
				w.Header().Set("Retry-After", "0")
				w.WriteHeader(429)
				return
			}
			io.WriteString(w, "ok")
		}))
		var sb strings.Builder
		_, err := client.DownloadModelVersion(context.Background(), 1, &sb)
		require.NoError(t, err)
		assert.Equal(t, "ok", sb.String())
		assert.EqualValues(t, 2, hits.Load())
	})
	t.Run("429 forever", func(t *testing.T) {
		client, _ := newClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(429)
		}), cv.WithMaxRetries(1))
		_, err := client.DownloadModelVersion(context.Background(), 1, io.Discard)
		var rl *cv.RateLimitedError
		assert.ErrorAs(t, err, &rl)
	})
}

func TestDownloadFile(t *testing.T) {
	t.Run("token goes to the site host only", func(t *testing.T) {
		var siteAuth, otherAuth string
		other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			otherAuth = r.Header.Get("Authorization")
			io.WriteString(w, "other")
		}))
		t.Cleanup(other.Close)
		client, srv := newClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			siteAuth = r.Header.Get("Authorization")
			io.WriteString(w, "site")
		}))

		var sb strings.Builder
		_, err := client.DownloadFile(context.Background(), other.URL+"/f", &sb)
		require.NoError(t, err)
		assert.Equal(t, "other", sb.String())
		assert.Empty(t, otherAuth, "the token must not be sent to a foreign host")

		sb.Reset()
		_, err = client.DownloadFile(context.Background(), srv.URL+"/f", &sb)
		require.NoError(t, err)
		assert.Equal(t, "site", sb.String())
		assert.Equal(t, "Bearer "+testToken, siteAuth)
	})

	t.Run("a foreign host needs no token", func(t *testing.T) {
		other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, "x") }))
		t.Cleanup(other.Close)
		client, _ := newClient(t, http.NotFoundHandler(), cv.WithToken(""))
		_, err := client.DownloadFile(context.Background(), other.URL, io.Discard)
		assert.NoError(t, err)
	})

	t.Run("the site host needs a token", func(t *testing.T) {
		client, srv := newClient(t, http.NotFoundHandler(), cv.WithToken(""))
		_, err := client.DownloadFile(context.Background(), srv.URL+"/f", io.Discard)
		assert.ErrorIs(t, err, cv.ErrMissingToken)
		assert.True(t, strings.HasPrefix(err.Error(), "download file: "), err.Error())
	})

	for _, bad := range []string{"", "not a url", "/relative/path", "ftp://example.com/x", "http://"} {
		t.Run("invalid url "+bad, func(t *testing.T) {
			client, _ := newClient(t, http.NotFoundHandler())
			_, err := client.DownloadFile(context.Background(), bad, io.Discard)
			var ve *cv.ValidationError
			require.ErrorAs(t, err, &ve)
			assert.Equal(t, "downloadURL", ve.Field)
		})
	}

	t.Run("a large file is streamed, not buffered", func(t *testing.T) {
		client, srv := newClient(t, serveBytes(largeSize))
		w := &countingWriter{}
		var n int64
		var err error
		alloc := allocatedDuring(func() {
			n, err = client.DownloadFile(context.Background(), srv.URL+"/big", w)
		})
		require.NoError(t, err)
		assert.EqualValues(t, largeSize, n)
		assert.EqualValues(t, largeSize, w.n)
		assert.Less(t, alloc, uint64(largeSize/8), "allocated %d bytes for a %d byte file", alloc, largeSize)
	})
}

func TestDownloadRedirects(t *testing.T) {
	t.Run("to another host drops the token", func(t *testing.T) {
		var otherAuth atomic.Value
		otherAuth.Store("unset")
		other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			otherAuth.Store(r.Header.Get("Authorization"))
			io.WriteString(w, "payload")
		}))
		t.Cleanup(other.Close)

		var siteAuth string
		client, _ := newClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			siteAuth = r.Header.Get("Authorization")
			http.Redirect(w, r, other.URL+"/cdn/file?sig=abc", http.StatusFound)
		}))
		var sb strings.Builder
		n, err := client.DownloadModelVersion(context.Background(), 5, &sb)
		require.NoError(t, err)
		assert.EqualValues(t, 7, n)
		assert.Equal(t, "payload", sb.String())
		assert.Equal(t, "Bearer "+testToken, siteAuth)
		assert.Equal(t, "", otherAuth.Load(), "the token must not follow a redirect to another host")
	})

	t.Run("to the same host keeps the token", func(t *testing.T) {
		var finalAuth string
		client, _ := newClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/real" {
				finalAuth = r.Header.Get("Authorization")
				io.WriteString(w, "payload")
				return
			}
			http.Redirect(w, r, "/real", http.StatusTemporaryRedirect)
		}))
		_, err := client.DownloadModelVersion(context.Background(), 5, io.Discard)
		require.NoError(t, err)
		assert.Equal(t, "Bearer "+testToken, finalAuth)
	})

	t.Run("redirect loop stops", func(t *testing.T) {
		var hits atomic.Int32
		client, _ := newClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			hits.Add(1)
			http.Redirect(w, r, "/again", http.StatusFound)
		}))
		_, err := client.DownloadModelVersion(context.Background(), 5, io.Discard)
		require.Error(t, err)
		assert.LessOrEqual(t, hits.Load(), int32(11))
	})

	t.Run("custom http client keeps its own redirect policy and still drops the token", func(t *testing.T) {
		var otherAuth atomic.Value
		otherAuth.Store("unset")
		other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			otherAuth.Store(r.Header.Get("Authorization"))
			io.WriteString(w, "payload")
		}))
		t.Cleanup(other.Close)
		var consulted atomic.Int32
		custom := &http.Client{CheckRedirect: func(req *http.Request, via []*http.Request) error {
			consulted.Add(1)
			return nil
		}}
		client, _ := newClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, other.URL, http.StatusFound)
		}), cv.WithHttpClient(custom))
		_, err := client.DownloadModelVersion(context.Background(), 5, io.Discard)
		require.NoError(t, err)
		assert.EqualValues(t, 1, consulted.Load())
		assert.Equal(t, "", otherAuth.Load())
	})
}

func TestDownloadModel(t *testing.T) {
	content := []byte("0123456789abcdef-lora-weights")

	t.Run("downloads, verifies and writes metadata", func(t *testing.T) {
		client, hits := modelServer(t, singleFile("my.safetensors", int64(len(content)), sha256Hex(content)), http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "Bearer "+testToken, r.Header.Get("Authorization"))
			w.Write(content)
		}))
		dir := filepath.Join(t.TempDir(), "nested", "loras")
		res, err := client.DownloadModel(context.Background(), 42, dir)
		require.NoError(t, err)
		assert.EqualValues(t, 1, hits.Load())

		assert.Equal(t, filepath.Join(dir, "42.safetensors"), res.ModelPath)
		assert.Equal(t, filepath.Join(dir, "42.json"), res.MetadataPath)
		assert.False(t, res.Skipped)
		assert.EqualValues(t, len(content), res.Size)
		assert.Equal(t, 42, res.Info.ID)
		assert.Equal(t, 7, res.Version.ID)
		assert.Equal(t, "my.safetensors", res.File.Name)

		got, err := os.ReadFile(res.ModelPath)
		require.NoError(t, err)
		assert.Equal(t, content, got)
		assert.Equal(t, []string{"42.json", "42.safetensors"}, entries(t, dir), "no temporary files may remain")

		raw, err := os.ReadFile(res.MetadataPath)
		require.NoError(t, err)
		var meta cv.ModelMetadata
		require.NoError(t, json.Unmarshal(raw, &meta))
		assert.Equal(t, cv.ModelMetadata{
			Name:         "My LoRA",
			BaseModel:    "SDXL 1.0",
			TriggerWords: []string{"trigger one", "trigger two"},
			Tags:         []string{"anime", "style"},
			Description:  "<p>desc</p>",
			VersionID:    7,
		}, meta)
		assert.Contains(t, string(raw), `"triggerWords"`)
		assert.Contains(t, string(raw), `"versionId": 7`)
	})

	t.Run("a large file is streamed to disk, not buffered", func(t *testing.T) {
		client, _ := modelServer(t, singleFile("big.safetensors", largeSize, patternSHA256(t, largeSize)), serveBytes(largeSize))
		dir := t.TempDir()
		var res *cv.DownloadedModel
		var err error
		alloc := allocatedDuring(func() {
			res, err = client.DownloadModel(context.Background(), 42, dir)
		})
		require.NoError(t, err)
		assert.EqualValues(t, largeSize, res.Size)
		st, err := os.Stat(res.ModelPath)
		require.NoError(t, err)
		assert.EqualValues(t, largeSize, st.Size())
		assert.Less(t, alloc, uint64(largeSize/8), "allocated %d bytes for a %d byte file", alloc, largeSize)
	})

	t.Run("hash mismatch", func(t *testing.T) {
		client, _ := modelServer(t, singleFile("my.safetensors", int64(len(content)), strings.Repeat("0", 64)), http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write(content)
		}))
		dir := t.TempDir()
		// A previous good copy of a different size must survive the failure.
		old := filepath.Join(dir, "42.safetensors")
		require.NoError(t, os.WriteFile(old, []byte("old"), 0o644))

		res, err := client.DownloadModel(context.Background(), 42, dir)
		assert.Nil(t, res)
		assert.ErrorIs(t, err, cv.ErrHashMismatch)
		assert.True(t, strings.HasPrefix(err.Error(), "download model: "), err.Error())
		assert.Equal(t, []string{"42.safetensors"}, entries(t, dir))
		b, _ := os.ReadFile(old)
		assert.Equal(t, "old", string(b))
	})

	t.Run("no hash in the metadata means no verification", func(t *testing.T) {
		client, _ := modelServer(t, singleFile("my.safetensors", int64(len(content)), ""), http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write(content)
		}))
		_, err := client.DownloadModel(context.Background(), 42, t.TempDir())
		assert.NoError(t, err)
	})

	t.Run("existing file of the expected size is skipped", func(t *testing.T) {
		existing := []byte("already here, 29 bytes long!!")
		client, hits := modelServer(t, singleFile("my.safetensors", int64(len(existing)), strings.Repeat("0", 64)), http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte("should not be fetched"))
		}))
		dir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(dir, "42.safetensors"), existing, 0o644))

		res, err := client.DownloadModel(context.Background(), 42, dir)
		require.NoError(t, err)
		assert.True(t, res.Skipped)
		assert.Zero(t, hits.Load(), "nothing may be downloaded")
		assert.EqualValues(t, len(existing), res.Size)
		b, _ := os.ReadFile(res.ModelPath)
		assert.Equal(t, existing, b)
		assert.FileExists(t, res.MetadataPath, "metadata is written even when the download is skipped")
	})

	t.Run("existing file of another size is replaced", func(t *testing.T) {
		client, hits := modelServer(t, singleFile("my.safetensors", int64(len(content)), sha256Hex(content)), http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write(content)
		}))
		dir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(dir, "42.safetensors"), []byte("short"), 0o644))
		res, err := client.DownloadModel(context.Background(), 42, dir)
		require.NoError(t, err)
		assert.False(t, res.Skipped)
		assert.EqualValues(t, 1, hits.Load())
		b, _ := os.ReadFile(res.ModelPath)
		assert.Equal(t, content, b)
	})

	t.Run("an existing file is not trusted without a known size", func(t *testing.T) {
		client, hits := modelServer(t, singleFile("my.safetensors", 0, ""), http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write(content)
		}))
		dir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(dir, "42.safetensors"), content, 0o644))
		res, err := client.DownloadModel(context.Background(), 42, dir)
		require.NoError(t, err)
		assert.False(t, res.Skipped)
		assert.EqualValues(t, 1, hits.Load())
	})

	t.Run("cancelled mid-download removes the temporary file", func(t *testing.T) {
		started := make(chan struct{})
		var once sync.Once
		client, _ := modelServer(t, singleFile("my.safetensors", 1<<30, ""), http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Length", strconv.Itoa(1<<30))
			w.Write(make([]byte, 64<<10))
			w.(http.Flusher).Flush()
			once.Do(func() { close(started) })
			<-r.Context().Done()
		}))
		dir := t.TempDir()
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		go func() {
			<-started
			cancel()
		}()

		res, err := client.DownloadModel(ctx, 42, dir)
		assert.Nil(t, res)
		assert.ErrorIs(t, err, context.Canceled)
		assert.Empty(t, entries(t, dir), "no temporary file may remain")
	})

	t.Run("server error removes the temporary file", func(t *testing.T) {
		client, _ := modelServer(t, singleFile("my.safetensors", 10, ""), http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(500)
		}), cv.WithMaxRetries(1))
		dir := t.TempDir()
		_, err := client.DownloadModel(context.Background(), 42, dir)
		var se *cv.StatusError
		require.ErrorAs(t, err, &se)
		assert.Equal(t, 500, se.Status)
		assert.Empty(t, entries(t, dir))
	})

	t.Run("429 followed by success", func(t *testing.T) {
		var calls atomic.Int32
		client, hits := modelServer(t, singleFile("my.safetensors", int64(len(content)), sha256Hex(content)), http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if calls.Add(1) == 1 {
				w.Header().Set("Retry-After", "0")
				w.WriteHeader(429)
				return
			}
			w.Write(content)
		}))
		dir := t.TempDir()
		res, err := client.DownloadModel(context.Background(), 42, dir)
		require.NoError(t, err)
		assert.EqualValues(t, 2, hits.Load())
		assert.EqualValues(t, len(content), res.Size)
		assert.Equal(t, []string{"42.json", "42.safetensors"}, entries(t, dir))
	})

	t.Run("redirect to another host while downloading", func(t *testing.T) {
		var cdnAuth atomic.Value
		cdnAuth.Store("unset")
		cdn := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			cdnAuth.Store(r.Header.Get("Authorization"))
			w.Write(content)
		}))
		t.Cleanup(cdn.Close)
		client, _ := modelServer(t, singleFile("my.safetensors", int64(len(content)), sha256Hex(content)), http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, cdn.URL+"/blob", http.StatusTemporaryRedirect)
		}))
		dir := t.TempDir()
		res, err := client.DownloadModel(context.Background(), 42, dir)
		require.NoError(t, err)
		assert.EqualValues(t, len(content), res.Size)
		assert.Equal(t, "", cdnAuth.Load())
	})

	t.Run("metadata failure", func(t *testing.T) {
		client, _ := newClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(404)
			io.WriteString(w, `{"error":"Model not found"}`)
		}))
		dir := t.TempDir()
		_, err := client.DownloadModel(context.Background(), 42, dir)
		var se *cv.StatusError
		require.ErrorAs(t, err, &se)
		assert.Equal(t, 404, se.Status)
		assert.Equal(t, "download model: get model: unexpected http status 404: {\"error\":\"Model not found\"}", err.Error())
		assert.Empty(t, entries(t, dir))
	})

	t.Run("invalid response from the metadata endpoint", func(t *testing.T) {
		client, _ := newClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			io.WriteString(w, "not json")
		}))
		_, err := client.DownloadModel(context.Background(), 42, t.TempDir())
		assert.ErrorIs(t, err, cv.ErrInvalidResponse)
	})
}

func TestDownloadModelArguments(t *testing.T) {
	client, _ := newClient(t, http.NotFoundHandler())
	_, err := client.DownloadModel(context.Background(), 0, t.TempDir())
	var ve *cv.ValidationError
	require.ErrorAs(t, err, &ve)
	assert.Equal(t, "modelID", ve.Field)

	_, err = client.DownloadModel(context.Background(), 1, "")
	require.ErrorAs(t, err, &ve)
	assert.Equal(t, "dir", ve.Field)
	assert.Equal(t, cv.RuleRequired, ve.Rule)

	var hits atomic.Int32
	noToken, _ := newClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hits.Add(1) }), cv.WithToken(""))
	_, err = noToken.DownloadModel(context.Background(), 1, t.TempDir())
	assert.ErrorIs(t, err, cv.ErrMissingToken)
	assert.Zero(t, hits.Load(), "a download without a token must fail before any request")
}

func TestDownloadModelChoosesVersionAndFile(t *testing.T) {
	file := func(name, typ string, primary bool) map[string]any {
		f := fileJSON(name, "BASE/files/"+name, 1, primary, "")
		f["type"] = typ
		return f
	}
	versions := func(vs ...map[string]any) func(string) []map[string]any {
		return func(base string) []map[string]any {
			raw, _ := json.Marshal(vs)
			raw = []byte(strings.ReplaceAll(string(raw), "BASE", base))
			var out []map[string]any
			_ = json.Unmarshal(raw, &out)
			return out
		}
	}
	ver := func(id int, files ...map[string]any) map[string]any { return versionJSON(id, files...) }

	tests := []struct {
		name     string
		versions []map[string]any
		opts     []cv.DownloadOption
		wantVer  int
		wantFile string
		wantErr  error
	}{
		{name: "newest version by id, whatever the order",
			versions: []map[string]any{ver(10, file("old.safetensors", "Model", true)), ver(30, file("new.safetensors", "Model", true)), ver(20, file("mid.safetensors", "Model", true))},
			wantVer:  30, wantFile: "new.safetensors"},
		{name: "WithVersionID",
			versions: []map[string]any{ver(10, file("old.safetensors", "Model", true)), ver(30, file("new.safetensors", "Model", true))},
			opts:     []cv.DownloadOption{cv.WithVersionID(10)}, wantVer: 10, wantFile: "old.safetensors"},
		{name: "primary safetensors is preferred",
			versions: []map[string]any{ver(1, file("a.safetensors", "Model", false), file("b.safetensors", "Model", true), file("c.zip", "Training Data", false))},
			wantVer:  1, wantFile: "b.safetensors"},
		{name: "extension is case-insensitive",
			versions: []map[string]any{ver(1, file("A.SafeTensors", "Model", true))},
			wantVer:  1, wantFile: "A.SafeTensors"},
		{name: "falls back to a Model file when none is primary",
			versions: []map[string]any{ver(1, file("data.safetensors", "Training Data", false), file("m.safetensors", "Model", false))},
			wantVer:  1, wantFile: "m.safetensors"},
		{name: "primary file is not safetensors",
			versions: []map[string]any{ver(1, file("m.pt", "Model", true), file("data.safetensors", "Training Data", false))},
			wantErr:  cv.ErrNoDownloadableFile},
		{name: "newest version has no safetensors, an older one does",
			versions: []map[string]any{ver(1, file("m.safetensors", "Model", true)), ver(2, file("m.ckpt", "Model", true))},
			wantErr:  cv.ErrNoDownloadableFile},
		{name: "version without files", versions: []map[string]any{ver(1)}, wantErr: cv.ErrNoDownloadableFile},
		{name: "model without versions", versions: nil, wantErr: cv.ErrNoDownloadableFile},
		{name: "unknown version id",
			versions: []map[string]any{ver(1, file("m.safetensors", "Model", true))},
			opts:     []cv.DownloadOption{cv.WithVersionID(99)}, wantErr: cv.ErrNoDownloadableFile},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, hits := modelServer(t, versions(tt.versions...), http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				io.WriteString(w, "data:"+path2name(r.URL.Path))
			}))
			dir := t.TempDir()
			res, err := client.DownloadModel(context.Background(), 42, dir, tt.opts...)
			if tt.wantErr != nil {
				assert.ErrorIs(t, err, tt.wantErr)
				assert.Nil(t, res)
				assert.Zero(t, hits.Load())
				assert.Empty(t, entries(t, dir), fmt.Sprint("nothing may be written: ", entries(t, dir)))
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantVer, res.Version.ID)
			assert.Equal(t, tt.wantFile, res.File.Name)
			b, _ := os.ReadFile(res.ModelPath)
			assert.Equal(t, "data:"+tt.wantFile, string(b))
			raw, _ := os.ReadFile(res.MetadataPath)
			var meta cv.ModelMetadata
			require.NoError(t, json.Unmarshal(raw, &meta))
			assert.Equal(t, tt.wantVer, meta.VersionID)
		})
	}
}

func path2name(p string) string { return p[strings.LastIndex(p, "/")+1:] }
