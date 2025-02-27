package parrot

import (
	"fmt"
	"os"
	"strconv"

	"github.com/smartcontractkit/chainlink-testing-framework/lib/k8s/client"
	"github.com/smartcontractkit/chainlink-testing-framework/lib/k8s/config"
	"github.com/smartcontractkit/chainlink-testing-framework/lib/k8s/environment"
	"github.com/smartcontractkit/chainlink-testing-framework/lib/utils/projectpath"

	"github.com/rs/zerolog/log"
)

const (
	LocalURLsKey    = "parrot_local"
	InternalURLsKey = "parrot_internal"
)

type Props struct {
}

type Chart struct {
	Name    string
	Path    string
	Version string
	Props   *Props
	Values  *map[string]any
}

func (m Chart) IsDeploymentNeeded() bool {
	return true
}

func (m Chart) GetName() string {
	return m.Name
}

func (m Chart) GetPath() string {
	return m.Path
}

func (m Chart) GetVersion() string {
	return m.Version
}

func (m Chart) GetProps() any {
	return m.Props
}

func (m Chart) GetValues() *map[string]any {
	return m.Values
}

func (m Chart) GetLabels() map[string]string {
	return map[string]string{
		"chain.link/component": "parrot",
	}
}

func (m Chart) ExportData(e *environment.Environment) error {
	parrotLocal, err := e.Fwd.FindPort("parrot:0", "parrot", "http").As(client.LocalConnection, client.HTTP)
	if err != nil {
		return err
	}
	services, err := e.Client.ListServices(e.Cfg.Namespace, fmt.Sprintf("app=%s", m.Name))
	if err != nil {
		return err
	}
	var parrotInternal string
	if services != nil && len(services.Items) != 0 {
		parrotInternal = fmt.Sprintf("http://%s:80", services.Items[0].Name)
	} else {
		parrotInternal, err = e.Fwd.FindPort("parrot:0", "parrot", "http").As(client.RemoteConnection, client.HTTP)
		if err != nil {
			return err
		}
	}
	if e.Cfg.InsideK8s {
		parrotLocal = parrotInternal
	}

	e.URLs[LocalURLsKey] = []string{parrotLocal}
	e.URLs[InternalURLsKey] = []string{parrotInternal}
	log.Info().Str("Local Connection", parrotLocal).Str("Internal Connection", parrotInternal).Msg("Parrot")
	return nil
}

func defaultProps() map[string]any {
	internalRepo := os.Getenv(config.EnvVarInternalDockerRepo)
	parrotRepo := "kalverra/parrot"
	if internalRepo != "" {
		parrotRepo = fmt.Sprintf("%s/parrot", internalRepo)
	}

	return map[string]any{
		"image": map[string]any{
			"repository":      parrotRepo,
			"version":         "0.5.0", //TODO: Update to latest version
			"imagePullPolicy": "IfNotPresent",
		},
		"logLevel": "trace",
		"resources": map[string]any{
			"limits": map[string]any{
				"cpu":    "250m",
				"memory": "256Mi",
			},
		},
		"service": map[string]any{
			"type": "ClusterIP",
			"port": 80,
		},
		"persistence": map[string]any{
			"enabled":    true,
			"accessMode": "ReadWriteOnce",
			"size":       "1Gi",
			"mountPath":  "/app",
		},
	}
}

func New(props map[string]any) environment.ConnectedChart {
	return NewVersioned("", props)
}

// NewVersioned enables choosing a specific helm chart version
func NewVersioned(helmVersion string, props map[string]any) environment.ConnectedChart {
	dp := defaultProps()
	config.MustMerge(&dp, props)
	chartPath := "chainlink-qa/parrot"
	if b, err := strconv.ParseBool(os.Getenv(config.EnvVarLocalCharts)); err == nil && b {
		chartPath = fmt.Sprintf("%s/parrot", projectpath.ChartsRoot)
	}
	return Chart{
		Name:    "parrot",
		Path:    chartPath,
		Values:  &dp,
		Version: helmVersion,
	}
}
