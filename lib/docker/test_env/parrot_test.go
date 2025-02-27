package test_env

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/smartcontractkit/chainlink-testing-framework/lib/docker"
	"github.com/smartcontractkit/chainlink-testing-framework/lib/logging"
	"github.com/smartcontractkit/chainlink-testing-framework/parrot"
)

func TestParrot(t *testing.T) {
	t.Parallel()

	l := logging.GetTestLogger(t)
	network, err := docker.CreateNetwork(l)
	require.NoError(t, err)

	p := NewParrot([]string{network.Name}).WithTestInstance(t)
	err = p.StartContainer()
	require.NoError(t, err)

	route := &parrot.Route{
		Method:             http.MethodGet,
		Path:               "/test",
		RawResponseBody:    "Squawk",
		ResponseStatusCode: http.StatusOK,
	}
	err = p.Client.RegisterRoute(route)
	require.NoError(t, err, "failed to register route")

	resp, err := p.Client.CallRoute(route.Method, route.Path)
	require.NoError(t, err, "failed to call route")
	require.Equal(t, http.StatusOK, resp.StatusCode())
	require.Equal(t, "Squawk", string(resp.Body()))
}
