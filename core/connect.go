package main

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"

	"arupa-ssh/core/internal/sshc"
)

// sshApplication contains SSH behavior shared by the WebSocket adapter and
// other future transports. It owns target resolution, authentication, and
// opening the SSH PTY; it does not know about HTTP or WebSocket messages.
type sshApplication struct {
	secrets        secretManager
	sshConfigPath  string
	knownHostsPath string
}

func (app *sshApplication) configure(settings sshSettings) {
	app.sshConfigPath = expandHome(settings.SSHConfigPath)
	app.knownHostsPath = expandHome(strings.TrimSpace(settings.KnownHostsPath))
}

func normalizeSSHConnectInput(req sshConnectInput) (sshConnectInput, error) {
	req.TargetType = strings.TrimSpace(req.TargetType)
	if req.TargetType == "" {
		req.TargetType = targetTypeAddress
	}
	req.Host = strings.TrimSpace(req.Host)
	req.Port = strings.TrimSpace(req.Port)
	req.Username = strings.TrimSpace(req.Username)
	req.Auth.Method = strings.TrimSpace(req.Auth.Method)
	req.Auth.SecretName = strings.TrimSpace(req.Auth.SecretName)
	req.Auth.PrivateKey = expandHome(strings.TrimSpace(req.Auth.PrivateKey))

	if req.Host == "" {
		return req, fmt.Errorf("host is required")
	}
	switch req.TargetType {
	case targetTypeAddress:
		if req.Username == "" {
			return req, fmt.Errorf("username is required for a direct address")
		}
		if req.Port == "" {
			req.Port = "22"
		}
	case targetTypeSSHConfig:
		// Username and port may be supplied by the selected SSH config alias.
	default:
		return req, fmt.Errorf("unsupported target type %q", req.TargetType)
	}
	if req.Port != "" {
		port, err := strconv.Atoi(req.Port)
		if err != nil || port < 1 || port > 65535 {
			return req, fmt.Errorf("port must be between 1 and 65535")
		}
	}

	switch req.Auth.Method {
	case authTypePassword:
		if req.Auth.Password == "" {
			return req, fmt.Errorf("password is required")
		}
		if req.Auth.SecretName != "" || req.Auth.SecretPassphrase != "" ||
			req.Auth.PrivateKey != "" || req.Auth.Passphrase != "" {
			return req, fmt.Errorf("password authentication contains fields for another method")
		}
	case authTypeSecret:
		if req.Auth.SecretName == "" {
			return req, fmt.Errorf("Secret Manager entry is required")
		}
		if req.Auth.Password != "" || req.Auth.PrivateKey != "" || req.Auth.Passphrase != "" {
			return req, fmt.Errorf("Secret Manager authentication contains fields for another method")
		}
	case authTypeKey:
		if req.Auth.Password != "" || req.Auth.SecretName != "" || req.Auth.SecretPassphrase != "" {
			return req, fmt.Errorf("key authentication contains fields for another method")
		}
	default:
		return req, fmt.Errorf("unsupported authentication method %q", req.Auth.Method)
	}
	return req, nil
}

// Connect resolves one explicitly selected target and authentication method,
// then opens its SSH shell. The caller owns cancellation and the returned
// session lifecycle.
func (app *sshApplication) Connect(
	ctx context.Context,
	request sshConnectInput,
) (*sshSession, io.Reader, error) {
	req, err := normalizeSSHConnectInput(request)
	if err != nil {
		return nil, nil, err
	}
	host, err := app.resolveHostConfig(req)
	if err != nil {
		return nil, nil, err
	}

	connectContext, cancel := context.WithTimeout(ctx, host.Timeout)
	defer cancel()
	authMethods, err := app.authMethods(connectContext, req, host)
	if err != nil {
		return nil, nil, err
	}
	return openSSH(connectContext, host, authMethods)
}

