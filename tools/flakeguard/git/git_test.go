package git

import (
	"bytes"
	"testing"

	"github.com/go-git/go-billy/v5/memfs"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/storage/memory"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInferGitData(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name             string
		projectPath      string
		userProvidedData *Data
		expected         *Data
		expectError      bool
	}{
		{
			name:        "Basic Case",
			projectPath: "/path/to/project",
			expected: &Data{
				RepoPath:      "/path/to/project",
				CurrentBranch: "main",
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testFiles := memfs.New()
			gitStorage := memory.NewStorage()
			repo, err := git.Init(gitStorage, testFiles)
			require.NoError(t, err)
			repo.CreateBranch(&config.Branch{
				Name: "main",
			})
		})
	}
}

func TestGetChangedGoPackagesFromDiff(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		out         string
		projectPath string
		excludes    []string
		fileMap     map[string][]string
		expected    []string
		expectError bool
	}{
		{
			name:     "Basic Case",
			out:      "pkg1/file1.go\npkg2/file2.go\n",
			excludes: []string{},
			fileMap: map[string][]string{
				"pkg1/file1.go": {"pkg1"},
				"pkg2/file2.go": {"pkg2"},
			},
			expected:    []string{"pkg1", "pkg2"},
			expectError: false,
		},
		{
			name:        "Empty Input",
			out:         "",
			excludes:    []string{},
			fileMap:     map[string][]string{},
			expected:    []string{},
			expectError: false,
		},
		{
			name:     "Non-Go Files Ignored",
			out:      "pkg1/file1.txt\npkg2/file2.go\n",
			excludes: []string{},
			fileMap: map[string][]string{
				"pkg2/file2.go": {"pkg2"},
			},
			expected:    []string{"pkg2"},
			expectError: false,
		},
		{
			name:     "Exclusions Applied",
			out:      "pkg1/file1.go\npkg2/file2.go\npkg3/file3.go\n",
			excludes: []string{"pkg2"},
			fileMap: map[string][]string{
				"pkg1/file1.go": {"pkg1"},
				"pkg2/file2.go": {"pkg2"},
				"pkg3/file3.go": {"pkg3"},
			},
			expected:    []string{"pkg1", "pkg3"},
			expectError: false,
		},
		{
			name:     "Multiple Imports",
			out:      "pkg1/file1.go\n",
			excludes: []string{},
			fileMap: map[string][]string{
				"pkg1/file1.go": {"pkg1", "pkg1/subpkg"},
			},
			expected:    []string{"pkg1", "pkg1/subpkg"},
			expectError: false,
		},
		{
			name:     "Duplicate Packages",
			out:      "pkg1/file1.go\npkg1/file1.go\n",
			excludes: []string{},
			fileMap: map[string][]string{
				"pkg1/file1.go": {"pkg1"},
			},
			expected:    []string{"pkg1"},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			outBuffer := bytes.Buffer{}
			outBuffer.WriteString(tt.out)
			result, err := GetChangedGoPackagesFromDiff(outBuffer, tt.projectPath, tt.excludes, tt.fileMap)
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.ElementsMatch(t, tt.expected, result)
			}
		})
	}
}
