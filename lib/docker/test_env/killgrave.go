package test_env

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/mount"
	"github.com/google/uuid"
	"github.com/otiai10/copy"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	tc "github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/smartcontractkit/chainlink-testing-framework/lib/k8s/config"
	"github.com/smartcontractkit/chainlink-testing-framework/lib/logging"
	"github.com/smartcontractkit/chainlink-testing-framework/lib/utils/testcontext"
)

const defaultKillgraveImage = "friendsofgo/killgrave:v0.5.1-request-dump"

// Deprecated: Use Parrot instead
type Killgrave struct {
	EnvComponent
	ExternalEndpoint      string
	InternalPort          string
	InternalEndpoint      string
	impostersPath         string
	impostersDirBinding   string
	requestDumpDirBinding string
	t                     *testing.T
	l                     zerolog.Logger
}

// Imposter define an imposter structure
//
// Deprecated: Use Parrot instead
type KillgraveImposter struct {
	Request  KillgraveRequest  `json:"request"`
	Response KillgraveResponse `json:"response"`
}

// Deprecated: Use Parrot instead
type KillgraveRequest struct {
	Method     string             `json:"method"`
	Endpoint   string             `json:"endpoint,omitempty"`
	SchemaFile *string            `json:"schemaFile,omitempty"`
	Params     *map[string]string `json:"params,omitempty"`
	Headers    *map[string]string `json:"headers"`
}

// Response represent the structure of real response
//
// Deprecated: Use Parrot instead
type KillgraveResponse struct {
	Status   int                     `json:"status"`
	Body     string                  `json:"body,omitempty"`
	BodyFile *string                 `json:"bodyFile,omitempty"`
	Headers  *map[string]string      `json:"headers,omitempty"`
	Delay    *KillgraveResponseDelay `json:"delay,omitempty"`
}

// ResponseDelay represent time delay before server responds.
//
// Deprecated: Use Parrot instead
type KillgraveResponseDelay struct {
	Delay  int64 `json:"delay,omitempty"`
	Offset int64 `json:"offset,omitempty"`
}

// AdapterResponse represents a response from an adapter
//
// Deprecated: Use Parrot instead
type KillgraveAdapterResponse struct {
	Id    string                 `json:"id"`
	Data  KillgraveAdapterResult `json:"data"`
	Error interface{}            `json:"error"`
}

// AdapterResult represents an int result for an adapter
//
// Deprecated: Use Parrot instead
type KillgraveAdapterResult struct {
	Result interface{} `json:"result"`
}

// NewKillgrave initializes a new Killgrave instance with specified networks and imposters directory.
// It sets default configurations and allows for optional environment component modifications.
// This function is useful for creating a Killgrave service for testing and simulating APIs.
//
// Deprecated: Use Parrot instead
func NewKillgrave(networks []string, impostersDirectoryPath string, opts ...EnvComponentOption) *Killgrave {
	k := &Killgrave{
		EnvComponent: EnvComponent{
			ContainerName:  fmt.Sprintf("%s-%s", "killgrave", uuid.NewString()[0:3]),
			Networks:       networks,
			StartupTimeout: 2 * time.Minute,
		},
		InternalPort:  "3000",
		impostersPath: impostersDirectoryPath,
		l:             log.Logger,
	}
	k.SetDefaultHooks()
	for _, opt := range opts {
		opt(&k.EnvComponent)
	}
	return k
}

// WithTestInstance sets up a Killgrave instance for testing by assigning a test logger and the testing context.
// This allows for better logging during tests and facilitates easier debugging.
func (k *Killgrave) WithTestInstance(t *testing.T) *Killgrave {
	k.l = logging.GetTestLogger(t)
	k.t = t
	return k
}

