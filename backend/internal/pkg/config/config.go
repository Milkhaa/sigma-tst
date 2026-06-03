package config

import (
	"os"
	"path/filepath"
)

type Config struct {
	Port        string
	ProjectRoot string
}

func GetConfig() *Config {
	wd, _ := os.Getwd()
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	return &Config{
		Port:        port,
		ProjectRoot: filepath.Clean(filepath.Join(wd, "..")),
	}
}
