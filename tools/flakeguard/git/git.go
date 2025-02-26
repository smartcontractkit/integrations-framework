package git

// TODO: I found this a little too late, but we can probably make good use of https://github.com/go-git/go-git

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"

	"dario.cat/mergo"
	"github.com/rs/zerolog/log"
	"github.com/smartcontractkit/chainlink-testing-framework/tools/flakeguard/utils"
)

// FindChangedFiles executes a git diff against a specified base reference and pipes the output through a user-defined grep command or sequence.
// The baseRef parameter specifies the base git reference for comparison (e.g., "main", "develop").
// The filterCmd parameter should include the full command to be executed after git diff, such as "grep '_test.go$'" or "grep -v '_test.go$' | sort".
func FindChangedFiles(rootGoModPath, baseRef, filterCmd string) ([]string, error) {
	// Find directories containing a go.mod file and build an exclusion string
	excludeStr, err := buildExcludeStringForGoModDirs(rootGoModPath)
	if err != nil {
		return nil, fmt.Errorf("error finding go.mod directories: %w", err)
	}

	// First command to list files changed between the baseRef and HEAD, excluding specified paths
	diffCmdStr := fmt.Sprintf("git diff --name-only --diff-filter=AM %s...HEAD -- %s %s", baseRef, rootGoModPath, excludeStr)
	diffCmd := exec.Command("bash", "-c", diffCmdStr)

	// Using a buffer to capture stdout and a separate buffer for stderr
	var out bytes.Buffer
	var errBuf bytes.Buffer
	diffCmd.Stdout = &out
	diffCmd.Stderr = &errBuf

	// Running the diff command
	if err := diffCmd.Run(); err != nil {
		return nil, fmt.Errorf("error executing git diff command: %s; error: %w; stderr: %s", diffCmdStr, err, errBuf.String())
	}

	// Check if there are any files listed; if not, return an empty slice
	diffOutput := strings.TrimSpace(out.String())
	if diffOutput == "" {
		return []string{}, nil
	}

	// Second command to filter files using grepCmd
	grepCmdStr := fmt.Sprintf("echo '%s' | %s", diffOutput, filterCmd)
	grepCmd := exec.Command("bash", "-c", grepCmdStr)

	// Reset buffers for reuse
	out.Reset()
	errBuf.Reset()
	grepCmd.Stdout = &out
	grepCmd.Stderr = &errBuf

	// Running the grep command
	if err := grepCmd.Run(); err != nil {
		if exiterr, ok := err.(*exec.ExitError); ok {
			if status, ok := exiterr.Sys().(syscall.WaitStatus); ok {
				if status.ExitStatus() == 1 {
					// Exit status 1 for grep means no lines matched, which is not an error in this context
					return []string{}, nil
				}
			}
		}
		return nil, fmt.Errorf("error executing grep command: %s; error: %w; stderr: %s", grepCmdStr, err, errBuf.String())
	}

	// Preparing the final list of files
	files := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(files) == 1 && files[0] == "" {
		return []string{}, nil
	}

	return files, nil
}

// buildExcludeStringForGoModDirs searches the given root directory for subdirectories
// containing a go.mod file and returns a formatted string to exclude those directories
// (except the root directory if it contains a go.mod file) from git diff.
func buildExcludeStringForGoModDirs(rootGoModPath string) (string, error) {
	var excludeStr string

	err := filepath.Walk(rootGoModPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.Name() == "go.mod" {
			dir := filepath.Dir(path)
			// Skip excluding the root directory if go.mod is found there
			if dir != rootGoModPath {
				excludeStr += fmt.Sprintf("':(exclude)%s/**' ", dir)
			}
		}
		return nil
	})
	if err != nil {
		return "", err
	}

	return excludeStr, nil
}

func Diff(baseBranch string) (*utils.CmdOutput, error) {
	return utils.ExecuteCmd("git", "diff", "--name-only", baseBranch)
}

