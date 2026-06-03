package generate

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/rs/zerolog/log"

	"sigma-tst/backend/internal/pkg/inspect"
	"sigma-tst/backend/internal/pkg/llm"
	"sigma-tst/backend/internal/pkg/respond"
	"sigma-tst/backend/internal/pkg/schema"
	"sigma-tst/backend/internal/pkg/types"
)

type Impl struct {
	ProjectRoot string
}

func (impl *Impl) Handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			respond.JSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
			return
		}
		impl.generate(w, r)
	}
}

func (impl *Impl) generate(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	log.Info().Str("remote", r.RemoteAddr).Msg("generate.start")

	var req types.GenerateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Error().Err(err).Msg("generate.decode_error")
		respond.JSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}

	log.Info().Int("intent_len", len(req.Intent)).Bool("force_drift", req.ForceDriftFallback).Msg("generate.intent")

	snapshot, inspectErr := inspect.Page(impl.ProjectRoot, req.Intent)
	if inspectErr != nil {
		log.Warn().Err(inspectErr).Msg("generate.inspect_error")
	}

	spec, usedFallback, err := llm.GenerateSpec(r.Context(), req.Intent, snapshot)
	if err != nil {
		log.Error().Err(err).Dur("duration", time.Since(start)).Msg("generate.llm_error")
		respond.JSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}

	inspect.GroundSpec(spec, snapshot)
	schema.NormalizeSpec(spec)
	if err := inspect.ProgressiveGroundSpec(impl.ProjectRoot, spec); err != nil {
		log.Warn().Err(err).Msg("generate.progressive_ground_error")
	}
	if req.ForceDriftFallback {
		llm.InjectDriftFallbackDemo(spec)
	}
	schema.NormalizeSpec(spec)

	errs := schema.Validate(spec)
	res := types.GenerateResponse{Spec: spec, UsedFallback: usedFallback}
	res.Validation.Valid = len(errs) == 0
	res.Validation.Errors = errs
	for _, step := range spec.Steps {
		if step.Ungrounded {
			note := step.GroundingNote
			if note == "" {
				note = "selector could not be confirmed against the live page"
			}
			res.GroundingWarnings = append(res.GroundingWarnings, "step "+step.ID+": "+note)
		}
	}

	log.Info().Bool("valid", res.Validation.Valid).Bool("used_fallback", usedFallback).Dur("duration", time.Since(start)).Msg("generate.end")
	respond.JSON(w, http.StatusOK, res)
}
