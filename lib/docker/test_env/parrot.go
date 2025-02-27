package test_env

import (
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	tc "github.com/testcontainers/testcontainers-go"
	tcwait "github.com/testcontainers/testcontainers-go/wait"

	"github.com/smartcontractkit/chainlink-testing-framework/lib/docker"
	"github.com/smartcontractkit/chainlink-testing-framework/lib/k8s/environment"
	helm_parrot "github.com/smartcontractkit/chainlink-testing-framework/lib/k8s/pkg/helm/parrot"
	"github.com/smartcontractkit/chainlink-testing-framework/lib/logging"
	"github.com/smartcontractkit/chainlink-testing-framework/lib/mirror"
	"github.com/smartcontractkit/chainlink-testing-framework/lib/utils/testcontext"
	"github.com/smartcontractkit/chainlink-testing-framework/parrot"
)

const (
	defaultParrotImage   = "kalverra/parrot"
	defaultParrotVersion = "v0.5.0"
	defaultParrotPort    = "80"
)

// Parrot is a test environment component that wraps a Parrot server.
type Parrot struct {
	EnvComponent
	Client           *parrot.Client
	ExternalEndpoint string
	InternalEndpoint string
	t                *testing.T
	l                zerolog.Logger
}

// ParrotAdapterResponse imitates the standard response from a Chainlink external adapter.
type ParrotAdapterResponse struct {
	ID    string              `json:"id"`
	Data  ParrotAdapterResult `json:"data"`
	Error any                 `json:"error"`
}

// ParrotAdapterResult is the data field of the ParrotAdapterResponse.
type ParrotAdapterResult struct {
	Result any `json:"result"`
}

// NewParrot creates a new instance of ParrotServer with specified networks and options.
// It initializes the server with a unique container name and a default startup timeout.
// This function is useful for testing decentralized applications in a controlled environment.
func NewParrot(networks []string, opts ...EnvComponentOption) *Parrot {
	p := &Parrot{
		EnvComponent: EnvComponent{
			ContainerName:  fmt.Sprintf("%s-%s", "parrot", uuid.NewString()[0:3]),
			Networks:       networks,
			StartupTimeout: 10 * time.Second,
		},
		l: log.Logger,
	}
	p.SetDefaultHooks()
	for _, opt := range opts {
		opt(&p.EnvComponent)
	}
	return p
}

// ConnectParrot connects to an existing Parrot server with the specified URL.
func ConnectParrot(url string) *Parrot {
	return &Parrot{
		Client: parrot.NewClient(url),
	}
}

// ConnectParrotTestEnv connects to an existing Parrot server in a running test environment.
func ConnectParrotTestEnv(e *environment.Environment) *Parrot {
	return &Parrot{
		Client: parrot.NewClient(e.URLs[helm_parrot.LocalURLsKey][0]),
	}
}

// WithTestInstance configures the MockServer with a test logger and test context.
// It returns the updated MockServer instance for use in testing scenarios.
func (p *Parrot) WithTestInstance(t *testing.T) *Parrot {
	p.l = logging.GetTestLogger(t)
	p.t = t
	return p
}

// StartContainer initializes and starts a Parrot container.
// It sets up logging, retrieves the container request, and establishes endpoints for communication.
// This function is essential for testing environments that require a mock server instance.
func (p *Parrot) StartContainer() error {
	l := logging.GetTestContainersGoTestLogger(p.t)
	cr, err := p.getContainerRequest()
	if err != nil {
		return err
	}
	c, err := docker.StartContainerWithRetry(p.l, tc.GenericContainerRequest{
		ContainerRequest: cr,
		Reuse:            true,
		Started:          true,
		Logger:           l,
	})
	if err != nil {
		return fmt.Errorf("cannot start Parrot container: %w", err)
	}
	p.Container = c
	endpoint, err := GetEndpoint(testcontext.Get(p.t), c, "http")
	if err != nil {
		return err
	}
	p.ExternalEndpoint = endpoint
	p.InternalEndpoint = fmt.Sprintf("http://%s", p.ContainerName)

	p.Client = parrot.NewClient(p.ExternalEndpoint)

	p.l.Info().Str("External Endpoint", p.ExternalEndpoint).
		Str("Internal Endpoint", p.InternalEndpoint).
		Str("Container Name", p.ContainerName).
		Msg("Started Parrot Container")
	return nil
}

func (p *Parrot) getContainerRequest() (tc.ContainerRequest, error) {
	pImage := mirror.AddMirrorToImageIfSet("parrot")
	if pImage == "" || pImage == "parrot" {
		pImage = defaultParrotImage
	}
	pImage = fmt.Sprintf("%s:%s", pImage, defaultParrotVersion)

	return tc.ContainerRequest{
		Name:         p.ContainerName,
		Image:        pImage,
		ExposedPorts: []string{NatPortFormat(defaultParrotPort)},
		Networks:     p.Networks,
		Env: map[string]string{
			"PARROT_PORT":      defaultParrotPort,
			"PARROT_LOG_LEVEL": "trace",
			"PARROT_HOST":      "0.0.0.0",
		},
		WaitingFor: tcwait.ForHealthCheck().
			WithPollInterval(100 * time.Millisecond).WithStartupTimeout(p.StartupTimeout),
		LifecycleHooks: []tc.ContainerLifecycleHooks{
			{
				PostStarts: p.PostStartsHooks,
				PostStops:  p.PostStopsHooks,
			},
		},
	}, nil
}

// SetAdapterRoute sets a new route for the mock external adapter, wrapping the provided response in a standard adapter response.
// If you don't want to wrap the response, use Client.RegisterRoute directly.
func (p *Parrot) SetAdapterRoute(route *parrot.Route) error {
	var result any
	if route.RawResponseBody != "" {
		result = route.RawResponseBody
	} else {
		result = route.ResponseBody
	}
	ar := ParrotAdapterResponse{
		ID: uuid.NewString(),
		Data: ParrotAdapterResult{
			Result: result,
		},
		Error: nil,
	}

	return p.Client.RegisterRoute(&parrot.Route{
		Method:             route.Method,
		Path:               route.Path,
		ResponseBody:       ar,
		ResponseStatusCode: route.ResponseStatusCode,
	})
}
