package parrot

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"
)

// Recorder records route calls
type Recorder struct {
	Host   string `json:"host"`
	Port   string `json:"port"`
	server *http.Server

	recordChan chan *RouteCall
	errChan    chan error
}

// RouteCall records when a route is called, the request and response
type RouteCall struct {
	// ID is a unique identifier for the route call for help with debugging
	ID string `json:"id"`
	// RouteID is the identifier of the route that was called
	RouteID string `json:"route_id"`
	// Request is the request made to the route
	Request *RouteCallRequest `json:"request"`
	// Response is the response from the route
	Response *RouteCallResponse `json:"response"`
}

// RouteCallRequest records the request made to a route
type RouteCallRequest struct {
	Method     string      `json:"method"`
	URL        *url.URL    `json:"url"`
	RemoteAddr string      `json:"caller"`
	Header     http.Header `json:"header"`
	Body       []byte      `json:"body"`
}

// RouteCallResponse records the response from a route
type RouteCallResponse struct {
	StatusCode int         `json:"status_code"`
	Header     http.Header `json:"header"`
	Body       []byte      `json:"body"`
}

// RecorderOption is a function that modifies a recorder
type RecorderOption func(*Recorder)

// WithRecorderHost sets the host of the recorder
func WithRecorderHost(host string) RecorderOption {
	return func(r *Recorder) {
		r.Host = host
	}
}

// NewRecorder creates a new recorder that listens for incoming requests to the parrot server
func NewRecorder(opts ...RecorderOption) (*Recorder, error) {
	r := &Recorder{
		recordChan: make(chan *RouteCall),
		errChan:    make(chan error),
	}

	listener, err := net.Listen("tcp", ":0") // nolint:gosec
	if err != nil {
		return nil, fmt.Errorf("failed to start listener: %w", err)
	}
	r.Host, r.Port, err = net.SplitHostPort(listener.Addr().String())
	if err != nil {
		return nil, fmt.Errorf("failed to split host and port: %w", err)
	}

	mux := http.NewServeMux()
	mux.Handle("/", r.defaultRecordHandler())
	r.server = &http.Server{
		ReadHeaderTimeout: 5 * time.Second,
		Addr:              listener.Addr().String(),
		Handler:           mux,
	}

	for _, opt := range opts {
		opt(r)
	}

	go func() {
		if err := r.server.Serve(listener); err != nil {
			if err != http.ErrServerClosed {
				fmt.Println("Error serving recorder:", err)
			}
		}
	}()
	return r, nil
}

// URL returns the URL of the recorder to send requests to
// WARNING: This URL automatically binds to the first available port on the host machine
// and the host will be 0.0.0.0 or localhost. If you're calling this from a different machine
// you will need to replace the host with the IP address of the machine running the recorder.
func (r *Recorder) URL() string {
	return fmt.Sprintf("http://%s:%s", r.Host, r.Port)
}

// Record receives recorded calls
func (r *Recorder) Record() chan *RouteCall {
	return r.recordChan
}

// Close shuts down the recorder
func (r *Recorder) Close() error {
	return r.server.Close()
}

// Err receives errors from the recorder
func (r *Recorder) Err() chan error {
	return r.errChan
}

func (r *Recorder) defaultRecordHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		var recordedCall *RouteCall
		if err := json.NewDecoder(req.Body).Decode(&recordedCall); err != nil {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}
		defer req.Body.Close()

		r.recordChan <- recordedCall
	}
}

// httpResponseRecorder is a wrapper around http.ResponseWriter that records the response
// for later inspection while still writing to the original writer.
// WARNING: If you mutate after calling Header(), the changes will not be reflected in the recorded response.
type responseWriterRecorder struct {
	originalWriter http.ResponseWriter
	record         *httptest.ResponseRecorder
}

func newResponseWriterRecorder(w http.ResponseWriter) *responseWriterRecorder {
	return &responseWriterRecorder{
		originalWriter: w,
		record:         httptest.NewRecorder(),
	}
}

// SetWriter sets a new writer to record and write to, flushing any previous record
func (rr *responseWriterRecorder) SetWriter(w http.ResponseWriter) {
	rr.originalWriter = w
	rr.record = httptest.NewRecorder()
}

func (rr *responseWriterRecorder) WriteHeader(code int) {
	rr.originalWriter.WriteHeader(code)
	rr.record.WriteHeader(code)
}

func (rr *responseWriterRecorder) Write(data []byte) (int, error) {
	_, _ = rr.record.Write(data) // ignore error as we still want to write to the original writer
	return rr.originalWriter.Write(data)
}

func (rr *responseWriterRecorder) Header() http.Header {
	return rr.record.Header()
}

func (rr *responseWriterRecorder) Result() *http.Response {
	resp := rr.record.Result()
	resp.Header = rr.originalWriter.Header() // Ensure headers are properly synced
	return resp
}

// routeRecordingMiddleware is a middleware that records the request and response of a route call
func routeRecordingMiddleware(p *Server, next http.Handler) http.HandlerFunc {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		routeCallID := uuid.New().String()[0:8]
		recordLogger := zerolog.Ctx(r.Context())
		recordLogger.UpdateContext(func(c zerolog.Context) zerolog.Context {
			return c.Str("Route Call ID", routeCallID)
		})

		reqBody, err := io.ReadAll(r.Body) // Read the body to ensure it's not closed before we can read it
		if err != nil {
			recordLogger.Error().Err(err).Msg("Failed to read request body")
			http.Error(w, "Failed to read request body", http.StatusInternalServerError)
			return
		}
		r.Body = io.NopCloser(bytes.NewBuffer(reqBody))

		routeCall := &RouteCall{
			ID:      routeCallID,
			RouteID: r.Method + ":" + r.URL.String(),
			Request: &RouteCallRequest{
				Method:     r.Method,
				URL:        r.URL,
				RemoteAddr: r.RemoteAddr,
				Header:     r.Header,
				Body:       reqBody,
			},
		}

		rr := newResponseWriterRecorder(w)
		next.ServeHTTP(rr, r)

		resp := rr.Result()
		routeCall.Response = &RouteCallResponse{
			StatusCode: resp.StatusCode,
			Header:     resp.Header,
			Body:       rr.record.Body.Bytes(),
		}

		p.sendToRecorders(routeCall)
	})
}
