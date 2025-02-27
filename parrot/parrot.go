package parrot

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/go-chi/chi"
	"github.com/go-resty/resty/v2"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/hlog"
	"gopkg.in/natefinch/lumberjack.v2"
)

const (
	HealthRoute   = "/health"
	RoutesRoute   = "/routes"
	RecorderRoute = "/recorder"

	// MethodAny is a wildcard for any HTTP method
	MethodAny = "ANY"

	logTimeFormat = "2006-01-02T15:04:05.000"
)

// These variables are set at build time and describe the Version of the application
var (
	version = "dev"
	commit  = "dev"
	date    = time.Now().Format(time.RFC3339)
	builtBy = "local"
)

func init() {
	zerolog.TimeFieldFormat = logTimeFormat
}

// Route holds information about the mock route configuration
type Route struct {
	// Method is the HTTP method to match
	Method string `json:"Method"`
	// Path is the URL path to match
	Path string `json:"Path"`
	// RawResponseBody is the static, raw string response to return when called
	RawResponseBody string `json:"raw_response_body"`
	// ResponseBody will be marshalled to JSON and returned when called
	ResponseBody any `json:"response_body"`
	// ResponseStatusCode is the HTTP status code to return when called
	ResponseStatusCode int `json:"response_status_code"`
}

// ID returns the unique identifier for the route
func (r *Route) ID() string {
	return r.Method + ":" + r.Path
}

// Server is a mock HTTP server that can register and respond to dynamic routes
type Server struct {
	port    int
	host    string
	address string

	router *chi.Mux
	server *http.Server
	client *resty.Client

	routes        map[string]*Route // Store routes for saving and retrieving
	routesMu      sync.RWMutex
	recorderHooks map[string]struct{} // Store recorders based on URL keys to avoid duplicates
	recordersMu   sync.RWMutex

	// Save and shutdown
	shutDown     atomic.Bool
	shutDownChan chan struct{}
	shutDownOnce sync.Once
	saveFileName string

	// Logging
	logFileName        string
	logFile            *lumberjack.Logger
	logLevel           zerolog.Level
	jsonLogs           bool
	disableConsoleLogs bool
	log                zerolog.Logger
}

// SaveFile is the structure of the file to save and load parrot data from
type SaveFile struct {
	Routes    []*Route `json:"routes"`
	Recorders []string `json:"recorders"`
}

// NewServer creates a new Parrot server with dynamic route handling
func NewServer(options ...ServerOption) (*Server, error) {
	p := &Server{
		port:         0,
		saveFileName: "parrot_save.json",
		logLevel:     zerolog.InfoLevel,
		logFileName:  "parrot.log",

		routes: make(map[string]*Route),
		router: chi.NewRouter(),
		client: resty.New(),

		shutDownChan: make(chan struct{}),

		recorderHooks: make(map[string]struct{}),
		recordersMu:   sync.RWMutex{},
	}
	p.router.Use(p.loggingMiddleware)

	for _, option := range options {
		if err := option(p); err != nil {
			return nil, err
		}
	}

	// Setup logger
	var writers []io.Writer

	if !p.disableConsoleLogs {
		if p.jsonLogs {
			writers = append(writers, os.Stderr)
		} else {
			consoleOut := zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: logTimeFormat}
			writers = append(writers, consoleOut)
		}
	}

	p.logFile = &lumberjack.Logger{
		Filename:   p.logFileName,
		MaxSize:    100, // megabytes
		MaxBackups: 10,
		MaxAge:     30,
		Compress:   true,
	}

	if p.logFileName != "" {
		writers = append(writers, p.logFile)
	}

	multiWriter := zerolog.MultiLevelWriter(writers...)
	p.log = zerolog.New(multiWriter).Level(p.logLevel).With().Timestamp().Logger()

	// Setup server
	listener, err := net.Listen("tcp", fmt.Sprintf("%s:%d", p.host, p.port))
	if err != nil {
		return nil, fmt.Errorf("failed to start listener: %w", err)
	}
	host, port, err := net.SplitHostPort(listener.Addr().String())
	if err != nil {
		return nil, fmt.Errorf("failed to split host and port: %w", err)
	}
	// Update host and port if they were not set before
	p.host = host
	p.address = listener.Addr().String()
	p.port, err = strconv.Atoi(port)
	if err != nil {
		return nil, fmt.Errorf("failed to parse port: %w", err)
	}

	// Initialize router
	p.router.Get("/", p.index())
	p.router.Get(HealthRoute, p.healthHandlerGET)

	p.router.Get(RoutesRoute, p.routesHandlerGET)
	p.router.Post(RoutesRoute, p.routesHandlerPOST)
	p.router.Delete(RoutesRoute, p.routesHandlerDELETE)

	p.router.Get(RecorderRoute, p.recorderHandlerGET)
	p.router.Post(RecorderRoute, p.recorderHandlerPOST)

	p.server = &http.Server{
		ReadHeaderTimeout: 5 * time.Second,
		Addr:              listener.Addr().String(),
		Handler:           p.router,
	}

	if err = p.load(); err != nil {
		return nil, fmt.Errorf("failed to load data from '%s': %w", p.saveFileName, err)
	}

	go p.run(listener)

	return p, nil
}

