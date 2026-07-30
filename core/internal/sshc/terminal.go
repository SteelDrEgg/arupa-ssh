package sshc

import (
	"fmt"
	"io"

	"golang.org/x/crypto/ssh"
)

// SetupTerminal requests a PTY and returns its interactive streams.
func SetupTerminal(session *ssh.Session, height int, width int) (stdin io.WriteCloser, stdout io.Reader, err error) {
	modes := ssh.TerminalModes{
		ssh.ECHO:          1,
		ssh.ECHOCTL:       0,
		ssh.TTY_OP_ISPEED: 14400,
		ssh.TTY_OP_OSPEED: 14400,
	}
	if err = session.RequestPty("xterm-256color", height, width, modes); err != nil {
		return nil, nil, fmt.Errorf("request pseudo terminal: %w", err)
	}
	stdin, err = session.StdinPipe()
	if err != nil {
		return nil, nil, fmt.Errorf("setup stdin for session: %w", err)
	}
	stdout, err = session.StdoutPipe()
	if err != nil {
		return nil, nil, fmt.Errorf("setup stdout for session: %w", err)
	}
	return stdin, stdout, nil
}
