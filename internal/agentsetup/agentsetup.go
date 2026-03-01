// Package agentsetup checks and installs coding-agent integrations (pi extensions).
package agentsetup

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
)

//go:embed extensions/sarge-complete.ts
var sargeCompleteExtension []byte

//go:embed extensions/beans-prime.ts
var beansPrimeExtension []byte

// PiExtensionInstalled reports whether the sarge-complete pi extension exists in the given repo directory.
func PiExtensionInstalled(repoDir string) bool {
	extPath := filepath.Join(repoDir, ".pi", "extensions", "sarge-complete.ts")
	_, err := os.Stat(extPath)
	return err == nil
}

// InstallPiExtension copies the embedded sarge-complete extension into the repo's .pi/extensions/ directory.
func InstallPiExtension(repoDir string) error {
	extDir := filepath.Join(repoDir, ".pi", "extensions")
	if err := os.MkdirAll(extDir, 0o750); err != nil {
		return fmt.Errorf("creating extensions directory: %w", err)
	}
	return os.WriteFile(filepath.Join(extDir, "sarge-complete.ts"), sargeCompleteExtension, 0o600)
}

// BeansPrimeExtensionInstalled reports whether the beans-prime pi extension exists in the given repo directory.
func BeansPrimeExtensionInstalled(repoDir string) bool {
	extPath := filepath.Join(repoDir, ".pi", "extensions", "beans-prime.ts")
	_, err := os.Stat(extPath)
	return err == nil
}

// InstallBeansPrimeExtension copies the embedded beans-prime extension into the repo's .pi/extensions/ directory.
func InstallBeansPrimeExtension(repoDir string) error {
	extDir := filepath.Join(repoDir, ".pi", "extensions")
	if err := os.MkdirAll(extDir, 0o750); err != nil {
		return fmt.Errorf("creating extensions directory: %w", err)
	}
	return os.WriteFile(filepath.Join(extDir, "beans-prime.ts"), beansPrimeExtension, 0o600)
}
