package run

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/rs/zerolog/log"

	"sigma-tst/backend/internal/pkg/respond"
	"sigma-tst/backend/internal/pkg/runner"
	"sigma-tst/backend/internal/pkg/schema"
	"sigma-tst/backend/internal/pkg/store"
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
		impl.run(w, r)
	}
}

func (impl *Impl) run(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	log.Info().Str("remote", r.RemoteAddr).Msg("run.start")

	var req types.RunRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Error().Err(err).Msg("run.decode_error")
		respond.JSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}

	log.Info().Str("spec_name", req.Spec.Name).Int("steps", len(req.Spec.Steps)).Msg("run.spec")

	schema.NormalizeSpec(&req.Spec)
	if errs := schema.Validate(&req.Spec); len(errs) > 0 {
		log.Warn().Strs("errors", errs).Msg("run.validation_error")
		respond.JSON(w, http.StatusBadRequest, map[string]any{"error": "invalid spec", "details": errs})
		return
	}

	result, err := runner.Execute(req.Spec, impl.ProjectRoot)
	if err != nil {
		log.Error().Err(err).Dur("duration", time.Since(start)).Msg("run.exec_error")
		respond.JSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	if saved, err := store.SaveRunResult(impl.ProjectRoot, result); err == nil {
		result = saved
	}

	log.Info().Str("run_id", result.RunID).Str("status", result.Status).Dur("duration", time.Since(start)).Msg("run.end")
	respond.JSON(w, http.StatusOK, result)
}
