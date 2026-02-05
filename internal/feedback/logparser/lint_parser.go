package logparser

import (
	"regexp"
	"strconv"
	"strings"
)

func init() {
	RegisterParser(&LintParser{})
}

// LintParser parses golangci-lint output from GitHub Actions logs.
type LintParser struct{}

var (
	// GitHub Actions annotation format: ##[error]file:line:col: message (linter)
	lintErrorPattern = regexp.MustCompile(`##\[error\]([^:]+):(\d+):(\d+):\s*(.+)\s*\(([^)]+)\)`)

	// Alternative format without column: ##[error]file:line: message (linter)
	lintErrorNoColPattern = regexp.MustCompile(`##\[error\]([^:]+):(\d+):\s*(.+)\s*\(([^)]+)\)`)
)

// CanParse returns true if the log contains lint errors in GitHub Actions format.
// Uses regex patterns that match any linter name, avoiding a hardcoded list.
func (p *LintParser) CanParse(logContent string) bool {
	return lintErrorPattern.MatchString(logContent) || lintErrorNoColPattern.MatchString(logContent)
}

// Parse extracts lint errors from the log content.
func (p *LintParser) Parse(logContent string) ([]Failure, error) {
	cleaned := CleanLog(logContent)
	lines := strings.Split(cleaned, "\n")

	var failures []Failure
	seen := make(map[string]bool)

	for _, line := range lines {
		// Try full format with column
		if matches := lintErrorPattern.FindStringSubmatch(line); len(matches) == 6 {
			file := matches[1]
			lineNum, _ := strconv.Atoi(matches[2])
			col, _ := strconv.Atoi(matches[3])
			message := strings.TrimSpace(matches[4])
			linter := matches[5]

			key := file + ":" + matches[2] + ":" + matches[3]
			if !seen[key] {
				seen[key] = true
				failures = append(failures, Failure{
					Name:    linter,
					File:    file,
					Line:    lineNum,
					Column:  col,
					Message: message,
				})
			}
			continue
		}

		// Try format without column
		if matches := lintErrorNoColPattern.FindStringSubmatch(line); len(matches) == 5 {
			file := matches[1]
			lineNum, _ := strconv.Atoi(matches[2])
			message := strings.TrimSpace(matches[3])
			linter := matches[4]

			key := file + ":" + matches[2]
			if !seen[key] {
				seen[key] = true
				failures = append(failures, Failure{
					Name:    linter,
					File:    file,
					Line:    lineNum,
					Column:  0,
					Message: message,
				})
			}
		}
	}

	return failures, nil
}

// FormatFailure returns a human-readable representation of the failure.
func FormatFailure(f Failure) string {
	if f.Column > 0 {
		return f.File + ":" + strconv.Itoa(f.Line) + ":" + strconv.Itoa(f.Column) + ": " + f.Message + " (" + f.Name + ")"
	}
	return f.File + ":" + strconv.Itoa(f.Line) + ": " + f.Message + " (" + f.Name + ")"
}

// GroupByName groups failures by name (e.g., linter name, test name).
func GroupByName(failures []Failure) map[string][]Failure {
	groups := make(map[string][]Failure)
	for _, f := range failures {
		groups[f.Name] = append(groups[f.Name], f)
	}
	return groups
}
