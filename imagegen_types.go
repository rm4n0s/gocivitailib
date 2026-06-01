package gocivitailib

// ---------------------------------------------------------------------------
// Engine
// ---------------------------------------------------------------------------

// Engine selects the inference backend.
type Engine = string

const (
	EngineSdCpp        Engine = "sdcpp"         // SD C++ on Civitai workers — default for SD1, SDXL, Flux1
	EngineComfy        Engine = "comfy"         // ComfyUI workers — needed for Comfy-exclusive samplers/schedulers
	EngineFlux1Kontext Engine = "flux1-kontext" // BFL-managed Kontext editing tier — Flux1 only
)

// ---------------------------------------------------------------------------
// Ecosystem
// ---------------------------------------------------------------------------

// Ecosystem identifies the model family.
type Ecosystem = string

const (
	EcosystemSD1   Ecosystem = "sd1"
	EcosystemSDXL  Ecosystem = "sdxl"
	EcosystemFlux1 Ecosystem = "flux1"
	EcosystemFlux2 Ecosystem = "flux2"
)

// ---------------------------------------------------------------------------
// Operation
// ---------------------------------------------------------------------------

// Operation is the generation mode for an imageGen step.
type Operation = string

const (
	OperationCreateImage   Operation = "createImage"   // text-to-image
	OperationCreateVariant Operation = "createVariant" // img2img, strength-weighted re-imagining
	OperationEditImage     Operation = "editImage"     // prompt-driven edit — Flux1 sdcpp and flux1-kontext only
)

// ---------------------------------------------------------------------------
// SdCpp enums — only valid when Engine == EngineSdCpp
// ---------------------------------------------------------------------------

// SdCppSampleMethod is the sampler used by the sdcpp engine (`sampleMethod` field).
// Not valid on comfy — use ComfySampler instead.
type SdCppSampleMethod = string

const (
	SdCppSampleEuler       SdCppSampleMethod = "euler"
	SdCppSampleEulerA      SdCppSampleMethod = "euler_a"
	SdCppSampleHeun        SdCppSampleMethod = "heun"
	SdCppSampleDPM2        SdCppSampleMethod = "dpm2"
	SdCppSampleDPMPP2SA    SdCppSampleMethod = "dpmpp2s_a"
	SdCppSampleDPMPP2M     SdCppSampleMethod = "dpmpp2m"
	SdCppSampleDPMPPSDE    SdCppSampleMethod = "dpmpp_sde"
	SdCppSampleDPMFast     SdCppSampleMethod = "dpm_fast"
	SdCppSampleDPMAdaptive SdCppSampleMethod = "dpm_adaptive"
	SdCppSampleLCM         SdCppSampleMethod = "lcm"
	SdCppSampleIPNDM       SdCppSampleMethod = "ipndm"
	SdCppSampleIPNDM_V     SdCppSampleMethod = "ipndm_v"
	SdCppSampleLMS         SdCppSampleMethod = "lms"
)

// SdCppSchedule is the noise schedule used by the sdcpp engine (`schedule` field).
// Not valid on comfy — use ComfyScheduler instead.
type SdCppSchedule = string

const (
	SdCppScheduleDiscrete    SdCppSchedule = "discrete" // default for SD1 and SDXL
	SdCppScheduleSimple      SdCppSchedule = "simple"   // default for Flux1
	SdCppScheduleKarras      SdCppSchedule = "karras"
	SdCppScheduleExponential SdCppSchedule = "exponential"
	SdCppScheduleAYS         SdCppSchedule = "ays"
	SdCppScheduleGITS        SdCppSchedule = "gits"
)

// SdCppUCacheMode controls the caching strategy on sdcpp workers (`uCache` field).
// Not valid on comfy.
type SdCppUCacheMode = string

const (
	SdCppUCacheNone    SdCppUCacheMode = "none"
	SdCppUCacheNormal  SdCppUCacheMode = "normal"
	SdCppUCacheTaeFast SdCppUCacheMode = "tae_fast"
)

// ---------------------------------------------------------------------------
// Comfy enums — only valid when Engine == EngineComfy
// ---------------------------------------------------------------------------

