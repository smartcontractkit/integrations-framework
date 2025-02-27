package transformer

// Options defines configuration options for the test transformer
type Options struct {
	// IgnoreAllSubtestFailures determines if all subtest failures should be ignored
	IgnoreAllSubtestFailures bool
}

// DefaultOptions returns a new Options with default values
func DefaultOptions() *Options {
	return &Options{
		IgnoreAllSubtestFailures: false,
	}
}

// NewOptions creates a new Options with the specified parameters
func NewOptions(ignoreAll bool) *Options {
	return &Options{
		IgnoreAllSubtestFailures: ignoreAll,
	}
}
