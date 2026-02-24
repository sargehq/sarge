package mise

import (
	"fmt"
	"os"
	"regexp"
	"strings"
)

// BeansToolKey is the mise tool key for beans.
const BeansToolKey = `"go:github.com/hmans/beans"`

// beansLineRegexp matches the beans tool line in a mise config (commented or not).
// Captures: (1) optional comment prefix, (2) version string.
var beansLineRegexp = regexp.MustCompile(
	`^(\s*#?\s*)` + regexp.QuoteMeta(BeansToolKey) + `\s*=\s*"([^"]*)"`,
)

// RequiredBeansVersion returns the beans version that sarge requires,
// extracted from the embedded mise template.
func RequiredBeansVersion() string {
	for _, line := range strings.Split(miseTemplateText, "\n") {
		if m := beansLineRegexp.FindStringSubmatch(line); m != nil {
			return m[2]
		}
	}
	return ""
}

// ReadBeansVersion reads the beans version from a mise config file.
// Returns the version string (e.g. "v0.49.2") and whether the line is commented out.
// Returns ("", false, nil) if no beans line is found.
func ReadBeansVersion(configPath string) (version string, commented bool, err error) {
	data, err := os.ReadFile(configPath) //nolint:gosec // path from trusted caller
	if err != nil {
		return "", false, fmt.Errorf("failed to read mise config: %w", err)
	}

	for _, line := range strings.Split(string(data), "\n") {
		if m := beansLineRegexp.FindStringSubmatch(line); m != nil {
			prefix := strings.TrimSpace(m[1])
			isCommented := strings.HasPrefix(prefix, "#")
			return m[2], isCommented, nil
		}
	}
	return "", false, nil
}

// UpdateBeansVersion updates the beans version in a mise config file.
// It replaces the version on the existing beans tool line.
// Returns true if the file was modified, false if no beans line was found or version already matches.
func UpdateBeansVersion(configPath, newVersion string) (bool, error) {
	data, err := os.ReadFile(configPath) //nolint:gosec // path from trusted caller
	if err != nil {
		return false, fmt.Errorf("failed to read mise config: %w", err)
	}

	lines := strings.Split(string(data), "\n")
	modified := false
	for i, line := range lines {
		if m := beansLineRegexp.FindStringSubmatch(line); m != nil {
			if m[2] == newVersion {
				return false, nil // already correct
			}
			// Replace just the version in the line
			lines[i] = beansLineRegexp.ReplaceAllString(line,
				`${1}`+BeansToolKey+` = "`+newVersion+`"`)
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