// ComfySampler is the sampler used by the comfy engine (`sampler` field).
// Not valid on sdcpp — use SdCppSampleMethod instead.
type ComfySampler = string

const (
	ComfySamplerEuler          ComfySampler = "euler"
	ComfySamplerEulerAncestral ComfySampler = "euler_ancestral"
	ComfySamplerHeun           ComfySampler = "heun"
	ComfySamplerHeunPP2        ComfySampler = "heunpp2"
	ComfySamplerDPM2           ComfySampler = "dpm_2"
	ComfySamplerDPM2Ancestral  ComfySampler = "dpm_2_ancestral"
	ComfySamplerLMS            ComfySampler = "lms"
	ComfySamplerDPMFast        ComfySampler = "dpm_fast"
	ComfySamplerDPMAdaptive    ComfySampler = "dpm_adaptive"
	ComfySamplerDPMPP2SA       ComfySampler = "dpmpp_2s_ancestral"
	ComfySamplerDPMPPSDE       ComfySampler = "dpmpp_sde"
	ComfySamplerDPMPPSDEGPU    ComfySampler = "dpmpp_sde_gpu"
	ComfySamplerDPMPP2M        ComfySampler = "dpmpp_2m"
	ComfySamplerDPMPP2MSDE     ComfySampler = "dpmpp_2m_sde"
	ComfySamplerDPMPP2MSDEGPu  ComfySampler = "dpmpp_2m_sde_gpu"
	ComfySamplerDPMPP3MSDE     ComfySampler = "dpmpp_3m_sde"
	ComfySamplerDPMPP3MSDEGPU  ComfySampler = "dpmpp_3m_sde_gpu"
	ComfySamplerDDPM           ComfySampler = "ddpm"
	ComfySamplerLCM            ComfySampler = "lcm"
	ComfySamplerIPNDM          ComfySampler = "ipndm"
	ComfySamplerIPNDM_V        ComfySampler = "ipndm_v"
	ComfySamplerDEIS           ComfySampler = "deis"
	ComfySamplerDDIM           ComfySampler = "ddim"
	ComfySamplerUniPC          ComfySampler = "uni_pc"
	ComfySamplerUniPCBH2       ComfySampler = "uni_pc_bh2"
)

// ComfyScheduler is the noise schedule used by the comfy engine (`scheduler` field).
// Not valid on sdcpp — use SdCppSchedule instead.
type ComfyScheduler = string

const (
	ComfySchedulerNormal          ComfyScheduler = "normal"
	ComfySchedulerKarras          ComfyScheduler = "karras"
	ComfySchedulerExponential     ComfyScheduler = "exponential"
	ComfySchedulerSGMUniform      ComfyScheduler = "sgm_uniform"
	ComfySchedulerSimple          ComfyScheduler = "simple"
	ComfySchedulerDDIM            ComfyScheduler = "ddim_uniform"
	ComfySchedulerBeta            ComfyScheduler = "beta"
	ComfySchedulerLinearQuadratic ComfyScheduler = "linear_quadratic"
	ComfySchedulerKLOptimal       ComfyScheduler = "kl_optimal"
)

// ---------------------------------------------------------------------------
// Shared helpers
// ---------------------------------------------------------------------------

// LoRAMap is a map of AIR URN → blend strength.
// Typical range 0.0–1.0; Flux ecosystems accept up to 2.0.
// Example: {"urn:air:sdxl:lora:civitai:123@456": 0.8}
type LoRAMap = map[string]float64

// WorkflowStatus is the lifecycle state of a workflow or individual step.
type WorkflowStatus = string

const (
	WorkflowStatusPending   WorkflowStatus = "pending"
	WorkflowStatusRunning   WorkflowStatus = "running"
	WorkflowStatusSucceeded WorkflowStatus = "succeeded"
	WorkflowStatusFailed    WorkflowStatus = "failed"
	WorkflowStatusCancelled WorkflowStatus = "cancelled"
)

