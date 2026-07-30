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
	arupa "github.com/SteelDrEgg/arupa-sdk/golang"
)

// connectSSH creates an SSH client/session from a browser connect_ssh event.
func (s *sshServer) connectSSH(ctx context.Context, event arupa.SocketEvent, emitter arupa.Emitter) error {
	req, err := parseConnectRequest(event.Payload)
	if err != nil {
		return emitError(emitter, event.SocketID, "Invalid connection data: "+err.Error())
	}

	hostConfig, authMethods, err := s.prepareSSH(req)
	if err != nil {
		return emitError(emitter, event.SocketID, err.Error())
	}
	req.Password = ""
	req.Passphrase = ""

	connectCtx, pending := s.startConnection(ctx, event.SocketID, hostConfig.Timeout)
	sshSess, stdout, err := openSSH(connectCtx, hostConfig, authMethods)
	if err != nil {
		return s.emitConnectError(connectCtx, event.SocketID, pending, emitter, err.Error())
	}

	if !s.activateSession(event.SocketID, pending, sshSess) {
		sshSess.close()
		return nil
	}
	go s.pipeOutput(event.SocketID, stdout, sshSess)

	return arupa.EmitJSON(emitter, socketNamespace, event.SocketID, eventSSHConnected, map[string]any{
		"host": req.Host,
		"port": req.Port,
		"user": req.Username,
	})
}

// emitConnectError reports a failed current attempt. Cancellation caused by a
// disconnect or a newer attempt is expected and does not produce a stale UI
// error; a deadline still produces a useful timeout message.
func (s *sshServer) emitConnectError(connectCtx context.Context, socketID string, pending *pendingConnection, emitter arupa.Emitter, message string) error {
	if !s.finishConnection(socketID, pending) {
		return nil
	}
	if connectCtx.Err() == context.Canceled {
		return nil
	}
	if connectCtx.Err() == context.DeadlineExceeded {
		message = "SSH connection timed out"
	}
	return emitError(emitter, socketID, message)
}

func emitError(emitter arupa.Emitter, socketID, message string) error {
	return arupa.EmitJSON(emitter, socketNamespace, socketID, eventSSHError, message)
}

// parseConnectRequest decodes and normalizes the first Socket.IO argument.
func parseConnectRequest(payload []byte) (connectRequest, error) {
	var req connectRequest
	if err := decodeFirstArg(payload, &req); err != nil {
		return req, err
	}
	return normalizeConnectRequest(req)
}

func normalizeConnectRequest(req connectRequest) (connectRequest, error) {
	req.Host = strings.TrimSpace(req.Host)
	req.Port = strings.TrimSpace(req.Port)
	req.Username = strings.TrimSpace(req.Username)
	req.PrivateKey = expandHome(strings.TrimSpace(req.PrivateKey))
	if req.Port == "" {
		req.Port = "22"
	}
	if req.Host == "" || req.Username == "" {
		return req, fmt.Errorf("host and username are required")
	}
	port, err := strconv.Atoi(req.Port)
	if err != nil || port < 1 || port > 65535 {
		return req, fmt.Errorf("port must be between 1 and 65535")
	}
	return req, nil
}

func (s *sshServer) prepareSSH(req connectRequest) (*sshc.Host, []ssh.AuthMethod, error) {
	hostConfig := s.resolveHostConfig(req)
	authMethods, err := s.authMethods(req, hostConfig)
	if err != nil {
		return nil, nil, err
	}
	return hostConfig, authMethods, nil
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

// resolveHostConfig loads ~/.ssh/config aliases when appropriate, then applies
// explicit values sent by the browser.
func (s *sshServer) resolveHostConfig(req connectRequest) *sshc.Host {
	if shouldLoadSSHConfig(req.Host) {
		if cfg, err := sshc.LoadConfig(req.Host, expandHome(s.sshConfigPath)); err == nil {
			if req.Username != "" {
				cfg.User = req.Username
			}
			if req.Port != "" {
				cfg.Port = req.Port
			}
			return cfg
		}
	}

	return &sshc.Host{
		User:     req.Username,
		Host:     req.Host,
		Hostname: req.Host,
		Port:     req.Port,
		Timeout:  30 * time.Second,
	}
}

// shouldLoadSSHConfig reports whether host looks like an SSH config alias.
func shouldLoadSSHConfig(host string) bool {
	return !strings.Contains(host, ".") && host != "localhost"
}

// authMethods builds SSH auth methods from password, explicit key, SSH config
// IdentityFile, or common default key paths.
func (s *sshServer) authMethods(req connectRequest, hostConfig *sshc.Host) ([]ssh.AuthMethod, error) {
	var methods []ssh.AuthMethod
	if req.Password != "" {
		methods = append(methods, ssh.Password(req.Password))
	}

	for _, keyPath := range candidateKeyPaths(req, hostConfig) {
		auth, err := sshc.LoadAuth("", []*sshc.Identity{{
			KeyPath:    expandHome(keyPath),
			Passphrase: req.Passphrase,
		}})
		if err == nil {
			methods = append(methods, auth...)
			if req.PrivateKey == "" {
				break
			}
		}
	}

	if len(methods) == 0 && req.Password == "" && req.PrivateKey == "" {
		for _, keyPath := range defaultKeyPaths() {
			auth, err := sshc.LoadAuth("", []*sshc.Identity{{
				KeyPath:    keyPath,
				Passphrase: req.Passphrase,
			}})
			if err == nil {
				methods = append(methods, auth...)
				break
			}
		}
	}

	if len(methods) == 0 {
		return nil, fmt.Errorf("no valid authentication method provided. Please provide either a password or a valid private key")
	}
	return methods, nil
}

// candidateKeyPaths returns the first-pass key paths for the connection.
func candidateKeyPaths(req connectRequest, hostConfig *sshc.Host) []string {
	if req.PrivateKey != "" {
		return []string{req.PrivateKey}
	}
	if hostConfig.IdentityFile != "" {
		return []string{hostConfig.IdentityFile}
	}
	if req.Password == "" {
		return defaultKeyPaths()
	}
	return nil
}

// defaultKeyPaths lists the conventional private keys tried when no auth is
// explicitly supplied.
func defaultKeyPaths() []string {
	return []string{"$HOME/.ssh/id_rsa", "$HOME/.ssh/id_ed25519", "$HOME/.ssh/id_ecdsa"}
}
