package test_env

import (
	"bytes"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/smartcontractkit/chainlink-testing-framework/lib/docker"
	"github.com/smartcontractkit/chainlink-testing-framework/lib/logging"
)

func TestMockServerSetStringValue(t *testing.T) {
	l := logging.GetTestLogger(t)
	network, err := docker.CreateNetwork(l)
	require.NoError(t, err)

	m := NewMockServer([]string{network.Name}).
		WithTestInstance(t)
	err = m.StartContainer()
	require.NoError(t, err)

	expected := "bar"
	path := "/foo"
	err = m.Client.SetStringValuePath(path, expected)
	require.NoError(t, err)

	//nolint:staticcheck // Ignore SA1019: client.NewMockserverClient is deprecated
	url := fmt.Sprintf("%s%s", m.Client.LocalURL(), path)
	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	resp, err := client.Get(url)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	buf := new(bytes.Buffer)
	_, err = buf.ReadFrom(resp.Body)
	require.NoError(t, err)

	responseString := buf.String()
	require.Equal(t, expected, responseString)
}