// ---------------------------------------------------------------------------
// ImageGenInput — unified input struct for all image-generation recipes
//
// Fields are grouped by which engine/ecosystem accepts them. Sending a field
// that does not belong to your chosen engine returns HTTP 400.
//
// Quick-start combos:
//   SD1 default  : Engine=sdcpp,  Ecosystem=sd1,   Operation=createImage
//   SDXL default : Engine=sdcpp,  Ecosystem=sdxl,  Operation=createImage
//   Flux1 default: Engine=sdcpp,  Ecosystem=flux1, Operation=createImage  (use DiffuserModel, not Model)
//   Comfy/SDXL   : Engine=comfy,  Ecosystem=sdxl,  Operation=createImage
// ---------------------------------------------------------------------------

// ImageGenInput is the single `input` block used for every imageGen workflow step.
type ImageGenInput struct {
	// ---- Routing (required for every request) ----

	Engine    string `json:"engine"`    // EngineSdCpp | EngineComfy | EngineFlux1Kontext
	Ecosystem string `json:"ecosystem"` // EcosystemSD1 | EcosystemSDXL | EcosystemFlux1 | EcosystemFlux2
	Operation string `json:"operation"` // OperationCreateImage | OperationCreateVariant | OperationEditImage

	// ---- Model selection ----

	// Model is the checkpoint AIR URN used by sdcpp (SD1/SDXL) and comfy (all ecosystems).
	// Format: urn:air:<ecosystem>:checkpoint:civitai:<modelId>@<versionId>
	// Not used by Flux1 sdcpp — set DiffuserModel instead.
	Model string `json:"model,omitempty"`

	// DiffuserModel is the diffuser AIR URN required by Flux1 on Engine=sdcpp.
	// Format: urn:air:flux1:diffuser:civitai:<modelId>@<versionId>
	// Not valid on comfy or non-Flux1 ecosystems — use Model instead.
	DiffuserModel string `json:"diffuserModel,omitempty"`

	// ---- Prompt ----

	// Prompt is required for all engines.
	// SD1/SDXL: Booru-style tags work best (≤ 10 000 chars).
	// Flux1/Flux2: natural language preferred (≤ 1 000 chars on sdcpp).
	Prompt string `json:"prompt"`

	// NegativePrompt is strongly recommended for SD1/SDXL; ignored by some Flux tiers.
	NegativePrompt string `json:"negativePrompt,omitempty"`

	// ---- Dimensions ----

	Width  int `json:"width,omitempty"`  // SD1 default 512; SDXL/Flux1 default 1024. Must be divisible by 16.
	Height int `json:"height,omitempty"` // same defaults as Width.

	// ---- Sampling (generic) ----

	CFGScale float64 `json:"cfgScale,omitempty"` // SD1/SDXL default 7 (range 0–30). Flux1 sdcpp default 3.5 (range 1–20).
	Steps    int     `json:"steps,omitempty"`    // SD1/SDXL sdcpp default 20. Flux1 sdcpp default 28. Comfy default 30.
	Quantity int     `json:"quantity,omitempty"` // Images per call. SD1/SDXL: 1–12. Flux1: 1–4.
	Seed     int64   `json:"seed,omitempty"`     // Pin for reproducibility; random if omitted.

	// ---- sdcpp-only sampler/schedule ----

	// SampleMethod selects the sampler on Engine=sdcpp.
	// Use SdCppSample* constants. Default: "euler".
	// NOT valid on Engine=comfy — use Sampler instead. Sending both returns 400.
	SampleMethod string `json:"sampleMethod,omitempty"`

	// Schedule selects the noise schedule on Engine=sdcpp.
	// Use SdCppSchedule* constants. Default: "discrete" (SD1/SDXL), "simple" (Flux1).
	// NOT valid on Engine=comfy — use Scheduler instead. Sending both returns 400.
	Schedule string `json:"schedule,omitempty"`

	// UCache controls the step-caching strategy on Engine=sdcpp.
	// Use SdCppUCache* constants. Leave empty to use the worker default.
	// NOT valid on Engine=comfy.
	UCache string `json:"uCache,omitempty"`

	// ---- comfy-only sampler/scheduler ----

	// Sampler selects the sampler on Engine=comfy.
	// Use ComfySampler* constants. Common pick: "dpmpp_2m" + Scheduler "karras".
	// NOT valid on Engine=sdcpp — use SampleMethod instead. Sending both returns 400.
	Sampler string `json:"sampler,omitempty"`

	// Scheduler selects the noise schedule on Engine=comfy.
	// Use ComfyScheduler* constants.
	// NOT valid on Engine=sdcpp — use Schedule instead. Sending both returns 400.
	Scheduler string `json:"scheduler,omitempty"`

	// ---- SD1-only ----

	// ClipSkip controls CLIP layer skipping. SD1 only — SDXL will 400 if this is set.
	// -1 = model default; 2 is a common hand-tuned value for many SD1 checkpoints.
	ClipSkip int `json:"clipSkip,omitempty"`

	// ---- Flux1 sdcpp component overrides ----
	// All three default to well-tested GGUF releases; only override if you need a specific quantization.
	// NOT valid outside Ecosystem=flux1 with Engine=sdcpp.

	VaeModel   string `json:"vaeModel,omitempty"`   // Override the VAE component. Also accepted by SD1/SDXL sdcpp.
	ClipLModel string `json:"clipLModel,omitempty"` // Override CLIP-L encoder. Flux1 sdcpp only.
	T5XXLModel string `json:"t5XXLModel,omitempty"` // Override T5-XXL text encoder. Flux1 sdcpp only.

	// ---- LoRAs and embeddings ----

	LoRAs LoRAMap `json:"loras,omitempty"` // AIR URN → strength. SD1/SDXL: 0.0–1.0 typical. Flux1: up to 2.0.

	// Embeddings are textual-inversion AIR URNs. Reference them by embedding name in Prompt/NegativePrompt.
	// sdcpp only (SD1 and SDXL). NOT valid on Engine=comfy or Flux ecosystems.
	Embeddings []string `json:"embeddings,omitempty"`

	// ---- img2img / editing fields (Operation=createVariant or editImage) ----

	// Image is the source image URL for createVariant. Plain string — not a {url:…} wrapper.
	// Used by both sdcpp and comfy for createVariant.
	Image string `json:"image,omitempty"`

	// Images accepts 1–2 source URLs for Operation=editImage (Flux1 sdcpp only).
	// Use this instead of Image when performing prompt-driven edits.
	Images []string `json:"images,omitempty"`

	// Strength controls how much of the source image to preserve on Engine=sdcpp with Operation=createVariant.
	// Range 0.0–1.0; 0.6–0.8 is the "keep composition, change style" sweet spot.
	// NOT valid on Engine=comfy — use DenoiseStrength instead. Sending both returns 400.
	Strength float64 `json:"strength,omitempty"`

	// DenoiseStrength is the comfy equivalent of Strength.
	// Range 0.0–1.0; default 0.75.
	// NOT valid on Engine=sdcpp — use Strength instead. Sending both returns 400.
	DenoiseStrength float64 `json:"denoiseStrength,omitempty"`
}

