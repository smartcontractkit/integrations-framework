package framework

import (
	"path/filepath"
	"testing"
)

// Table test for checkLogFilesForLevels
func TestCheckLogFilesForLevels(t *testing.T) {
	tests := []struct {
		name        string
		dir         string
		content     string
		ignoreFlag  bool
		expectError bool
	}{
		{
			name:        "Clean",
			dir:         "clean",
			expectError: false,
		},
		{
			name:        "Ignore all",
			dir:         "crit",
			ignoreFlag:  true,
			expectError: false,
		},
		{
			name:        "Contains CRIT",
			dir:         "crit",
			expectError: true,
		},
		{
			name:        "Contains PANIC",
			dir:         "panic",
			expectError: true,
		},
		{
			name:        "Contains FATAL",
			dir:         "fatal",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.ignoreFlag {
				t.Setenv("CTF_IGNORE_CRITICAL_LOGS", "true")
			}
			err := checkNodeLogErrors(filepath.Join("testdata", tt.dir))
			if tt.expectError && err == nil {
				t.Errorf("expected error but got none")
			} else if !tt.expectError && err != nil {
				t.Errorf("did not expect error but got: %v", err)
			}
		})
	}
}
