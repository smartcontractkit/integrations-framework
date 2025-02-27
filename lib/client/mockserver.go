package client

import (
	"crypto/tls"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/go-resty/resty/v2"
	"github.com/rs/zerolog/log"

	"github.com/smartcontractkit/chainlink-testing-framework/lib/k8s/environment"
	"github.com/smartcontractkit/chainlink-testing-framework/lib/k8s/pkg/helm/mockserver"
)

// MockserverClient mockserver client
//
// Deprecated: Use Parrot instead
type MockserverClient struct {
	APIClient *resty.Client
	Config    *MockserverConfig
}

// MockserverConfig holds config information for MockserverClient
//
// Deprecated: Use Parrot instead
type MockserverConfig struct {
	LocalURL   string
	ClusterURL string
	Headers    map[string]string
}

// ConnectMockServer creates a connection to a deployed mockserver in the environment
//
// Deprecated: Use Parrot instead
func ConnectMockServer(e *environment.Environment) *MockserverClient {
	c := NewMockserverClient(&MockserverConfig{
		LocalURL:   e.URLs[mockserver.LocalURLsKey][0],
		ClusterURL: e.URLs[mockserver.InternalURLsKey][0],
	})
	return c
}

// ConnectMockServerURL creates a connection to a mockserver at a given url, should only be used for inside K8s tests
//
// Deprecated: Use Parrot instead
func ConnectMockServerURL(url string) *MockserverClient {
	c := NewMockserverClient(&MockserverConfig{
		LocalURL:   url,
		ClusterURL: url,
	})
	return c
}

// NewMockserverClient returns a mockserver client
//
// Deprecated: Use Parrot instead
func NewMockserverClient(cfg *MockserverConfig) *MockserverClient {
	log.Debug().Str("Local URL", cfg.LocalURL).Str("Remote URL", cfg.ClusterURL).Msg("Connected to MockServer")
	isDebug := os.Getenv("RESTY_DEBUG") == "true"
	return &MockserverClient{
		Config: cfg,
		APIClient: resty.New().
			SetBaseURL(cfg.LocalURL).
			SetHeaders(cfg.Headers).
			SetDebug(isDebug).
			//nolint
			SetTLSClientConfig(&tls.Config{InsecureSkipVerify: true}),
	}
}

// PutExpectations sets the expectations (i.e. mocked responses)
func (em *MockserverClient) PutExpectations(body interface{}) error {
	resp, err := em.APIClient.R().SetBody(body).Put("/expectation")
	if resp.StatusCode() != http.StatusCreated {
		err = fmt.Errorf("Unexpected Status Code. Expected %d; Got %d", http.StatusCreated, resp.StatusCode())
	}
	return err
}

// ClearExpectation clears expectations
func (em *MockserverClient) ClearExpectation(body interface{}) error {
	resp, err := em.APIClient.R().SetBody(body).Put("/clear")
	if resp.StatusCode() != http.StatusOK {
		err = fmt.Errorf("Unexpected Status Code. Expected %d; Got %d", http.StatusOK, resp.StatusCode())
	}
	return err
}

// SetRandomValuePath sets a random int value for a path
func (em *MockserverClient) SetRandomValuePath(path string) error {
	sanitizedPath := strings.ReplaceAll(path, "/", "_")
	log.Debug().Str("ID", fmt.Sprintf("%s_mock_id", sanitizedPath)).
		Str("Path", path).
		Msg("Setting Random Value Mock Server Path")
	initializer := HttpInitializerTemplate{
		Id:      fmt.Sprintf("%s_mock_id", sanitizedPath),
		Request: HttpRequest{Path: path},
		Response: HttpResponseTemplate{
			Template:     "return { statusCode: 200, body: JSON.stringify({id: '', error: null, data: { result: Math.floor(Math.random() * (1000 - 900) + 900) } }) }",
			TemplateType: "JAVASCRIPT",
		},
	}
	initializers := []HttpInitializerTemplate{initializer}
	resp, err := em.APIClient.R().SetBody(&initializers).Put("/expectation")
	if resp.StatusCode() != http.StatusCreated {
		err = fmt.Errorf("status code expected %d got %d", http.StatusCreated, resp.StatusCode())
	}
	return err
}

// SetValuePath sets an int for a path
func (em *MockserverClient) SetValuePath(path string, v int) error {
	if !strings.HasPrefix(path, "/") {
		path = fmt.Sprintf("/%s", path)
	}
	sanitizedPath := strings.ReplaceAll(path, "/", "_")
	log.Debug().Str("ID", fmt.Sprintf("%s_mock_id", sanitizedPath)).
		Str("URL", em.APIClient.BaseURL).
		Str("Path", path).
		Int("Value", v).
		Msg("Setting Mock Server Path")
	initializer := HttpInitializer{
		Id:      fmt.Sprintf("%s_mock_id", sanitizedPath),
		Request: HttpRequest{Path: path},
		Response: HttpResponse{Body: AdapterResponse{
			Id:    "",
			Data:  AdapterResult{Result: v},
			Error: nil,
		}},
	}
	initializers := []HttpInitializer{initializer}
	resp, err := em.APIClient.R().SetBody(&initializers).Put("/expectation")
	if resp.StatusCode() != http.StatusCreated {
		err = fmt.Errorf("status code expected %d got %d, err: %s", http.StatusCreated, resp.StatusCode(), err)
	}
	return err
}

