package app

import (
	"net/http"
	"os"

	"github.com/justinas/alice"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"sigma-tst/backend/internal/app/generate"
	"sigma-tst/backend/internal/app/promotions"
	apprun "sigma-tst/backend/internal/app/run"
	"sigma-tst/backend/internal/pkg/config"
	"sigma-tst/backend/internal/pkg/respond"
)

type App struct {
	Config *config.Config
}

func (a *App) Serve() {
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix

	handler := a.setupHandlers()
	log.Info().Str("port", a.Config.Port).Msg("API listening")
	if err := http.ListenAndServe(":"+a.Config.Port, handler); err != nil {
		log.Fatal().Err(err).Msg("server failed")
	}
}

func (a *App) setupHandlers() http.Handler {
	mux := http.NewServeMux()

	gen := &generate.Impl{ProjectRoot: a.Config.ProjectRoot}
	run := &apprun.Impl{ProjectRoot: a.Config.ProjectRoot}
	promo := &promotions.Impl{ProjectRoot: a.Config.ProjectRoot}

	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		respond.JSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	mux.Handle("/v1/tests/generate", gen.Handler())
	mux.Handle("/v1/tests/run", run.Handler())
	mux.Handle("/v1/promotions/", promo.Handler())

	return alice.New(corsMiddleware).Then(mux)
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
