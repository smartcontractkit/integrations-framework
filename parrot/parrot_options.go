package parrot

import (
	"fmt"

	"github.com/rs/zerolog"
)

// ServerOption defines functional options for configuring the ParrotServer
type ServerOption func(*Server) error

// WithHost sets the address for the ParrotServer to run on
func WithHost(host string) ServerOption {
	return func(s *Server) error {
		s.host = host
		return nil
	}
}

// WithPort sets the port for the ParrotServer to run on
func WithPort(port int) ServerOption {
	return func(s *Server) error {
		if port < 0 || port > 65535 {
			return fmt.Errorf("invalid port: %d", port)
		}
		s.port = port
		return nil
	}
}

// WithLogLevel sets the visible log level of the default logger
func WithLogLevel(level zerolog.Level) ServerOption {
	return func(s *Server) error {
		s.logLevel = level
		return nil
	}
}

// WithJSONLogs sets the logger to output JSON logs
func WithJSONLogs() ServerOption {
	return func(s *Server) error {
		s.jsonLogs = true
		return nil
	}
}

// DisableConsoleLogs disables logging to the console
func DisableConsoleLogs() ServerOption {
	return func(s *Server) error {
		s.disableConsoleLogs = true
		return nil
	}
}

// WithSaveFile sets the file to save the routes to
func WithSaveFile(saveFile string) ServerOption {
	return func(s *Server) error {
		if saveFile == "" {
			return fmt.Errorf("invalid save file name: %s", saveFile)
		}
		s.saveFileName = saveFile
		return nil
	}
}

// WithLogFile sets the file to save the logs to
func WithLogFile(logFile string) ServerOption {
	return func(s *Server) error {
		if logFile == "" {
			return fmt.Errorf("invalid log file name: %s", logFile)
		}
		s.logFileName = logFile
		return nil
	}
}

// WithRoutes sets the initial routes for the Parrot
func WithRoutes(routes []*Route) ServerOption {
	return func(s *Server) error {
		for _, route := range routes {
			if err := s.Register(route); err != nil {
				return fmt.Errorf("failed to register route: %w", err)
			}
		}
		return nil
	}
}

// WithRecorders sets the initial recorders for the Parrot
func WithRecorders(recorders ...string) ServerOption {
	return func(s *Server) error {
		for _, recorder := range recorders {
			if err := s.Record(recorder); err != nil {
				return fmt.Errorf("failed to register recorder: %w", err)
			}
		}
		return nil
	}
}
