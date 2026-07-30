package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// decodeFirstArg decodes the first JSON-encoded legacy socket argument into out.
func decodeFirstArg(payload []byte, out any) error {
	var args []json.RawMessage
	if err := json.Unmarshal(payload, &args); err != nil {
		return err
	}
	if len(args) == 0 {
		return fmt.Errorf("missing payload")
	}
	return json.Unmarshal(args[0], out)
}

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