// SetAnyValuePath sets any type of value for a path
func (em *MockserverClient) SetAnyValuePath(path string, v interface{}) error {
	if !strings.HasPrefix(path, "/") {
		path = fmt.Sprintf("/%s", path)
	}
	sanitizedPath := strings.ReplaceAll(path, "/", "_")
	id := fmt.Sprintf("%s_mock_id", sanitizedPath)
	log.Debug().Str("ID", id).
		Str("Path", path).
		Interface("Value", v).
		Msg("Setting Mock Server Path")
	initializer := HttpInitializer{
		Id:      id,
		Request: HttpRequest{Path: path},
		Response: HttpResponse{
			Body: AdapterResponse{
				Id: "",
				Data: AdapterResult{
					Result: v,
				},
				Error: nil,
			},
		},
	}
	initializers := []HttpInitializer{initializer}
	resp, err := em.APIClient.R().SetBody(&initializers).Put("/expectation")
	if resp.StatusCode() != http.StatusCreated {
		err = fmt.Errorf("status code expected %d got %d", http.StatusCreated, resp.StatusCode())
	}
	return err
}

// SetAnyValueResponse configures a mock server to return a specified value for a given path.
// It ensures the path starts with a '/', sanitizes it, and logs the operation.
// This function is useful for testing and simulating API responses in a controlled environment.
func (em *MockserverClient) SetAnyValueResponse(path string, v interface{}) error {
	if !strings.HasPrefix(path, "/") {
		path = fmt.Sprintf("/%s", path)
	}
	sanitizedPath := strings.ReplaceAll(path, "/", "_")
	id := fmt.Sprintf("%s_mock_id", sanitizedPath)
	log.Debug().Str("ID", id).
		Str("Path", path).
		Interface("Value", v).
		Msg("Setting Mock Server Path")
	initializer := HttpInitializer{
		Id:      id,
		Request: HttpRequest{Path: path},
		Response: HttpResponse{
			Body: v,
		},
	}
	initializers := []HttpInitializer{initializer}
	resp, err := em.APIClient.R().SetBody(&initializers).Put("/expectation")
	if resp.StatusCode() != http.StatusCreated {
		err = fmt.Errorf("status code expected %d got %d", http.StatusCreated, resp.StatusCode())
	}
	return err
}

// SetStringValuePath sets a string value for a path and returns it as a raw string
func (em *MockserverClient) SetStringValuePath(path string, stringValue string) error {
	sanitizedPath := strings.ReplaceAll(path, "/", "_")
	id := fmt.Sprintf("%s_mock_id", sanitizedPath)
	log.Debug().Str("ID", id).
		Str("Path", path).
		Msg("Setting Mock Server String Path")

	initializer := HttpInitializer{
		Id:      id,
		Request: HttpRequest{Path: path},
		Response: HttpResponse{
			Body: stringValue,
		},
	}

	initializers := []HttpInitializer{initializer}
	resp, err := em.APIClient.R().SetBody(&initializers).Put("/expectation")
	if resp.StatusCode() != http.StatusCreated {
		err = fmt.Errorf("status code expected %d got %d", http.StatusCreated, resp.StatusCode())
	}
	return err
}

// LocalURL returns the local url of the mockserver
//
// Deprecated: Use Parrot instead
func (em *MockserverClient) LocalURL() string {
	return em.Config.LocalURL
}

// PathSelector represents the json object used to find expectations by path
//
// Deprecated: Use Parrot instead
type PathSelector struct {
	Path string `json:"path"`
}

// HttpRequest represents the httpRequest json object used in the mockserver initializer
//
// Deprecated: Use Parrot instead
type HttpRequest struct {
	Path string `json:"path"`
}

// HttpResponse represents the httpResponse json object used in the mockserver initializer
//
// Deprecated: Use Parrot instead
type HttpResponse struct {
	Body interface{} `json:"body"`
}

// HttpInitializer represents an element of the initializer array used in the mockserver initializer
//
// Deprecated: Use Parrot instead
type HttpInitializer struct {
	Id       string       `json:"id"`
	Request  HttpRequest  `json:"httpRequest"`
	Response HttpResponse `json:"httpResponse"`
}

// HttpResponse represents the httpResponse json object used in the mockserver initializer
//
// Deprecated: Use Parrot instead
type HttpResponseTemplate struct {
	Template     string `json:"template"`
	TemplateType string `json:"templateType"`
}

// HttpInitializer represents an element of the initializer array used in the mockserver initializer
//
// Deprecated: Use Parrot instead
type HttpInitializerTemplate struct {
	Id       string               `json:"id"`
	Request  HttpRequest          `json:"httpRequest"`
	Response HttpResponseTemplate `json:"httpResponseTemplate"`
}

// For OTPE - weiwatchers

// NodeInfoJSON represents an element of the nodes array used to deliver configs to otpe
//
// Deprecated: Use Parrot instead
type NodeInfoJSON struct {
	ID          string   `json:"id"`
	NodeAddress []string `json:"nodeAddress"`
}

// ContractInfoJSON represents an element of the contracts array used to deliver configs to otpe
//
// Deprecated: Use Parrot instead
type ContractInfoJSON struct {
	ContractAddress string `json:"contractAddress"`
	ContractVersion int    `json:"contractVersion"`
	Path            string `json:"path"`
	Status          string `json:"status"`
}

// For Adapter endpoints

// AdapterResult represents an int result for an adapter
//
// Deprecated: Use Parrot instead
type AdapterResult struct {
	Result interface{} `json:"result"`
}

// AdapterResponse represents a response from an adapter
//
// Deprecated: Use Parrot instead
type AdapterResponse struct {
	Id    string        `json:"id"`
	Data  AdapterResult `json:"data"`
	Error interface{}   `json:"error"`
}
