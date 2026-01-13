package version

// These variables are set at build time via -ldflags
var (
	// Version is the semantic version (e.g., "1.0.0")
	Version = "dev"
	// Commit is the git commit hash
	Commit = "unknown"
	// BuildTime is the build timestamp
	BuildTime = "unknown"
)

// Info returns formatted version information
func Info() string {
	return Version + " (" + Commit + ")"
}

// Full returns full version information including build time
func Full() string {
	return Version + " (commit: " + Commit + ", built: " + BuildTime + ")"
}
