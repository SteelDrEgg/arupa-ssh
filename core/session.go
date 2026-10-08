package main

import (
	"errors"
	"io"
	"sync"

	"golang.org/x/crypto/ssh"
)

// sshSession holds one browser transport's SSH connection and PTY session.
type sshSession struct {
	client  *ssh.Client
	session *ssh.Session
	stdin   io.WriteCloser

	mu        sync.RWMutex
	active    bool
	writeMu   sync.Mutex
	resizeMu  sync.Mutex
	closeOnce sync.Once
}

var errSSHSessionInactive = errors.New("SSH session is not active")

// newSSHSession wraps an established SSH client, session, and stdin pipe.
func newSSHSession(client *ssh.Client, session *ssh.Session, stdin io.WriteCloser) *sshSession {
	return &sshSession{
		client:  client,
		session: session,
		stdin:   stdin,
		active:  true,
	}
}

func (ss *sshSession) write(data []byte) error {
	if ss == nil {
		return errSSHSessionInactive
	}
	ss.writeMu.Lock()
	defer ss.writeMu.Unlock()

	ss.mu.RLock()
	active := ss.active
	stdin := ss.stdin
	ss.mu.RUnlock()
	if !active || stdin == nil {
		return errSSHSessionInactive
	}
	_, err := stdin.Write(data)
	return err
}

func (ss *sshSession) resize(rows, cols int) error {
	if ss == nil {
		return errSSHSessionInactive
	}
	ss.resizeMu.Lock()
	defer ss.resizeMu.Unlock()

	ss.mu.RLock()
	active := ss.active
	session := ss.session
	ss.mu.RUnlock()
	if !active || session == nil {
		return errSSHSessionInactive
	}
	return session.WindowChange(rows, cols)
}

// close tears down all SSH resources owned by the session.
func (ss *sshSession) close() {
	if ss == nil {
		return
	}
	ss.closeOnce.Do(func() {
		ss.mu.Lock()
		ss.active = false
		stdin := ss.stdin
		session := ss.session
		client := ss.client
		ss.mu.Unlock()

		if stdin != nil {
			_ = stdin.Close()
		}
		if session != nil {
			_ = session.Close()
		}
		if client != nil {
			_ = client.Close()
		}
	})
}
