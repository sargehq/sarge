package mise

//go:generate moq -stub -out mise_mock.go . Operations:MiseOperationsMock

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Operations defines the interface for mise operations.
// This abstraction enables testing without actual mise commands.
// Each Operations instance is bound to a specific directory.
type Operations interface {
	// IsManaged returns true if the directory has a mise config file.
	IsManaged() bool
	// Trust runs `mise trust` in the directory.
	Trust() error
	// Install runs `mise install` in the directory.
	Install() error
	// HasTask checks if a mise task exists in the directory.
	HasTask(taskName string) bool
	// RunTask runs a mise task in the directory.
	RunTask(taskName string) error
	// Exec runs a command with the mise environment in the directory.
	Exec(command string, args ...string) ([]byte, error)
	// Initialize runs mise trust, install, and setup task if available.
	Initialize() error
	// InitializeWithOutput runs mise trust, install, and setup task if available,
	// writing progress messages to the provided writer.
	InitializeWithOutput(w io.Writer) error
}

// cliOperations implements Operations using the mise CLI.
type cliOperations struct {
	dir string
}

// Compile-time check that cliOperations implements Operations.
var _ Operations = (*cliOperations)(nil)

// NewOperations creates a new Operations instance bound to the specified directory.
func NewOperations(dir string) Operations {
	return &cliOperations{dir: dir}
}

// configFiles lists all mise config file locations to check.
var configFiles = []string{
	".mise.toml",
	"mise.toml",
	".mise/config.toml",
	".tool-versions",
}

// FindConfigFile returns the first mise config file found in dir, or empty string if none.
func FindConfigFile(dir string) string {
	return findConfigFile(dir)
}

// findConfigFile returns the first mise config file found in dir, or empty string if none.
func findConfigFile(dir string) string {
	for _, file := range configFiles {
		path := filepath.Join(dir, file)
		if _, err := os.Stat(path); err == nil {
			return file
		}
	}
	return ""
}

// IsManaged implements Operations.IsManaged.
func (c *cliOperations) IsManaged() bool {
	return findConfigFile(c.dir) != ""
}

// IsManaged returns true if the directory has a mise config file.
func IsManaged(dir string) bool {
	return NewOperations(dir).IsManaged()
}

// Trust implements Operations.Trust.
func (c *cliOperations) Trust() error {
	cmd := exec.Command("mise", "trust")
	cmd.Dir = c.dir
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("mise trust failed: %w\n%s", err, output)
	}
	return nil
}

// Install implements Operations.Install.
func (c *cliOperations) Install() error {
	cmd := exec.Command("mise", "install")
	cmd.Dir = c.dir
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("mise install failed: %w\n%s", err, output)
	}
	return nil
}

// HasTask implements Operations.HasTask.
func (c *cliOperations) HasTask(taskName string) bool {
	cmd := exec.Command("mise", "task", "ls")
	cmd.Dir = c.dir
	output, err := cmd.Output()
	if err != nil {
		return false
	}
	// Check if task name appears in output
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) > 0 && fields[0] == taskName {
			return true
		}
	}
	return false
}

// RunTask implements Operations.RunTask.
func (c *cliOperations) RunTask(taskName string) error {
	cmd := exec.Command("mise", "run", taskName)
	cmd.Dir = c.dir
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("mise run %s failed: %w\n%s", taskName, err, output)
	}
	return nil
}

// Exec implements Operations.Exec.
func (c *cliOperations) Exec(command string, args ...string) ([]byte, error) {
	miseArgs := append([]string{"exec", "--"}, command)
	miseArgs = append(miseArgs, args...)
	// #nosec G204 -- command and args are from trusted internal callers
	cmd := exec.Command("mise", miseArgs...)
	cmd.Dir = c.dir
	output, err := cmd.CombinedOutput()
	if err != nil {
		return output, fmt.Errorf("mise exec %s failed: %w\n%s", command, err, output)
	}
	return output, nil
}

// Exec runs a command with the mise environment in the given directory.
func Exec(dir, command string, args ...string) ([]byte, error) {
	return NewOperations(dir).Exec(command, args...)
}

// Initialize implements Operations.Initialize.
func (c *cliOperations) Initialize() error {
	return c.InitializeWithOutput(os.Stdout)
}

// Initialize runs mise trust, install, and setup task if available.
func Initialize(dir string) error {
	return NewOperations(dir).Initialize()
}

// InitializeWithOutput implements Operations.InitializeWithOutput.
func (c *cliOperations) InitializeWithOutput(w io.Writer) error {
	configFile := findConfigFile(c.dir)
	if configFile == "" {
		fmt.Fprintf(w, "  Mise: not enabled (no config file found)\n")
		return nil
	}

	fmt.Fprintf(w, "  Mise: found %s\n", configFile)

	fmt.Fprintf(w, "  Mise: running trust...\n")
	if err := c.Trust(); err != nil {
		return err
	}

	fmt.Fprintf(w, "  Mise: running install...\n")
	if err := c.Install(); err != nil {
		return err
	}

	// Run setup task if it exists
	if c.HasTask("setup") {
		fmt.Fprintf(w, "  Mise: running setup task...\n")
		if err := c.RunTask("setup"); err != nil {
			return err
		}
	}

	fmt.Fprintf(w, "  Mise: initialization complete\n")
	return nil
}

// InitializeWithOutput runs mise trust, install, and setup task if available.
func InitializeWithOutput(dir string, w io.Writer) error {
	return NewOperations(dir).InitializeWithOutput(w)
}