// ---------------------------------------------------------------------------
// Workflow output types
// ---------------------------------------------------------------------------

// ImageOutput is a single generated image in a step's output block.
type ImageOutput struct {
	ID  string `json:"id"`  // blob_… prefixed blob ID
	URL string `json:"url"` // signed, expiring HTTPS URL — refetch if expired
}

// ImageGenOutput is the output block of a succeeded imageGen step.
type ImageGenOutput struct {
	Images []ImageOutput `json:"images"`
}

// ---------------------------------------------------------------------------
// Workflow request / response wrappers
// ---------------------------------------------------------------------------

// ImageGenStep is a single workflow step with $type = "imageGen".
type ImageGenStep struct {
	Type  string        `json:"$type"` // always "imageGen"
	Input ImageGenInput `json:"input"`
}

// ImageWorkflowRequest is the body sent to POST /v2/consumer/workflows.
type ImageWorkflowRequest struct {
	Steps []ImageGenStep `json:"steps"`
}

// WorkflowStep is one step entry in a workflow response.
type WorkflowStep struct {
	Name   string          `json:"name"`
	Type   string          `json:"$type"`
	Status WorkflowStatus  `json:"status"`
	Output *ImageGenOutput `json:"output,omitempty"`
}

// ImageWorkflowResponse is returned by SubmitWorkflow and GetWorkflow.
type ImageWorkflowResponse struct {
	ID     string         `json:"id"` // wf_… ULID
	Status WorkflowStatus `json:"status"`
	Steps  []WorkflowStep `json:"steps"`
}
