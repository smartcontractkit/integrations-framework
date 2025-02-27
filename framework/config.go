package framework

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"text/template"
	"time"

	"github.com/davecgh/go-spew/spew"
	"github.com/go-playground/locales/en"
	ut "github.com/go-playground/universal-translator"
	"github.com/go-playground/validator/v10"
	"github.com/pelletier/go-toml/v2"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/network"
)

const (
	DefaultConfigDir = "."
)

const (
	EnvVarTestConfigs       = "CTF_CONFIGS"
	EnvVarAWSSecretsManager = "CTF_AWS_SECRETS_MANAGER"
	// EnvVarCI this is a default env variable many CI runners use so code can detect we run in CI
	EnvVarCI = "CI"
)

const (
	OutputFieldNameTOML = "out"
	OutputFieldName     = "Out"
	OverridesFieldName  = "Overrides"
)

var (
	once = &sync.Once{}
	// Secrets is a singleton AWS Secrets Manager
	// Loaded once on start inside Load and is safe to call concurrently
	Secrets *AWSSecretsManager

	DefaultNetworkName string

	AllowedEmptyConfigurationFields = []string{OutputFieldName, OverridesFieldName}

	Validator *validator.Validate = validator.New(validator.WithRequiredStructEnabled())

	ValidatorTranslator ut.Translator
)

func init() {
	eng := en.New()
	uni := ut.New(eng, eng)
	ValidatorTranslator, _ = uni.GetTranslator("en")
}

type ValidationError struct {
	Field   string
	Value   interface{}
	Message string
}

// mergeInputs merges all EnvVarTestConfigs filenames into one files, starting from the last and applying to the first
func mergeInputs[T any]() (*T, error) {
	var config T
	paths := strings.Split(os.Getenv(EnvVarTestConfigs), ",")
	_, err := getBaseConfigPath()
	if err != nil {
		return nil, err
	}
	for _, path := range paths {
		L.Info().Str("Path", path).Msg("Loading configuration input")
		data, err := os.ReadFile(filepath.Join(DefaultConfigDir, path))
		if err != nil {
			return nil, fmt.Errorf("error reading config file %s: %w", path, err)
		}
		if L.GetLevel() == zerolog.DebugLevel {
			fmt.Println(string(data))
		}

		decoder := toml.NewDecoder(strings.NewReader(string(data)))
		decoder.DisallowUnknownFields()

		if err := decoder.Decode(&config); err != nil {
			var details *toml.StrictMissingError
			if errors.As(err, &details) {
				fmt.Println(details.String())
			}
			return nil, fmt.Errorf("failed to decode TOML config, strict mode: %s", err)
		}
	}
	if L.GetLevel() == zerolog.DebugLevel {
		L.Debug().Msg("Merged inputs")
		spew.Dump(config)
	}
	return &config, nil
}

func validateWithCustomErr(cfg interface{}) []ValidationError {
	var validationErrors []ValidationError
	err := Validator.Struct(cfg)
	if err != nil {
		for _, err := range err.(validator.ValidationErrors) {
			customMessage := err.Translate(ValidatorTranslator)
			defaultMessage := fmt.Sprintf("validation failed on '%s' with tag '%s'", err.Field(), err.Tag())

			messageToUse := customMessage
			if strings.HasPrefix(customMessage, "validation failed") {
				messageToUse = defaultMessage
			}

			validationErrors = append(validationErrors, ValidationError{
				Field:   err.StructNamespace(),
				Value:   err.Value(),
				Message: messageToUse,
			})
		}
	}
	return validationErrors
}

func validate(s interface{}) error {
	errs := validateWithCustomErr(s)
	for _, e := range errs {
		L.Error().Any("error", e).Send()
	}
	if len(errs) > 0 {
		return fmt.Errorf("config validation failed\nwe are using 'go-playground/validator', please read more here: https://github.com/go-playground/validator?tab=readme-ov-file#usage-and-documentation")
	}
	return nil
}

func Load[X any](t *testing.T) (*X, error) {
	input, err := mergeInputs[X]()
	if err != nil {
		return input, err
	}
	if err := validate(input); err != nil {
		return nil, err
	}
	t.Cleanup(func() {
		err := Store[X](input)
		require.NoError(t, err)
	})
	// TODO: not all the people have AWS access, sadly enough, uncomment when granted
	//if os.Getenv(EnvVarAWSSecretsManager) == "true" {
	//	Secrets, err = NewAWSSecretsManager(1 * time.Minute)
	//	if err != nil {
	//		return nil, fmt.Errorf("failed to connect AWSSecretsManager: %w", err)
	//	}
	//}
	err = DefaultNetwork(once)
	require.NoError(t, err)
	return input, nil
}

func DefaultNetwork(once *sync.Once) error {
	var net *testcontainers.DockerNetwork
	var err error
	once.Do(func() {
		net, err = network.New(
			context.Background(),
			network.WithLabels(map[string]string{"framework": "ctf"}),
		)
		DefaultNetworkName = net.Name
	})
	return err
}

func RenderTemplate(tmpl string, data interface{}) (string, error) {
	var buf bytes.Buffer
	err := template.Must(template.New("tmpl").Parse(tmpl)).Execute(&buf, data)
	return buf.String(), err
}

func getBaseConfigPath() (string, error) {
	configs := os.Getenv("CTF_CONFIGS")
	if configs == "" {
		return "", fmt.Errorf("no %s env var is provided, you should provide at least one test config in TOML", EnvVarTestConfigs)
	}
	return strings.Split(configs, ",")[0], nil
}

func Store[T any](cfg *T) error {
	baseConfigPath, err := getBaseConfigPath()
	if err != nil {
		return err
	}
	newCacheName := strings.Replace(baseConfigPath, ".toml", "", -1)
	if strings.Contains(newCacheName, "cache") {
		L.Info().Str("Cache", baseConfigPath).Msg("Cache file already exists, skipping")
		return nil
	}
	cachedOutName := fmt.Sprintf("%s-cache.toml", strings.Replace(baseConfigPath, ".toml", "", -1))
	L.Info().Str("OutputFile", cachedOutName).Msg("Storing configuration output")
	d, err := toml.Marshal(cfg)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(DefaultConfigDir, cachedOutName), d, os.ModePerm)
}

// JSONStrDuration is JSON friendly duration that can be parsed from "1h2m0s" Go format
type JSONStrDuration struct {
	time.Duration
}

func (d *JSONStrDuration) MarshalJSON() ([]byte, error) {
	return json.Marshal(d.String())
}

func (d *JSONStrDuration) UnmarshalJSON(b []byte) error {
	var v interface{}
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	switch value := v.(type) {
	case string:
		var err error
		d.Duration, err = time.ParseDuration(value)
		if err != nil {
			return err
		}
		return nil
	default:
		return errors.New("invalid duration")
	}
}

// MustParseDuration parses a duration string in Go's format and returns the corresponding time.Duration.
// It panics if the string cannot be parsed, ensuring that the caller receives a valid duration.
func MustParseDuration(s string) time.Duration {
	d, err := time.ParseDuration(s)
	if err != nil {
		L.Fatal().Msg("cannot parse duration, should be Go format 1h2m3s")
	}
	return d
}
