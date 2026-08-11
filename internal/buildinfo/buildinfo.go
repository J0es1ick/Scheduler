package buildinfo

import "runtime"

var (
	Version   = "dev"
	Commit    = "local"
	BuildTime = "unknown"
)

func Values() map[string]string {
	return map[string]string{
		"version":    Version,
		"commit":     Commit,
		"build_time": BuildTime,
		"go_version": runtime.Version(),
	}
}
