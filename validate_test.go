package gocivitailib_test

import (
	"errors"
	"testing"

	cv "github.com/rm4n0s/gocivitailib"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// violations flattens the error of Validate into (field, rule) pairs.
func violations(t *testing.T, err error) map[string]string {
	t.Helper()
	out := map[string]string{}
	var walk func(error)
	walk = func(err error) {
		if j, ok := err.(interface{ Unwrap() []error }); ok {
			for _, e := range j.Unwrap() {
				walk(e)
			}
			return
		}
		var ve *cv.ValidationError
		require.True(t, errors.As(err, &ve), "not a *ValidationError: %v", err)
		out[ve.Field] = ve.Rule
	}
	if err != nil {
		walk(err)
	}
	return out
}

func TestImageGenInputValidate(t *testing.T) {
	flux := func(in *cv.ImageGenInput) {
		in.Ecosystem = cv.EcosystemFlux1
		in.Model = ""
		in.DiffuserModel = "urn:air:flux1:diffuser:civitai:1@2"
	}
	comfy := func(in *cv.ImageGenInput) { in.Engine = cv.EngineComfy }

	tests := []struct {
		name   string
		mutate func(in *cv.ImageGenInput)
		want   map[string]string // field -> rule; nil means valid
	}{
		{name: "valid sdxl on sdcpp"},
		{name: "valid sdcpp fields on sdcpp", mutate: func(in *cv.ImageGenInput) {
			in.SampleMethod, in.Schedule, in.UCache, in.Strength = cv.SdCppSampleEulerA, cv.SdCppScheduleKarras, cv.SdCppUCacheNormal, 0.7
		}},
		{name: "valid comfy fields on comfy", mutate: func(in *cv.ImageGenInput) {
			comfy(in)
			in.Sampler, in.Scheduler, in.DenoiseStrength = cv.ComfySamplerDPMPP2M, cv.ComfySchedulerKarras, 0.7
		}},
		{name: "valid sd1 with clip skip", mutate: func(in *cv.ImageGenInput) { in.Ecosystem, in.ClipSkip = cv.EcosystemSD1, 2 }},
		{name: "valid flux1 on sdcpp", mutate: flux},
		{name: "valid kontext without model", mutate: func(in *cv.ImageGenInput) {
			flux(in)
			in.Engine, in.DiffuserModel = cv.EngineFlux1Kontext, ""
		}},
		{name: "unknown engine and ecosystem are not rejected", mutate: func(in *cv.ImageGenInput) {
			in.Engine, in.Ecosystem = "future", "future"
			in.Sampler, in.SampleMethod = "a", "b"
		}},

		// routing
		{name: "missing engine", mutate: func(in *cv.ImageGenInput) { in.Engine = "" }, want: map[string]string{"engine": cv.RuleRequired}},
		{name: "missing ecosystem", mutate: func(in *cv.ImageGenInput) { in.Ecosystem = "" }, want: map[string]string{"ecosystem": cv.RuleRequired}},
		{name: "missing operation", mutate: func(in *cv.ImageGenInput) { in.Operation = "" }, want: map[string]string{"operation": cv.RuleRequired}},
		{name: "missing prompt", mutate: func(in *cv.ImageGenInput) { in.Prompt = "" }, want: map[string]string{"prompt": cv.RuleRequired}},

		// sdcpp-only fields on comfy
		{name: "sampleMethod on comfy", mutate: func(in *cv.ImageGenInput) { comfy(in); in.SampleMethod = "euler" }, want: map[string]string{"sampleMethod": cv.RuleNotAllowed}},
		{name: "schedule on comfy", mutate: func(in *cv.ImageGenInput) { comfy(in); in.Schedule = "karras" }, want: map[string]string{"schedule": cv.RuleNotAllowed}},
		{name: "uCache on comfy", mutate: func(in *cv.ImageGenInput) { comfy(in); in.UCache = "normal" }, want: map[string]string{"uCache": cv.RuleNotAllowed}},
		{name: "strength on comfy", mutate: func(in *cv.ImageGenInput) { comfy(in); in.Strength = 0.5 }, want: map[string]string{"strength": cv.RuleNotAllowed}},

		// comfy-only fields on sdcpp
		{name: "sampler on sdcpp", mutate: func(in *cv.ImageGenInput) { in.Sampler = "euler" }, want: map[string]string{"sampler": cv.RuleNotAllowed}},
		{name: "scheduler on sdcpp", mutate: func(in *cv.ImageGenInput) { in.Scheduler = "karras" }, want: map[string]string{"scheduler": cv.RuleNotAllowed}},
		{name: "denoiseStrength on sdcpp", mutate: func(in *cv.ImageGenInput) { in.DenoiseStrength = 0.5 }, want: map[string]string{"denoiseStrength": cv.RuleNotAllowed}},

		// the combination the original integration test used
		{name: "original integration test input", mutate: func(in *cv.ImageGenInput) {
			in.Scheduler, in.DenoiseStrength, in.ClipSkip = cv.SdCppSampleEulerA, 0.8, 2
		}, want: map[string]string{"scheduler": cv.RuleNotAllowed, "denoiseStrength": cv.RuleNotAllowed, "clipSkip": cv.RuleNotAllowed}},

		// clipSkip
		{name: "clipSkip on sdxl", mutate: func(in *cv.ImageGenInput) { in.ClipSkip = 2 }, want: map[string]string{"clipSkip": cv.RuleNotAllowed}},
		{name: "clipSkip -1 on sdxl", mutate: func(in *cv.ImageGenInput) { in.ClipSkip = -1 }, want: map[string]string{"clipSkip": cv.RuleNotAllowed}},
		{name: "clipSkip on flux1", mutate: func(in *cv.ImageGenInput) { flux(in); in.ClipSkip = 2 }, want: map[string]string{"clipSkip": cv.RuleNotAllowed}},

		// model selection
		{name: "model missing on sdxl", mutate: func(in *cv.ImageGenInput) { in.Model = "" }, want: map[string]string{"model": cv.RuleRequired}},
		{name: "model missing on comfy flux1", mutate: func(in *cv.ImageGenInput) { comfy(in); flux(in); in.DiffuserModel = "" }, want: map[string]string{"model": cv.RuleRequired}},
		{name: "diffuserModel on sdxl", mutate: func(in *cv.ImageGenInput) { in.DiffuserModel = "urn:x" }, want: map[string]string{"diffuserModel": cv.RuleNotAllowed}},
		{name: "diffuserModel on comfy flux1", mutate: func(in *cv.ImageGenInput) { comfy(in); flux(in); in.Model = "urn:m" }, want: map[string]string{"diffuserModel": cv.RuleNotAllowed}},
		{name: "diffuserModel missing on flux1 sdcpp", mutate: func(in *cv.ImageGenInput) { flux(in); in.DiffuserModel = "" }, want: map[string]string{"diffuserModel": cv.RuleRequired}},
		{name: "model on flux1 sdcpp", mutate: func(in *cv.ImageGenInput) { flux(in); in.Model = "urn:m" }, want: map[string]string{"model": cv.RuleNotAllowed}},
		{name: "diffuserModel on kontext", mutate: func(in *cv.ImageGenInput) {
			flux(in)
			in.Engine = cv.EngineFlux1Kontext
		}, want: map[string]string{"diffuserModel": cv.RuleNotAllowed}},

		// dimensions
		{name: "width not divisible by 16", mutate: func(in *cv.ImageGenInput) { in.Width = 1000 }, want: map[string]string{"width": cv.RuleMultipleOf16}},
		{name: "height not divisible by 16", mutate: func(in *cv.ImageGenInput) { in.Height = 513 }, want: map[string]string{"height": cv.RuleMultipleOf16}},
		{name: "negative width", mutate: func(in *cv.ImageGenInput) { in.Width = -16 }, want: map[string]string{"width": cv.RuleOutOfRange}},
		{name: "unset dimensions use the server default", mutate: func(in *cv.ImageGenInput) { in.Width, in.Height = 0, 0 }},
		{name: "width 16", mutate: func(in *cv.ImageGenInput) { in.Width = 16 }},

		// quantity
		{name: "quantity 1 on sdxl", mutate: func(in *cv.ImageGenInput) { in.Quantity = 1 }},
		{name: "quantity 12 on sdxl", mutate: func(in *cv.ImageGenInput) { in.Quantity = 12 }},
		{name: "quantity 13 on sdxl", mutate: func(in *cv.ImageGenInput) { in.Quantity = 13 }, want: map[string]string{"quantity": cv.RuleOutOfRange}},
		{name: "quantity 12 on sd1", mutate: func(in *cv.ImageGenInput) { in.Ecosystem, in.Quantity = cv.EcosystemSD1, 12 }},
		{name: "quantity 13 on sd1", mutate: func(in *cv.ImageGenInput) { in.Ecosystem, in.Quantity = cv.EcosystemSD1, 13 }, want: map[string]string{"quantity": cv.RuleOutOfRange}},
		{name: "quantity 4 on flux1", mutate: func(in *cv.ImageGenInput) { flux(in); in.Quantity = 4 }},
		{name: "quantity 5 on flux1", mutate: func(in *cv.ImageGenInput) { flux(in); in.Quantity = 5 }, want: map[string]string{"quantity": cv.RuleOutOfRange}},
		{name: "negative quantity", mutate: func(in *cv.ImageGenInput) { in.Quantity = -1 }, want: map[string]string{"quantity": cv.RuleOutOfRange}},

		// several rules at once
		{name: "several rules", mutate: func(in *cv.ImageGenInput) {
			in.Width, in.Quantity, in.ClipSkip, in.Prompt = 100, 99, 2, ""
		}, want: map[string]string{"width": cv.RuleMultipleOf16, "quantity": cv.RuleOutOfRange, "clipSkip": cv.RuleNotAllowed, "prompt": cv.RuleRequired}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := validInput()
			if tt.mutate != nil {
				tt.mutate(&in)
			}
			err := in.Validate()
			if tt.want == nil {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Equal(t, tt.want, violations(t, err))

			var ve *cv.ValidationError
			require.True(t, errors.As(err, &ve), "errors.As must find a *ValidationError")
			assert.NotEmpty(t, ve.Message)
			if len(tt.want) == 1 {
				_, joined := err.(interface{ Unwrap() []error })
				assert.False(t, joined, "a single violation is returned unjoined")
			} else {
				_, joined := err.(interface{ Unwrap() []error })
				assert.True(t, joined, "several violations are joined")
			}
		})
	}
}

func TestImageWorkflowRequestValidate(t *testing.T) {
	second := validInput()
	second.Width = 7
	req := cv.ImageWorkflowRequest{Steps: []cv.ImageGenStep{
		{Type: cv.TypeImageGen, Input: validInput()},
		{Type: cv.TypeImageGen, Input: second},
	}}
	assert.Equal(t, map[string]string{"steps[1].width": cv.RuleMultipleOf16}, violations(t, req.Validate()))
	assert.Equal(t, map[string]string{"steps": cv.RuleRequired}, violations(t, cv.ImageWorkflowRequest{}.Validate()))
	assert.NoError(t, validRequest().Validate())
}