func ModDiff(baseBranch, projectPath string) (*utils.CmdOutput, error) {
	return utils.ExecuteCmd("git", "diff", baseBranch, "--unified=0", "--", filepath.Join(projectPath, "go.mod"))
}

func GetGoModChangesFromDiff(lines bytes.Buffer) ([]string, error) {
	changedLines := strings.Split(lines.String(), "\n")

	// Filter out lines that do not indicate package changes
	var packages []string
	for _, line := range changedLines {
		if strings.HasPrefix(line, "+") {
			// ignore comments or empty lines (e.g., not relevant)
			if strings.HasPrefix(line, "+ ") || strings.HasPrefix(line, "+++ ") {
				continue
			}

			fields := strings.Fields(line)
			if len(fields) > 1 {
				// The second field should contains the module path
				packages = append(packages, fields[1])
			}
		}
	}

	return packages, nil
}

// GetChangedGoPackagesFromDiff identifies the Go packages affected by changes in a Git diff output.
// It analyzes a buffer containing the output of a 'git diff' command, filtering the list of changed
// files to determine which Go packages have been affected based on the project's file map.
//
// Parameters:
//   - out: A bytes.Buffer containing the 'git diff' command output. This output should list the
//     changed files, one per line.
//   - projectPath: The root directory of the project within the repository. This parameter is
//     used to filter files based on their paths. [Note: This functionality is currently commented out,
//     ensure to uncomment the related line if you decide to use it.]
//   - excludes: A slice of strings representing paths to exclude from the analysis. This can be useful
//     to ignore changes in certain directories or files that are not relevant to the package analysis.
//   - fileMap: A map where the key is a file path and the value is a slice of strings representing the
//     Go import paths of the packages that file belongs to. This map is used to map changed files
//     to their respective packages.
//
// Returns:
//   - A slice of strings representing the unique Go packages that have changes. These packages are
//     identified by their import paths.
//   - An error, which is nil in the current implementation but can be used to return errors encountered
//     during the execution of the function.
func GetChangedGoPackagesFromDiff(out bytes.Buffer, projectPath string, excludes []string, fileMap map[string][]string) ([]string, error) {
	changedFiles := strings.Split(out.String(), "\n")

	// Filter out non-Go files and directories and embeds
	changedPackages := make(map[string]bool)
	for _, file := range changedFiles {
		if file == "" || shouldExclude(excludes, file) {
			continue
		}

		// if the changed file is in the fileMap then we add it to the changed packages
		for _, importPath := range fileMap[file] {
			changedPackages[importPath] = true
		}
	}

	// Convert map keys to slice
	var packages []string
	for pkg := range changedPackages {
		packages = append(packages, pkg)
	}

	return packages, nil
}

func shouldExclude(excludes []string, item string) bool {
	for _, v := range excludes {
		if strings.HasPrefix(item, v) {
			return true
		}
	}
	return false
}

// Data holds standard git data we're interested in
type Data struct {
	RepoPath      string
	RepoURL       string
	CurrentBranch string
	DefaultBranch string
	HeadSHA       string
	userOverride  bool
}

// empty checks if the struct is empty with default values
func (g *Data) empty() bool {
	return g.RepoPath == "" && g.CurrentBranch == "" && g.DefaultBranch == "" && g.HeadSHA == "" && g.RepoURL == ""
}

// log prints out the inferred git data
func (g *Data) log() {
	var (
		inferredAny bool
		logMsg      = log.Debug()
	)

	if g.RepoPath != "" {
		logMsg = logMsg.Str("repo path", g.RepoPath)
		inferredAny = true
	}
	if g.CurrentBranch != "" {
		logMsg = logMsg.Str("current branch", g.CurrentBranch)
		inferredAny = true
	}
	if g.DefaultBranch != "" {
		logMsg = logMsg.Str("default branch", g.DefaultBranch)
		inferredAny = true
	}
	if g.HeadSHA != "" {
		logMsg = logMsg.Str("head SHA", g.HeadSHA)
		inferredAny = true
	}
	if g.RepoURL != "" {
		logMsg = logMsg.Str("repo URL", g.RepoURL)
		inferredAny = true
	}
	if inferredAny {
		msg := "Inferred git data"
		if g.userOverride {
			msg += " with user provided overrides"
		}
		logMsg.Msg(msg)
	} else {
		log.Warn().Msg("No git data inferred")
	}
}

