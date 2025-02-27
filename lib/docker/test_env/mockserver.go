package test_env

import (
	"fmt"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	tc "github.com/testcontainers/testcontainers-go"
	tcwait "github.com/testcontainers/testcontainers-go/wait"

	ctfClient "github.com/smartcontractkit/chainlink-testing-framework/lib/client"
	"github.com/smartcontractkit/chainlink-testing-framework/lib/docker"
	"github.com/smartcontractkit/chainlink-testing-framework/lib/logging"
	"github.com/smartcontractkit/chainlink-testing-framework/lib/mirror"
	"github.com/smartcontractkit/chainlink-testing-framework/lib/utils/testcontext"
)

const defaultMockServerImage = "mockserver/mockserver:5.15.0"

// Deprecated: Use Parrot instead
type MockServer struct {
	EnvComponent
	//nolint:staticcheck // Ignore SA1019: MockserverClient is deprecated
	Client           *ctfClient.MockserverClient
	Endpoint         string
	InternalEndpoint string
	EAMockUrls       []*url.URL
	t                *testing.T
	l                zerolog.Logger
}

// NewMockServer creates a new instance of MockServer with specified networks and options.
// It initializes the server with a unique container name and a default startup timeout.
// This function is useful for testing decentralized applications in a controlled environment.
//
// Deprecated: Use Parrot instead
func NewMockServer(networks []string, opts ...EnvComponentOption) *MockServer {
	ms := &MockServer{
		EnvComponent: EnvComponent{
			ContainerName:  fmt.Sprintf("%s-%s", "mockserver", uuid.NewString()[0:8]),
			Networks:       networks,
			StartupTimeout: 1 * time.Minute,
		},
		l: log.Logger,
	}
	for _, opt := range opts {
		opt(&ms.EnvComponent)
	}
	return ms
}

// WithTestInstance configures the MockServer with a test logger and test context.
// It returns the updated MockServer instance for use in testing scenarios.
func (ms *MockServer) WithTestInstance(t *testing.T) *MockServer {
	ms.l = logging.GetTestLogger(t)
	ms.t = t
	return ms
}

// SetExternalAdapterMocks configures a specified number of mock external adapter endpoints.
// It generates unique paths for each adapter and stores their URLs for later use.
// This function is useful for testing scenarios that require multiple external adapter interactions.
func (ms *MockServer) SetExternalAdapterMocks(count int) error {
	for i := 0; i < count; i++ {
		path := fmt.Sprintf("/ea-%d", i)
		err := ms.Client.SetRandomValuePath(path)
		if err != nil {
			return err
		}
		cName, err := ms.Container.Name(testcontext.Get(ms.t))
		if err != nil {
			return err
		}
		cName = strings.Replace(cName, "/", "", -1)
		eaUrl, err := url.Parse(fmt.Sprintf("http://%s:%s%s",
			cName, "1080", path))
		if err != nil {
			return err
		}
		ms.EAMockUrls = append(ms.EAMockUrls, eaUrl)
	}
	return nil
}

// StartContainer initializes and starts a MockServer container.
// It sets up logging, retrieves the container request, and establishes endpoints for communication.
// This function is essential for testing environments that require a mock server instance.
func (ms *MockServer) StartContainer() error {
	l := logging.GetTestContainersGoTestLogger(ms.t)
	cr, err := ms.getContainerRequest()
	if err != nil {
		return err
	}
	c, err := docker.StartContainerWithRetry(ms.l, tc.GenericContainerRequest{
		ContainerRequest: cr,
		Reuse:            true,
		Started:          true,
		Logger:           l,
	})
	if err != nil {
		return fmt.Errorf("cannot start MockServer container: %w", err)
	}
	ms.Container = c
	endpoint, err := GetEndpoint(testcontext.Get(ms.t), c, "http")
	if err != nil {
		return err
	}
	ms.l.Info().Any("endpoint", endpoint).Str("containerName", ms.ContainerName).
		Msgf("Started MockServer container")
	ms.Endpoint = endpoint
	ms.InternalEndpoint = fmt.Sprintf("http://%s:%s", ms.ContainerName, "1080")

	//nolint:staticcheck // Ignore SA1019: client.NewMockserverClient is deprecated
	client := ctfClient.NewMockserverClient(&ctfClient.MockserverConfig{
		LocalURL:   endpoint,
		ClusterURL: ms.InternalEndpoint,
	})
	ms.Client = client

	return nil
}

func (ms *MockServer) getContainerRequest() (tc.ContainerRequest, error) {
	msImage := mirror.AddMirrorToImageIfSet(defaultMockServerImage)

	return tc.ContainerRequest{
		Name:         ms.ContainerName,
		Image:        msImage,
		ExposedPorts: []string{"1080/tcp"},
		Env: map[string]string{
			"SERVER_PORT": "1080",
		},
		Networks: ms.Networks,
		WaitingFor: tcwait.ForLog("INFO 1080 started on port: 1080").
			WithPollInterval(100 * time.Millisecond).WithStartupTimeout(ms.StartupTimeout),
		LifecycleHooks: []tc.ContainerLifecycleHooks{
			{
				PostStarts: ms.PostStartsHooks,
				PostStops:  ms.PostStopsHooks,
			},
		},
	}, nil
}
