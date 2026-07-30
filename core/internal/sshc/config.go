package sshc

import (
	"fmt"
	"os"
	"time"

	"github.com/kevinburke/ssh_config"
	"github.com/spf13/cast"
)

// LoadConfig resolves SSH configuration for a specific host alias.
func LoadConfig(hostAlias string, configPath string) (*Host, error) {
	if configPath == "" {
		configPath = "$HOME/.ssh/config"
	}
	configPath = os.ExpandEnv(configPath)

	f, err := os.Open(configPath)
	if err != nil {
		return nil, fmt.Errorf("open SSH config file %s: %w", configPath, err)
	}
	defer f.Close()

	sshConfig, err := ssh_config.Decode(f)
	if err != nil {
		return nil, fmt.Errorf("parse SSH config: %w", err)
	}

	getValue := func(key, defaultValue string) string {
		value, _ := sshConfig.Get(hostAlias, key)
		if value == "" {
			return defaultValue
		}
		return value
	}

	host := &Host{
		Host:         hostAlias,
		User:         getValue("User", os.Getenv("USER")),
		Hostname:     getValue("HostName", hostAlias),
		Port:         getValue("Port", "22"),
		IdentityFile: getValue("IdentityFile", "$HOME/.ssh/id_rsa"),
		Timeout:      cast.ToDuration(getValue("ConnectTimeout", "10")) * time.Second,
	}
	host.IdentityFile = os.ExpandEnv(host.IdentityFile)

	return host, nil
}