// InferData uses 'git' to find out relevant git data about the repo we're in.
// User provided data will override inferred data.
func InferData(userProvidedData *Data) (*Data, error) {
	_, err := exec.LookPath("git")
	if err != nil {
		return nil, fmt.Errorf("git not installed: %w", err)
	}

	cmd := exec.Command("git", "rev-parse", "--is-inside-work-tree")
	combinedOut, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("not inside a git repository: %s: %w", combinedOut, err)
	}

	var (
		stdOut  strings.Builder
		stdErr  strings.Builder
		gitData = &Data{}
	)
	defer gitData.log()

	cmd = exec.Command("git", "rev-parse", "--show-toplevel")
	cmd.Stdout = &stdOut
	cmd.Stderr = &stdErr
	err = cmd.Run()
	if err != nil {
		return nil, fmt.Errorf("error getting repo path: %s: %w", stdErr.String(), err)
	}
	gitData.RepoPath = strings.TrimSpace(stdOut.String())

	stdOut.Reset()
	stdErr.Reset()
	cmd = exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
	cmd.Stdout = &stdOut
	cmd.Stderr = &stdErr
	err = cmd.Run()
	if err != nil {
		return nil, fmt.Errorf("error getting branch name: %s: %w", stdErr.String(), err)
	}
	gitData.CurrentBranch = strings.TrimSpace(stdOut.String())

	stdOut.Reset()
	stdErr.Reset()
	cmd = exec.Command("git", "symbolic-ref", "refs/remotes/origin/HEAD")
	cmd.Stdout = &stdOut
	cmd.Stderr = &stdErr
	err = cmd.Run()
	if err != nil {
		return nil, fmt.Errorf("error getting default branch name: %s: %w", stdErr.String(), err)
	}
	gitData.DefaultBranch = strings.TrimSpace(strings.TrimPrefix(stdOut.String(), "refs/heads/"))

	stdOut.Reset()
	stdErr.Reset()
	cmd = exec.Command("git", "rev-parse", "HEAD")
	cmd.Stdout = &stdOut
	cmd.Stderr = &stdErr
	err = cmd.Run()
	if err != nil {
		return nil, fmt.Errorf("error getting head SHA: %s: %w", stdErr.String(), err)
	}
	gitData.HeadSHA = strings.TrimSpace(stdOut.String())

	stdOut.Reset()
	stdErr.Reset()
	cmd = exec.Command("git", "config", "--get", "remote.origin.url")
	cmd.Stdout = &stdOut
	cmd.Stderr = &stdErr
	err = cmd.Run()
	if err != nil {
		return nil, fmt.Errorf("error getting repo URL: %s: %w", stdErr.String(), err)
	}
	gitData.RepoURL = strings.TrimSpace(stdOut.String())

	if userProvidedData != nil && !userProvidedData.empty() {
		gitData.userOverride = true
		if err = mergo.Merge(&gitData, userProvidedData, mergo.WithOverride); err != nil {
			return nil, fmt.Errorf("error merging user provided GitHub data: %w", err)
		}
	}
	return gitData, nil
}

// HubActionsData holds GitHub Actions specific data we're interested in
type HubActionsData struct {
	IsOnGitHubActions bool
	EventName         string
	BaseSHA           string
	BaseBranch        string
	WorkflowName      string
	WorkflowRunURL    string
	userOverride      bool
}

