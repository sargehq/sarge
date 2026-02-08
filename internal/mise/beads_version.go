package mise

import (
	"fmt"
	"os"
	"regexp"
	"strings"
)

// BeadsToolKey is the mise tool key for beads.
const BeadsToolKey = `"aqua:steveyegge/beads"`

// beadsLineRegexp matches the beads tool line in a mise config (commented or not).
// Captures: (1) optional comment prefix, (2) version string.
var beadsLineRegexp = regexp.MustCompile(
	`^(\s*#?\s*)` + regexp.QuoteMeta(BeadsToolKey) + `\s*=\s*"([^"]*)"`,
)

// RequiredBeadsVersion returns the beads version that sarge requires,
// extracted from the embedded mise template.
func RequiredBeadsVersion() string {
	for _, line := range strings.Split(miseTemplateText, "\n") {
		if m := beadsLineRegexp.FindStringSubmatch(line); m != nil {
			return m[2]
		}
	}
	return ""
}

// ReadBeadsVersion reads the beads version from a mise config file.
// Returns the version string (e.g. "v0.49.2") and whether the line is commented out.
// Returns ("", false, nil) if no beads line is found.
func ReadBeadsVersion(configPath string) (version string, commented bool, err error) {
	data, err := os.ReadFile(configPath) //nolint:gosec // path from trusted caller
	if err != nil {
		return "", false, fmt.Errorf("failed to read mise config: %w", err)
	}

	for _, line := range strings.Split(string(data), "\n") {
		if m := beadsLineRegexp.FindStringSubmatch(line); m != nil {
			prefix := strings.TrimSpace(m[1])
			isCommented := strings.HasPrefix(prefix, "#")
			return m[2], isCommented, nil
		}
	}
	return "", false, nil
}

// UpdateBeadsVersion updates the beads version in a mise config file.
// It replaces the version on the existing beads tool line.
// Returns true if the file was modified, false if no beads line was found or version already matches.
func UpdateBeadsVersion(configPath, newVersion string) (bool, error) {
	data, err := os.ReadFile(configPath) //nolint:gosec // path from trusted caller
	if err != nil {
		return false, fmt.Errorf("failed to read mise config: %w", err)
	}

	lines := strings.Split(string(data), "\n")
	modified := false
	for i, line := range lines {
		if m := beadsLineRegexp.FindStringSubmatch(line); m != nil {
			if m[2] == newVersion {
				return false, nil // already correct
			}
			// Replace just the version in the line
			lines[i] = beadsLineRegexp.ReplaceAllString(line,
				`${1}`+BeadsToolKey+` = "`+newVersion+`"`)
			modified = true
			break
		}
	}

	if !modified {
		return false, nil
	}

	if err := os.WriteFile(configPath, []byte(strings.Join(lines, "\n")), 0600); err != nil {
		return false, fmt.Errorf("failed to write mise config: %w", err)
	}
	return true, nil
}