// index handles the root route
func (p *Server) index() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<html>Welcome to Parrot! See <a href="https://github.com/smartcontractkit/chainlink-testing-framework/tree/main/parrot">here</a> for docs</html>`))
	}
}

// run starts the parrot server
func (p *Server) run(listener net.Listener) {
	defer func() {
		p.shutDown.Store(true)
		if err := p.save(); err != nil {
			p.log.Error().Err(err).Msg("Failed to save routes")
		}
		if err := p.logFile.Close(); err != nil {
			fmt.Println("ERROR: Failed to close log file:", err)
		}
		p.shutDownOnce.Do(func() {
			close(p.shutDownChan)
		})
	}()

	p.log.Info().Str("Address", fmt.Sprintf("http://%s", p.address)).Msg("Parrot awake and ready to squawk")
	p.log.Debug().
		Int("Port", p.port).
		Str("Save File", p.saveFileName).
		Str("Log File", p.logFileName).
		Str("Version", version).
		Str("Commit", commit).
		Str("Build Date", date).
		Str("Built By", builtBy).
		Msg("Configuration")
	if err := p.server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
		fmt.Println("ERROR: Failed to start server:", err)
	}
}

// routeCallHandler handles incoming requests to the parrot server routes
func (p *Server) routeCallHandler(route *Route) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		routeCallLogger := zerolog.Ctx(r.Context())
		routeCallLogger.UpdateContext(func(c zerolog.Context) zerolog.Context {
			return c.Str("Route ID", route.ID())
		})

		if route.RawResponseBody != "" {
			w.Header().Set("Content-Type", "text/plain")
			w.WriteHeader(route.ResponseStatusCode)
			if _, err := w.Write([]byte(route.RawResponseBody)); err != nil {
				routeCallLogger.Error().Err(err).Msg("Failed to write response")
				http.Error(w, "Failed to write response", http.StatusInternalServerError)
			}
			return
		}

		if route.ResponseBody != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(route.ResponseStatusCode)
			if err := json.NewEncoder(w).Encode(route.ResponseBody); err != nil {
				routeCallLogger.Error().Err(err).Msg("Failed to write response")
				http.Error(w, "Failed to write response", http.StatusInternalServerError)
			}
			return
		}

		routeCallLogger.Error().Msg("No response provided")
		http.Error(w, "No response provided", http.StatusInternalServerError)
	}
}

// Healthy checks if the parrot server is healthy
func (p *Server) Healthy() error {
	if p.shutDown.Load() {
		return ErrServerShutdown
	}

	healthCheckRoute := &Route{
		Method:             http.MethodGet,
		Path:               "/check/health",
		RawResponseBody:    "Healthy",
		ResponseStatusCode: http.StatusOK,
	}

	p.log.Info().Msg("Checking Parrot health")
	err := p.Register(healthCheckRoute)
	if err != nil {
		return newDynamicError(ErrServerUnhealthy, fmt.Sprintf("%s: unable to register routes", err.Error()))
	}

	resp, err := p.Call(http.MethodGet, healthCheckRoute.Path)
	if err != nil {
		return newDynamicError(ErrServerUnhealthy, fmt.Sprintf("%s: unable to call routes", err.Error()))
	}

	if resp.StatusCode() != http.StatusOK {
		return newDynamicError(ErrServerUnhealthy, fmt.Sprintf("routes not responding with expected code, expected %d, got %d", http.StatusOK, resp.StatusCode()))
	}

	p.Delete(healthCheckRoute)

	p.log.Info().Msg("Parrot healthy")
	return nil
}

// healthHandlerGET handles the health check route
// GET /health
func (p *Server) healthHandlerGET(w http.ResponseWriter, r *http.Request) {
	healthLogger := zerolog.Ctx(r.Context())

	err := p.Healthy()
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		healthLogger.Error().Err(err).Msg("Parrot is unhealthy")
		return
	}
	w.WriteHeader(http.StatusOK)
}

// Shutdown gracefully shuts down the parrot server
func (p *Server) Shutdown(ctx context.Context) error {
	if p.shutDown.Load() {
		return ErrServerShutdown
	}

	p.log.Info().Msg("Putting cloth over the parrot's cage...")
	return p.server.Shutdown(ctx)
}

// WaitShutdown blocks until the parrot server has shut down
func (p *Server) WaitShutdown() {
	<-p.shutDownChan
}

// Address returns the address the parrot is running on
func (p *Server) Address() string {
	return p.address
}

// Port returns the port the parrot is running on
func (p *Server) Port() int {
	return p.port
}

// Host returns the host the parrot is running on
func (p *Server) Host() string {
	return p.host
}

// Register adds a new route to the parrot
func (p *Server) Register(route *Route) error {
	if p.shutDown.Load() {
		return ErrServerShutdown
	}
	if route == nil {
		return ErrNilRoute
	}
	if !strings.HasPrefix(route.Path, "/") {
		route.Path = "/" + route.Path
	}
	if !isValidPath(route.Path) {
		return newDynamicError(ErrInvalidPath, fmt.Sprintf("'%s'", route.Path))
	}
	if !isValidMethod(route.Method) {
		return newDynamicError(ErrInvalidMethod, fmt.Sprintf("'%s'", route.Method))
	}
	if route.ResponseBody == nil && route.RawResponseBody == "" {
		return ErrNoResponse
	}
	if route.ResponseBody != nil && route.RawResponseBody != "" {
		return ErrOnlyOneResponse
	}
	if route.ResponseBody != nil {
		if _, err := json.Marshal(route.ResponseBody); err != nil {
			return newDynamicError(ErrResponseMarshal, err.Error())
		}
	}
	numWildcards := strings.Count(route.Path, "*")
	if numWildcards > 1 {
		return newDynamicError(ErrWildcardPath, fmt.Sprintf("more than 1 wildcard '%s'", route.Path))
	}
	if numWildcards == 1 && !strings.HasSuffix(route.Path, "*") {
		return newDynamicError(ErrWildcardPath, fmt.Sprintf("wildcard not at end '%s'", route.Path))
	}

	if route.Method == MethodAny {
		p.router.Handle(route.Path, routeRecordingMiddleware(p, p.routeCallHandler(route)))
	} else {
		p.router.MethodFunc(route.Method, route.Path, routeRecordingMiddleware(p, p.routeCallHandler(route)))
	}

	p.routesMu.Lock()
	defer p.routesMu.Unlock()
	p.routes[route.ID()] = route
	p.log.Info().
		Str("Route ID", route.ID()).
		Msg("Registered route")

	return nil
}

// routesHandlerPOST handles registering a new route
// POST /routes
func (p *Server) routesHandlerPOST(w http.ResponseWriter, r *http.Request) {
	routesLogger := zerolog.Ctx(r.Context())

	var route *Route
	if err := json.NewDecoder(r.Body).Decode(&route); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		routesLogger.Debug().Err(err).Msg("Failed to decode request body")
		return
	}
	defer r.Body.Close()

	err := p.Register(route)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		routesLogger.Debug().Err(err).Msg("Failed to register route")
		return
	}

	w.WriteHeader(http.StatusCreated)
}

// Record registers a new recorder with the parrot. All incoming requests to the parrot will be sent to the recorder.
func (p *Server) Record(recorderURL string) error {
	if p.shutDown.Load() {
		return ErrServerShutdown
	}

	p.recordersMu.Lock()
	defer p.recordersMu.Unlock()
	if recorderURL == "" {
		return ErrNoRecorderURL
	}
	_, err := url.ParseRequestURI(recorderURL)
	if err != nil {
		return ErrInvalidRecorderURL
	}
	p.recorderHooks[recorderURL] = struct{}{}
	p.log.Info().Str("URL", recorderURL).Msg("Registered Recorder")
	return nil
}

// recorderHandlerPOST handles registering a new recorder
// POST /recorder
func (p *Server) recorderHandlerPOST(w http.ResponseWriter, r *http.Request) {
	recordingLogger := zerolog.Ctx(r.Context())

	var recorder *Recorder
	if err := json.NewDecoder(r.Body).Decode(&recorder); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		recordingLogger.Error().Err(err).Msg("Failed to decode request body")
		return
	}
	defer r.Body.Close()

	if recorder == nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		recordingLogger.Debug().Msg("No recorder provided")
		return
	}

	if recorder.URL() == "" {
		http.Error(w, "Recorder URL required", http.StatusBadRequest)
		recordingLogger.Debug().Msg("No recorder URL provided")
		return
	}

	if err := p.Record(recorder.URL()); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		recordingLogger.Debug().Err(err).Msg("Failed to register recorder")
		return
	}

	w.WriteHeader(http.StatusCreated)
}

// Recorders returns the URLs of all registered recorders
func (p *Server) Recorders() []string {
	if p.shutDown.Load() {
		return nil
	}

	p.recordersMu.RLock()
	defer p.recordersMu.RUnlock()
	recorders := make([]string, 0, len(p.recorderHooks))
	for recorder := range p.recorderHooks {
		recorders = append(recorders, recorder)
	}
	p.log.Debug().Int("Count", len(recorders)).Msg("Got recorders")
	return recorders
}

// recorderHandlerGET handles getting all recorders
// GET /recorder
func (p *Server) recorderHandlerGET(w http.ResponseWriter, r *http.Request) {
	recordersLogger := zerolog.Ctx(r.Context())

	recorders := p.Recorders()
	jsonRecorders, err := json.Marshal(recorders)
	if err != nil {
		http.Error(w, "Failed to marshal recorders", http.StatusInternalServerError)
		recordersLogger.Error().Err(err).Msg("Failed to marshal recorders")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if _, err = w.Write(jsonRecorders); err != nil {
		http.Error(w, "Failed to write response", http.StatusInternalServerError)
		recordersLogger.Error().Err(err).Msg("Failed to write response")
		return
	}

	w.WriteHeader(http.StatusOK)
}

// Delete removes a route from the parrot
func (p *Server) Delete(route *Route) {
	p.router.Method(route.Method, route.Path, http.NotFoundHandler())
	p.routesMu.Lock()
	defer p.routesMu.Unlock()
	delete(p.routes, route.ID())
	p.log.Info().
		Str("Route ID", route.ID()).
		Msg("Route deleted")
}

// routesHandlerDELETE handles deleting a route
// DELETE /routes
func (p *Server) routesHandlerDELETE(w http.ResponseWriter, r *http.Request) {
	routesLogger := zerolog.Ctx(r.Context())

	var route *Route
	if err := json.NewDecoder(r.Body).Decode(&route); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		routesLogger.Debug().Err(err).Msg("Failed to decode request body")
		return
	}
	defer r.Body.Close()

	p.Delete(route)
	w.WriteHeader(http.StatusNoContent)
}

// Call makes a request to the parrot server
func (p *Server) Call(method, path string) (*resty.Response, error) {
	if p.shutDown.Load() {
		return nil, ErrServerShutdown
	}
	if !isValidMethod(method) {
		return nil, newDynamicError(ErrInvalidMethod, fmt.Sprintf("'%s'", method))
	}
	return p.client.R().Execute(method, "http://"+filepath.Join(p.Address(), path))
}

func (p *Server) Routes() []*Route {
	if p.shutDown.Load() {
		return nil
	}

	p.routesMu.RLock()
	defer p.routesMu.RUnlock()
	routes := make([]*Route, 0, len(p.routes))
	for _, route := range p.routes {
		routes = append(routes, route)
	}
	p.log.Debug().Int("Count", len(routes)).Msg("Returned routes")
	return routes
}

// routesHandlerGET handles getting all routes
// GET /routes
func (p *Server) routesHandlerGET(w http.ResponseWriter, r *http.Request) {
	routesLogger := zerolog.Ctx(r.Context())

	routes := p.Routes()
	jsonRoutes, err := json.Marshal(routes)
	if err != nil {
		http.Error(w, "Failed to marshal routes", http.StatusInternalServerError)
		routesLogger.Error().Err(err).Msg("Failed to marshal routes")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if _, err = w.Write(jsonRoutes); err != nil {
		http.Error(w, "Failed to write response", http.StatusInternalServerError)
		routesLogger.Error().Err(err).Msg("Failed to write response")
		return
	}
}

// load loads all registered routes from a file.
func (p *Server) load() error {
	if _, err := os.Stat(p.saveFileName); os.IsNotExist(err) {
		p.log.Trace().Str("file", p.saveFileName).Msg("No data to load")
		return nil
	}

	p.log.Debug().Str("File", p.saveFileName).Msg("Loading data")

	fileData, err := os.ReadFile(p.saveFileName)
	if err != nil {
		return fmt.Errorf("failed to read routes from file: %w", err)
	}
	if len(fileData) == 0 {
		p.log.Trace().Str("File", p.saveFileName).Msg("No data to load")
		return nil
	}

	var saveData SaveFile
	err = json.Unmarshal(fileData, &saveData)
	if err != nil {
		return fmt.Errorf("failed to unmarshal save file: %w", err)
	}

	for _, route := range saveData.Routes {
		if err = p.Register(route); err != nil {
			return fmt.Errorf("failed to register route: %w", err)
		}
	}

	for _, recorder := range saveData.Recorders {
		if err = p.Record(recorder); err != nil {
			return fmt.Errorf("failed to register recorder: %w", err)
		}
	}

	p.log.Info().Str("file", p.saveFileName).Int("number", len(p.routes)).Msg("Loaded routes")
	return nil
}

// save saves all registered routes to a file.
func (p *Server) save() error {
	saveFile := &SaveFile{
		Routes:    p.Routes(),
		Recorders: p.Recorders(),
	}
	if len(saveFile.Routes) == 0 && len(saveFile.Recorders) == 0 {
		p.log.Trace().Str("File", p.saveFileName).Msg("No data to save")
		return nil
	}

	jsonData, err := json.Marshal(saveFile)
	if err != nil {
		return fmt.Errorf("failed to marshal save file: %w", err)
	}

	if err = os.WriteFile(p.saveFileName, jsonData, 0644); err != nil { //nolint:gosec
		return fmt.Errorf("failed to write to save file: %w", err)
	}

	p.log.Debug().Str("File", p.saveFileName).Msg("Saved data")
	return nil
}

// sendToRecorders sends the route call to all registered recorders
func (p *Server) sendToRecorders(routeCall *RouteCall) {
	p.recordersMu.RLock()
	defer p.recordersMu.RUnlock()
	if len(p.recorderHooks) == 0 {
		return
	}

	client := resty.New()
	p.log.Trace().
		Int("Recorder Count", len(p.recorderHooks)).
		Str("Route Call ID", routeCall.ID).
		Str("Route ID", routeCall.RouteID).
		Msg("Sending route call to recorders")

	for hook := range p.recorderHooks {
		go func(hook string) {
			resp, err := client.R().SetBody(routeCall).Post(hook)
			if err != nil {
				p.log.Error().Err(err).Str("Recorder Hook", hook).Msg("Failed to send route call to recorder")
				return
			}
			if resp.IsError() {
				p.log.Error().
					Str("Recorder Hook", hook).
					Int("Code", resp.StatusCode()).
					Str("Response", resp.String()).
					Msg("Failed to send route call to recorder")
				return
			}
			p.log.Trace().
				Str("Route Call ID", routeCall.ID).
				Str("Route ID", routeCall.RouteID).
				Str("Recorder Hook", hook).
				Msg("Route call sent to recorder")
		}(hook)
	}
}

// loggingMiddleware logs all incoming requests
func (p *Server) loggingMiddleware(next http.Handler) http.Handler {
	h := hlog.NewHandler(p.log)

	accessHandler := hlog.AccessHandler(
		func(r *http.Request, status, size int, duration time.Duration) {
			hlog.FromRequest(r).Trace().
				Str("Method", r.Method).
				Stringer("URL", r.URL).
				Int("Status Code", status).
				Int("Response Size Bytes", size).
				Str("Duration", duration.String()).
				Str("Remote Addr", r.RemoteAddr).
				Msg("Handled request")
		},
	)

	return h(accessHandler(next))
}

var pathRegex = regexp.MustCompile(`^\/[a-zA-Z0-9\-._~%!$&'()*+,;=:@\/]*$`)

// isValidPath checks if the path is a valid URL path
func isValidPath(path string) bool {
	if path == "" || path == "/" {
		return false
	}
	if strings.Contains(path, "//") {
		return false
	}
	if !strings.HasPrefix(path, "/") {
		return false
	}
	if strings.HasPrefix(path, RecorderRoute) {
		return false
	}
	if strings.HasPrefix(path, HealthRoute) {
		return false
	}
	if strings.HasPrefix(path, RoutesRoute) {
		return false
	}
	return pathRegex.MatchString(path)
}

// isValidMethod checks if the method is a valid HTTP method, in loose terms
func isValidMethod(method string) bool {
	switch method {
	case
		http.MethodGet,
		http.MethodPost,
		http.MethodPut,
		http.MethodPatch,
		http.MethodDelete,
		http.MethodOptions,
		http.MethodConnect,
		http.MethodTrace,
		MethodAny:
		return true
	}
	return false
}