func openSSH(ctx context.Context, hostConfig *sshc.Host, authMethods []ssh.AuthMethod) (*sshSession, io.Reader, error) {
	sshClient, err := sshc.Connect(ctx, hostConfig, authMethods)
	if err != nil {
		return nil, nil, fmt.Errorf("SSH connection failed: %w", err)
	}

	session, err := sshClient.NewSession()
	if err != nil {
		_ = sshClient.Close()
		return nil, nil, fmt.Errorf("failed to create SSH session: %w", err)
	}

	stdin, stdout, err := sshc.SetupTerminal(session, 24, 80)
	if err != nil {
		_ = session.Close()
		_ = sshClient.Close()
		return nil, nil, fmt.Errorf("failed to setup terminal: %w", err)
	}
	if err := session.Shell(); err != nil {
		_ = stdin.Close()
		_ = session.Close()
		_ = sshClient.Close()
		return nil, nil, fmt.Errorf("failed to start shell: %w", err)
	}
	return newSSHSession(sshClient, session, stdin), stdout, nil
}

func (app *sshApplication) resolveHostConfig(req sshConnectInput) (*sshc.Host, error) {
	var host *sshc.Host
	switch req.TargetType {
	case targetTypeAddress:
		host = &sshc.Host{
			User:     req.Username,
			Host:     req.Host,
			Hostname: req.Host,
			Port:     req.Port,
			Timeout:  30 * time.Second,
		}
	case targetTypeSSHConfig:
		configPath := app.sshConfigPath
		if configPath == "" {
			configPath = expandHome("~/.ssh/config")
		}
		var err error
		host, err = sshc.LoadConfig(req.Host, configPath)
		if err != nil {
			return nil, err
		}
		if req.Username != "" {
			host.User = req.Username
		}
		if req.Port != "" {
			host.Port = req.Port
		}
	default:
		return nil, fmt.Errorf("unsupported target type %q", req.TargetType)
	}
	if host.User == "" || host.Hostname == "" {
		return nil, fmt.Errorf("SSH target did not resolve a hostname and username")
	}
	if host.Port == "" {
		host.Port = "22"
	}
	port, err := strconv.Atoi(host.Port)
	if err != nil || port < 1 || port > 65535 {
		return nil, fmt.Errorf("resolved SSH port must be between 1 and 65535")
	}
	if host.Timeout <= 0 {
		host.Timeout = 30 * time.Second
	}
	host.KnownHostsPath = app.knownHostsPath
	return host, nil
}

func (app *sshApplication) authMethods(
	ctx context.Context,
	req sshConnectInput,
	host *sshc.Host,
) ([]ssh.AuthMethod, error) {
	switch req.Auth.Method {
	case authTypePassword:
		return []ssh.AuthMethod{ssh.Password(req.Auth.Password)}, nil
	case authTypeSecret:
		if app.secrets == nil {
			return nil, fmt.Errorf("Secret Manager is unavailable")
		}
		password, err := app.secrets.Reveal(ctx, req.Auth.SecretName, req.Auth.SecretPassphrase)
		if err != nil {
			return nil, fmt.Errorf("reveal Secret Manager entry: %w", err)
		}
		if password == "" {
			return nil, fmt.Errorf("Secret Manager entry is empty")
		}
		return []ssh.AuthMethod{ssh.Password(password)}, nil
	case authTypeKey:
		keyPaths := candidateKeyPaths(req, host)
		identities := make([]*sshc.Identity, 0, len(keyPaths))
		for _, path := range keyPaths {
			identities = append(identities, &sshc.Identity{
				KeyPath:    expandHome(path),
				Passphrase: req.Auth.Passphrase,
			})
		}
		return sshc.LoadAuth("", identities)
	default:
		return nil, fmt.Errorf("unsupported authentication method %q", req.Auth.Method)
	}
}

func candidateKeyPaths(req sshConnectInput, host *sshc.Host) []string {
	if req.Auth.PrivateKey != "" {
		return []string{req.Auth.PrivateKey}
	}
	if req.TargetType == targetTypeSSHConfig && host.IdentityFile != "" {
		return []string{host.IdentityFile}
	}
	return defaultKeyPaths()
}

func defaultKeyPaths() []string {
	return []string{"$HOME/.ssh/id_ed25519", "$HOME/.ssh/id_ecdsa", "$HOME/.ssh/id_rsa"}
}