// empty checks if the struct is empty with default values
func (g *HubActionsData) empty() bool {
	return g.EventName == "" && g.BaseSHA == "" && g.BaseBranch == "" && g.WorkflowName == "" && g.WorkflowRunURL == ""
}

// log prints out the inferred GitHub data
func (g *HubActionsData) log() {
	if !g.IsOnGitHubActions {
		log.Debug().Msg("Not running on GitHub Actions")
		return
	}
	logMsg := log.Debug()
	if g.EventName != "" {
		logMsg = logMsg.Str("event name", g.EventName)
	}
	if g.BaseSHA != "" {
		logMsg = logMsg.Str("base SHA", g.BaseSHA)
	}
	if g.BaseBranch != "" {
		logMsg = logMsg.Str("base branch", g.BaseBranch)
	}
	if g.WorkflowName != "" {
		logMsg = logMsg.Str("workflow name", g.WorkflowName)
	}
	if g.WorkflowRunURL != "" {
		logMsg = logMsg.Str("workflow run URL", g.WorkflowRunURL)
	}
	msg := "Inferred GitHub Actions data"
	if g.userOverride {
		msg += " with user provided overrides"
	}
	logMsg.Msg(msg)
}

// InferGitHubData checks if the code is running on GitHub Actions and infers some data from it.
// User provided data will override inferred data.
// GitHub runners have a lot of data exposed though environment variables.
// https://docs.github.com/en/actions/writing-workflows/choosing-what-your-workflow-does/store-information-in-variables#default-environment-variables
func InferGitHubData(userProvidedData *HubActionsData) (*HubActionsData, error) {
	const (
		isOnGitHubActions = "GITHUB_ACTIONS"
		eventName         = "GITHUB_EVENT_NAME"
		baseRef           = "GITHUB_BASE_REF"
		workflowName      = "GITHUB_WORKFLOW"
		serverURL         = "GITHUB_SERVER_URL"
		repo              = "GITHUB_REPOSITORY"
		runID             = "GITHUB_RUN_ID"
		prEventName       = "pull_request"
		prTargetName      = "pull_request_target"
	)
	var gitHubData = &HubActionsData{}
	defer gitHubData.log()

	gitHubData.IsOnGitHubActions = os.Getenv(isOnGitHubActions) == "true"
	if !gitHubData.IsOnGitHubActions {
		return gitHubData, nil
	}
	gitHubData.EventName = strings.TrimSpace(os.Getenv(eventName))
	gitHubData.WorkflowName = strings.TrimSpace(os.Getenv(workflowName))
	gitHubData.WorkflowRunURL = fmt.Sprintf("%s/%s/actions/runs/%s", os.Getenv(serverURL), os.Getenv(repo), os.Getenv(runID))

	// If we're not in a PR event, we don't need to infer any more data
	if gitHubData.EventName != prEventName && gitHubData.EventName != prTargetName {
		return gitHubData, nil
	}

	gitHubData.BaseBranch = strings.TrimSpace(os.Getenv(baseRef))

	var (
		stdOut strings.Builder
		stdErr strings.Builder
	)
	cmd := exec.Command("git", "rev-parse", fmt.Sprintf("origin/%s", gitHubData.BaseBranch)) //nolint:gosec
	cmd.Stdout = &stdOut
	cmd.Stderr = &stdErr
	err := cmd.Run()
	if err != nil {
		return nil, fmt.Errorf("error getting base SHA: %s: %w", stdErr.String(), err)
	}
	gitHubData.BaseSHA = strings.TrimSpace(stdOut.String())

	if userProvidedData != nil && !userProvidedData.empty() {
		gitHubData.userOverride = true
		if err = mergo.Merge(&gitHubData, userProvidedData, mergo.WithOverride); err != nil {
			return nil, fmt.Errorf("error merging user provided GitHub data: %w", err)
		}
	}
	return gitHubData, nil
}
