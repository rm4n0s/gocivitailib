package gocivitailib

import (
	"errors"
	"fmt"
)

// Validate checks every step of the request, see ImageGenInput.Validate. Field
// names are prefixed with the step, such as "steps[0].clipSkip".
func (r ImageWorkflowRequest) Validate() error {
	if len(r.Steps) == 0 {
		return &ValidationError{Field: "steps", Rule: RuleRequired, Message: "at least one step is required"}
	}
	var errs []error
	for i, step := range r.Steps {
		for _, ve := range step.Input.violations() {
			ve.Field = fmt.Sprintf("steps[%d].%s", i, ve.Field)
			errs = append(errs, ve)
		}
	}
	return joinErrors(errs)
}

// Validate enforces the engine and ecosystem rules documented on the fields
// of ImageGenInput, so that a request Civitai would answer with HTTP 400 is
// rejected before it is sent. It returns nil, a single *ValidationError, or an
// errors.Join of several; errors.As finds the first of them.
//
// Rules that Civitai does not document are not checked, and neither are the
// values of Engine, Ecosystem and Operation, so that new ones keep working.
func (in ImageGenInput) Validate() error {
	var errs []error
	for _, ve := range in.violations() {
		errs = append(errs, ve)
	}
	return joinErrors(errs)
}

func joinErrors(errs []error) error {
	switch len(errs) {
	case 0:
		return nil
	case 1:
		return errs[0]
	}
	return errors.Join(errs...)
}

func (in ImageGenInput) violations() []*ValidationError {
	var out []*ValidationError
	add := func(field, rule, format string, args ...any) {
		out = append(out, &ValidationError{Field: field, Rule: rule, Message: fmt.Sprintf(format, args...)})
	}
	required := func(field, value string) {
		if value == "" {
			add(field, RuleRequired, "%s is required", field)
		}
	}

	required("engine", in.Engine)
	required("ecosystem", in.Ecosystem)
	required("operation", in.Operation)
	required("prompt", in.Prompt)

	sdcpp := in.Engine == EngineSdCpp
	comfy := in.Engine == EngineComfy

	// sdcpp-only fields.
	if comfy {
		for _, f := range []struct {
			name string
			set  bool
			use  string
		}{
			{"sampleMethod", in.SampleMethod != "", "sampler"},
			{"schedule", in.Schedule != "", "scheduler"},
			{"uCache", in.UCache != "", ""},
			{"strength", in.Strength != 0, "denoiseStrength"},
		} {
			if f.set {
				add(f.name, RuleNotAllowed, "%s is only valid on engine %s%s", f.name, EngineSdCpp, instead(f.use))
			}
		}
	}
	// comfy-only fields.
	if sdcpp {
		for _, f := range []struct {
			name string
			set  bool
			use  string
		}{
			{"sampler", in.Sampler != "", "sampleMethod"},
			{"scheduler", in.Scheduler != "", "schedule"},
			{"denoiseStrength", in.DenoiseStrength != 0, "strength"},
		} {
			if f.set {
				add(f.name, RuleNotAllowed, "%s is only valid on engine %s%s", f.name, EngineComfy, instead(f.use))
			}
		}
	}

	if in.ClipSkip != 0 && in.Ecosystem != "" && in.Ecosystem != EcosystemSD1 {
		add("clipSkip", RuleNotAllowed, "clipSkip is only valid on ecosystem %s, not %s", EcosystemSD1, in.Ecosystem)
	}

	fluxSdcpp := in.Ecosystem == EcosystemFlux1 && sdcpp
	switch {
	case fluxSdcpp:
		required("diffuserModel", in.DiffuserModel)
		if in.Model != "" {
			add("model", RuleNotAllowed, "model is not used by ecosystem %s on engine %s, set diffuserModel instead", EcosystemFlux1, EngineSdCpp)
		}
	case in.Engine == EngineFlux1Kontext:
		// The Kontext tier is managed by the provider: no checkpoint is
		// documented for it, so Model is not required.
		if in.DiffuserModel != "" {
			add("diffuserModel", RuleNotAllowed, "diffuserModel is only valid on ecosystem %s with engine %s", EcosystemFlux1, EngineSdCpp)
		}
	default:
		required("model", in.Model)
		if in.DiffuserModel != "" {
			add("diffuserModel", RuleNotAllowed, "diffuserModel is only valid on ecosystem %s with engine %s, use model instead", EcosystemFlux1, EngineSdCpp)
		}
	}

	for _, d := range []struct {
		name  string
		value int
	}{{"width", in.Width}, {"height", in.Height}} {
		switch {
		case d.value < 0:
			add(d.name, RuleOutOfRange, "%s must not be negative", d.name)
		case d.value%16 != 0:
			add(d.name, RuleMultipleOf16, "%s must be divisible by 16, got %d", d.name, d.value)
		}
	}

	// Quantity 0 means unset: the server default applies.
	switch lo, hi := quantityRange(in.Ecosystem); {
	case in.Quantity < 0:
		add("quantity", RuleOutOfRange, "quantity must not be negative")
	case hi > 0 && in.Quantity > hi:
		add("quantity", RuleOutOfRange, "quantity must be between %d and %d on ecosystem %s, got %d", lo, hi, in.Ecosystem, in.Quantity)
	}

	return out
}

// quantityRange returns the documented number of images per call. hi is 0 when
// the range is not documented.
func quantityRange(ecosystem Ecosystem) (lo, hi int) {
	switch ecosystem {
	case EcosystemSD1, EcosystemSDXL:
		return 1, 12
	case EcosystemFlux1:
		return 1, 4
	}
	return 1, 0
}

func instead(use string) string {
	if use == "" {
		return ""
	}
	return ", use " + use + " instead"
}
