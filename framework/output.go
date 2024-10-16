package framework

import (
	"fmt"
	"github.com/pelletier/go-toml/v2"
	"os"
	"path/filepath"
	"strings"
)

func NoCache() bool {
	return os.Getenv("NO_CACHE") == "true"
}

func getBaseConfigPath() (string, error) {
	configs := os.Getenv("CTF_CONFIGS")
	if configs == "" {
		return "", fmt.Errorf("no %s env var is provided, you should provide at least one test promtailConfig in TOML", EnvVarTestConfigs)
	}
	return strings.Split(configs, ",")[0], nil
}

func Store[T any](cfg *T) error {
	baseConfigPath, err := getBaseConfigPath()
	if err != nil {
		return err
	}
	L.Info().Str("OutputFile", baseConfigPath).Msg("Storing configuration output")
	d, err := toml.Marshal(cfg)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(DefaultConfigDir, baseConfigPath), d, os.ModePerm)
}
