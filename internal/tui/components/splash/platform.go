package splash

import "runtime"

// Platform represents the operating system.
type Platform int

const (
	PlatformDarwin Platform = iota
	PlatformLinux
	PlatformWindows
	PlatformUnknown
)

// DetectPlatform returns the current platform from runtime.GOOS.
func DetectPlatform() Platform {
	switch runtime.GOOS {
	case "darwin":
		return PlatformDarwin
	case "linux":
		return PlatformLinux
	case "windows":
		return PlatformWindows
	default:
		return PlatformUnknown
	}
}
