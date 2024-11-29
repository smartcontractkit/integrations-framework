package framework

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
)

// checkNodeLogsErrors check Chainlink nodes logs for error levels
func checkNodeLogErrors(dir string) error {
	fileRegex := regexp.MustCompile(`^node.*\.log$`)
	logLevelRegex := regexp.MustCompile(`(CRIT|PANIC|FATAL)`)

	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && fileRegex.MatchString(info.Name()) {
			file, err := os.Open(path)
			if err != nil {
				return fmt.Errorf("failed to open file %s: %w", path, err)
			}
			defer file.Close()

			scanner := bufio.NewScanner(file)
			lineNumber := 1
			for scanner.Scan() {
				line := scanner.Text()
				if logLevelRegex.MatchString(line) {
					return fmt.Errorf("file %s contains a matching log level at line %d: %s", path, lineNumber, line)
				}
				lineNumber++
			}
			if err := scanner.Err(); err != nil {
				return fmt.Errorf("error reading file %s: %w", path, err)
			}
		}
		return nil
	})
	return err
}
