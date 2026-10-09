# gocivitailib

A small Go client for [Civitai](https://civitai.com): image generation through the
orchestration API, and model lookup and download through the site API.
Standard library only (tests use testify).

```go
import cv "github.com/rm4n0s/gocivitailib"
```

Create an API key under *Account settings → API Keys* on civitai.com. Civitai requires a
token for every model file download and for the orchestration API; model metadata is public.

## Client

```go
client := cv.NewClient(cv.WithToken(os.Getenv("CIVITAI_TOKEN")))
```

`NewClient(opts ...Option) *Client` reads `CIVITAI_TOKEN` and `CIVITAI_DOMAIN` from the
environment unless options say otherwise. The domain defaults to `civitai.com`, which gives
`https://civitai.com` (site API) and `https://orchestration.civitai.com` (orchestration API).

| Option | Effect |
| --- | --- |
| `WithToken(string)` | API token. Without one, calls that need it return `ErrMissingToken` before sending anything. |
| `WithDomain(string)` | Domain the two URLs are derived from. |
| `WithSiteURL(string)` | Override the site API base URL (for tests: point it at an `httptest` server). |
| `WithOrchestrationURL(string)` | Override the orchestration API base URL. |
| `WithHttpClient(*http.Client)` | Use your own HTTP client. It is copied; its redirect policy is wrapped so the token is never sent to another host. |
| `WithLogger(*slog.Logger)` | Debug logging of method, path, status and duration. Never bodies, query strings or redirect targets. |
| `WithMaxRetries(int)` | Retries of a GET after a 429 or 5xx. Default 3, `0` disables. |
| `WithRetryBackoff(base, max time.Duration)` | Back-off between retries: doubles from `base` up to `max`, with jitter. Default 500ms and 30s. A `Retry-After` header is honoured; one longer than `max` ends the retrying at once. |

The default HTTP client has dial, TLS and response-header timeouts but no overall timeout.
Bound a call with the context.

Only GET requests are retried. A workflow submission is never retried, because it could spend
Buzz twice.

## Image generation

### `SubmitImageWorkflow(ctx, ImageWorkflowRequest) (*ImageWorkflowResponse, int, error)`

Validates the request, submits it with `wait=60`, and returns the workflow, the HTTP status
(`200` finished, `202` still running) and an error.

```go
req := cv.ImageWorkflowRequest{Steps: []cv.ImageGenStep{{
    Type: cv.TypeImageGen,
    Input: cv.ImageGenInput{
        Engine:       cv.EngineSdCpp,
        Ecosystem:    cv.EcosystemSDXL,
        Operation:    cv.OperationCreateImage,
        Model:        "urn:air:sdxl:checkpoint:civitai:2940478",
        Prompt:       "masterpiece, portrait, warrior, ornate armor",
        SampleMethod: cv.SdCppSampleEulerA, // sdcpp field; comfy would use Sampler
        Width:        1024,
        Height:       1024,
    },
}}}
wf, status, err := client.SubmitImageWorkflow(ctx, req)
```

### `GetImageWorkflow(ctx, workflowID string) (*ImageWorkflowResponse, error)`

```go
for !cv.IsTerminalStatus(wf.Status) {
    time.Sleep(3 * time.Second)
    if wf, err = client.GetImageWorkflow(ctx, wf.ID); err != nil {
        return err
    }
}
for _, step := range wf.Steps {
    if step.Output != nil {
        for _, img := range step.Output.Images {
            fmt.Println(img.URL) // signed and expiring
        }
    }
}
```

### `IsTerminalStatus(WorkflowStatus) bool`

True for `succeeded`, `failed`, `expired` and `canceled`. The other statuses are
`unassigned`, `preparing`, `scheduled` and `processing`.

### `ImageGenInput.Validate() error` and `ImageWorkflowRequest.Validate() error`

`SubmitImageWorkflow` calls these for you. They return `nil`, one `*ValidationError`, or
several joined with `errors.Join`:

* `engine`, `ecosystem`, `operation` and `prompt` are required.
* `sampleMethod`, `schedule`, `uCache`, `strength` are sdcpp-only; `sampler`, `scheduler`,
  `denoiseStrength` are comfy-only.
* `clipSkip` is for SD1 only.
* Flux1 on sdcpp requires `diffuserModel` and rejects `model`; every other combination
  (except the Kontext engine) requires `model` and rejects `diffuserModel`.
* `width` and `height` must be divisible by 16.
* `quantity` is 1–12 for SD1 and SDXL, and 1–4 for Flux1.

```go
if err := in.Validate(); err != nil {
    var ve *cv.ValidationError
    if errors.As(err, &ve) && ve.Rule == cv.RuleNotAllowed {
        log.Printf("%s: %s", ve.Field, ve.Message)
    }
}
```

## Models

### `GetModel(ctx, modelID int) (*ModelInfo, error)`

`GET /api/v1/models/{id}`. `ModelInfo` has `ID`, `Name`, `Type`, `Tags`, `Description` and
`Versions []ModelVersion`; a `ModelVersion` has `ID`, `Name`, `BaseModel`, `TrainedWords` and
`Files []ModelFile`; a `ModelFile` has `Name`, `SizeKB`, `Type`, `Primary`, `DownloadURL` and
`Hashes`. `ModelFile.SHA256()` returns the lower-case SHA256 or `""`.

```go
m, err := client.GetModel(ctx, 12345)
for _, v := range m.Versions {
    fmt.Println(v.ID, v.BaseModel, v.TrainedWords)
}
```

### `DownloadModelVersion(ctx, versionID int, w io.Writer) (int64, error)`

`GET /api/download/models/{versionId}` with the bearer token, streamed into `w`.

```go
f, _ := os.Create("lora.safetensors")
defer f.Close()
n, err := client.DownloadModelVersion(ctx, 67890, f)
```

### `DownloadFile(ctx, downloadURL string, w io.Writer) (int64, error)`

The same, for a `DownloadURL` taken from the metadata. The token is sent only to the
configured site host. Redirects to any other host are followed without it. A 429 or 5xx is
retried before streaming starts; a failure after bytes were written is not.

```go
n, err := client.DownloadFile(ctx, file.DownloadURL, w)
```

### `DownloadModel(ctx, modelID int, dir string, opts ...DownloadOption) (*DownloadedModel, error)`

The convenience for importing a LoRA by model id:

1. fetches the metadata;
2. picks the newest version (highest id) or the one given with `WithVersionID(id)`, and its
   primary `.safetensors` file, else `ErrNoDownloadableFile`;
3. keeps an existing `<dir>/<modelID>.safetensors` of the expected size;
4. otherwise streams to a temporary file in `dir`, checks the SHA256 when the metadata has one
   (`ErrHashMismatch`), and renames it into place;
5. writes `<dir>/<modelID>.json` (`ModelMetadata`: `name`, `baseModel`, `triggerWords`,
   `tags`, `description`, `versionId`).

Temporary files are removed on every failure, including cancellation.

```go
res, err := client.DownloadModel(ctx, 12345, "/var/lib/app/loras")
if errors.Is(err, cv.ErrNoDownloadableFile) {
    // the model has no .safetensors file
}
fmt.Println(res.ModelPath, res.MetadataPath, res.Skipped, res.Version.TrainedWords)

res, err = client.DownloadModel(ctx, 12345, dir, cv.WithVersionID(67890))
```

## Errors

Every error wraps its cause with `%w`, so `errors.Is` and `errors.As` work through any
number of layers. Messages are prefixed with the operation, for example
`download model: get model: unexpected http status 404`.

| Error | Meaning |
| --- | --- |
| `ErrMissingToken` | The call needs a token and the client has none. Nothing was sent. |
| `ErrNoDownloadableFile` | No version, or no primary `.safetensors` file with a download URL. |
| `ErrInvalidResponse` | A response was not valid JSON, lacked a required field, or a download returned an HTML page. |
| `ErrHashMismatch` | The downloaded file's SHA256 differs from the metadata. |
| `*StatusError{Status, Body}` | A non-2xx response. `Body` is cut to 4 KiB. |
| `*RateLimitedError{RetryAfter}` | A 429, after the retries ran out. Also matches `*StatusError`. |
| `*ValidationError{Field, Rule, Message}` | The request broke a rule and was not sent. |

`ValidationError.Rule` is one of `RuleRequired`, `RuleNotAllowed`, `RuleMultipleOf16` and
`RuleOutOfRange`.

```go
var (
    se *cv.StatusError
    rl *cv.RateLimitedError
)
switch {
case errors.Is(err, cv.ErrMissingToken):
case errors.As(err, &rl):
    time.Sleep(rl.RetryAfter)
case errors.As(err, &se) && se.Status == http.StatusNotFound:
}
```

## Tests

`go test ./...` uses `httptest` and needs no network. `tests/` holds an integration test
against the real API; it spends Buzz and runs only when `CIVITAI_TOKEN` is set.