// StartContainer initializes and starts the Killgrave container, setting up imposters and request dumping.
// It also configures cleanup for the container and logs the external and internal endpoints for access.
func (k *Killgrave) StartContainer() error {
	err := k.setupImposters()
	if err != nil {
		return err
	}
	err = k.setupRequestDump()
	if err != nil {
		return err
	}
	if k.t != nil {
		k.t.Cleanup(func() {
			os.RemoveAll(k.impostersDirBinding)
		})
	}
	l := logging.GetTestContainersGoTestLogger(k.t)
	cr, err := k.getContainerRequest()
	if err != nil {
		return err
	}
	c, err := tc.GenericContainer(testcontext.Get(k.t), tc.GenericContainerRequest{
		ContainerRequest: cr,
		Started:          true,
		Reuse:            true,
		Logger:           l,
	})
	if err != nil {
		return fmt.Errorf("cannot start Killgrave container: %w", err)
	}
	endpoint, err := GetEndpoint(testcontext.Get(k.t), c, "http")
	if err != nil {
		return err
	}
	k.Container = c
	k.ExternalEndpoint = endpoint
	k.InternalEndpoint = fmt.Sprintf("http://%s:%s", k.ContainerName, k.InternalPort)

	k.l.Info().Str("External Endpoint", k.ExternalEndpoint).
		Str("Internal Endpoint", k.InternalEndpoint).
		Str("Container Name", k.ContainerName).
		Msgf("Started Killgrave Container")
	return nil
}

func (k *Killgrave) getContainerRequest() (tc.ContainerRequest, error) {
	// TT-1290 Temporary work around using fork of killgrave, uncomment line below when fork is merged
	// killgraveImage := mirror.AddMirrorToImageIfSet(defaultKillgraveImage)
	// TT-1290 Temporary code to set image to the fork or the ecr mirror depending on the config
	killgraveImage := "tateexon/killgrave:v0.5.1-request-dump"
	ecr := os.Getenv(config.EnvVarInternalDockerRepo)
	if ecr != "" {
		killgraveImage = fmt.Sprintf("%s/%s", ecr, defaultKillgraveImage)
	}
	// end temporary code

	return tc.ContainerRequest{
		Name:         k.ContainerName,
		Networks:     k.Networks,
		Image:        killgraveImage,
		ExposedPorts: []string{NatPortFormat(k.InternalPort)},
		Cmd:          []string{"-H=0.0.0.0", "-i=/imposters", "-w", "-v", "-d=/requestDump/requestDump.log"},
		HostConfigModifier: func(hostConfig *container.HostConfig) {
			hostConfig.Mounts = append(hostConfig.Mounts, mount.Mount{
				Type:     mount.TypeBind,
				Source:   k.impostersDirBinding,
				Target:   "/imposters",
				ReadOnly: false,
			}, mount.Mount{
				Type:     mount.TypeBind,
				Source:   k.requestDumpDirBinding,
				Target:   "/requestDump",
				ReadOnly: false,
			})
		},
		WaitingFor: wait.ForLog("The fake server is on tap now").WithStartupTimeout(k.StartupTimeout),
		LifecycleHooks: []tc.ContainerLifecycleHooks{
			{
				PostStarts: k.PostStartsHooks,
				PostStops:  k.PostStopsHooks,
			},
		},
	}, nil
}

func (k *Killgrave) setupImposters() error {
	// create temporary directory for imposters
	var err error
	k.impostersDirBinding, err = os.MkdirTemp(k.impostersDirBinding, "imposters*")
	if err != nil {
		return err
	}
	k.l.Info().Str("Path", k.impostersDirBinding).Msg("Imposters directory created at")

	// copy user imposters
	if len(k.impostersPath) != 0 {
		err = copy.Copy(k.impostersPath, k.impostersDirBinding)
		if err != nil {
			return err
		}
	}

	// add default five imposter
	return k.SetAdapterBasedIntValuePath("/five", []string{http.MethodGet, http.MethodPost}, 5)
}

func (k *Killgrave) setupRequestDump() error {
	// create temporary directory for request dumps
	var err error
	k.requestDumpDirBinding, err = os.MkdirTemp(k.requestDumpDirBinding, "requestDump*")
	return err
}

// AddImposter adds an imposter to the killgrave container
func (k *Killgrave) AddImposter(imposters []KillgraveImposter) error {
	// if the endpoint paths do not start with '/' then add it
	for i, imposter := range imposters {
		if !strings.HasPrefix(imposter.Request.Endpoint, "/") {
			imposter.Request.Endpoint = fmt.Sprintf("/%s", imposter.Request.Endpoint)
			imposters[i] = imposter
		}
	}

	req := imposters[0].Request
	data, err := json.Marshal(imposters)
	if err != nil {
		return err
	}

	// build the file name from the req.Endpoint
	unsafeFileName := strings.TrimPrefix(req.Endpoint, "/")
	safeFileName := strings.ReplaceAll(unsafeFileName, "/", ".")
	f, err := os.Create(filepath.Join(k.impostersDirBinding, fmt.Sprintf("%s.imp.json", safeFileName)))
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = f.WriteString(string(data))
	if err != nil {
		return err
	}

	// when adding default imposters, the container is not yet started and the container will be nil
	// this allows us to add them without having to wait for the imposter to load later
	if k.Container != nil {
		// wait for the log saying the imposter was loaded
		containerFile := filepath.Join("/imposters", fmt.Sprintf("%s.imp.json", safeFileName))
		logWaitStrategy := wait.ForLog(fmt.Sprintf("imposter %s loaded", containerFile)).WithStartupTimeout(15 * time.Second)
		err = logWaitStrategy.WaitUntilReady(testcontext.Get(k.t), k.Container)
	}
	return err
}

