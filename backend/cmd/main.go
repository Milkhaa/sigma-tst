package main

import (
	"sigma-tst/backend/internal/app"
	"sigma-tst/backend/internal/pkg/config"
)

func main() {
	cfg := config.GetConfig()
	a := &app.App{Config: cfg}
	a.Serve()
}
