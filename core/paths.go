package main

import (
	"os"
	"strings"
)

// expandHome expands "~", "~/...", and environment variables in user paths.
func expandHome(path string) string {
	if path == "" {
		return ""
	}
	if path == "~" {
		if home, err := os.UserHomeDir(); err == nil {
			return home
		}
	}
	if strings.HasPrefix(path, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return home + path[1:]
		}
	}
	return os.ExpandEnv(path)
}