// SetStringValuePath sets a path to return a string value
func (k *Killgrave) SetStringValuePath(path string, methods []string, headers map[string]string, v string) error {
	imposters := []KillgraveImposter{}
	for _, method := range methods {
		imposters = append(imposters, KillgraveImposter{
			Request: KillgraveRequest{
				Method:   method,
				Endpoint: path,
			},
			Response: KillgraveResponse{
				Status:  200,
				Body:    v,
				Headers: &headers,
			},
		})
	}

	return k.AddImposter(imposters)
}

// SetAdapterBasedAnyValuePath sets a path to return a value as though it was from an adapter
func (k *Killgrave) SetAdapterBasedAnyValuePath(path string, methods []string, v interface{}) error {
	ar := KillgraveAdapterResponse{
		Id: "",
		Data: KillgraveAdapterResult{
			Result: v,
		},
		Error: nil,
	}
	data, err := json.Marshal(ar)
	if err != nil {
		return err
	}

	return k.SetStringValuePath(path, methods, map[string]string{
		"Content-Type": "application/json",
	}, string(data))
}

// SetAnyValueResponse sets a JSON-encoded response for a specified path and HTTP methods.
// It marshals the provided value into JSON format and updates the response headers accordingly.
// This function is useful for configuring dynamic API responses in a flexible manner.
func (k *Killgrave) SetAnyValueResponse(path string, methods []string, v interface{}) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}

	return k.SetStringValuePath(path, methods, map[string]string{
		"Content-Type": "application/json",
	}, string(data))
}

// SetAdapterBasedAnyValuePathObject sets a path to return a value as though it was from an adapter
func (k *Killgrave) SetAdapterBasedIntValuePath(path string, methods []string, v int) error {
	return k.SetAdapterBasedAnyValuePath(path, methods, v)
}

// Deprecated: Use Parrot instead
type RequestData struct {
	Method string              `json:"method"`
	Host   string              `json:"host"`
	URL    string              `json:"url"`
	Header map[string][]string `json:"header"`
	Body   string              `json:"body"`
}

// GetReceivedRequests retrieves and parses request data from a log file.
// It ensures all requests are written before reading and returns a slice of
// RequestData along with any encountered errors. This function is useful for
// accessing logged request information in a structured format.
func (k *Killgrave) GetReceivedRequests() ([]RequestData, error) {
	// killgrave uses a channel to write the request data to a file so we want to make sure
	// all requests have been written before reading the file
	time.Sleep(1 * time.Second)

	// Read the directory entries
	files, err := os.ReadDir(k.requestDumpDirBinding)
	if err != nil {
		return nil, err
	}

	// Iterate over the directory entries
	fmt.Println("Files Start")
	for _, file := range files {
		fmt.Println(file.Name())
	}
	fmt.Println("Files End")

	fileContent, err := os.ReadFile(filepath.Join(k.requestDumpDirBinding, "requestDump.log"))
	if err != nil {
		return nil, fmt.Errorf("error reading file: %w", err)
	}

	fmt.Println("File Content Start")
	fmt.Println(string(fileContent))
	fmt.Println("File Content End")

	// Split the contents by the newline separator
	requestDumps := strings.Split(string(fileContent), "\n")
	requestsData := []RequestData{}
	for _, requestDump := range requestDumps {
		if requestDump == "" {
			continue
		}

		rd := RequestData{}
		err := json.Unmarshal([]byte(requestDump), &rd)
		if err != nil {
			return nil, fmt.Errorf("error unmarshalling JSON: %w", err)
		}
		requestsData = append(requestsData, rd)
	}
	return requestsData, nil
}
