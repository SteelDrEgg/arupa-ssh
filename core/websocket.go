package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"golang.org/x/crypto/ssh"

	"arupa-ssh/core/internal/sshc"
)

const (
	proxyAuthenticatedHeader = "X-Arupa-Authenticated"
	websocketWriteWait       = 10 * time.Second
	websocketPongWait        = 60 * time.Second
	websocketPingPeriod      = 45 * time.Second
	websocketMaxMessageSize  = 1 << 20
	maxTerminalDimension     = 4096
)

var terminalUpgrader = websocket.Upgrader{
	HandshakeTimeout: 10 * time.Second,
	CheckOrigin:      sameWebSocketOrigin,
}

type websocketEnvelope struct {
	Event string          `json:"event"`
	Data  json.RawMessage `json:"data,omitempty"`
}

type websocketOutbound struct {
	Event string `json:"event"`
	Data  any    `json:"data,omitempty"`
}

// webTerminal owns exactly one browser WebSocket and at most one SSH session.
// It is independent of the SDK and Socket.IO transports; the inherited HTTP
// server is only responsible for constructing and serving it.
type webTerminal struct {
	server *sshServer
	conn   *websocket.Conn
	ctx    context.Context
	cancel context.CancelFunc

	writeMu sync.Mutex

	stateMu       sync.Mutex
	attempt       uint64
	connectCancel context.CancelFunc
	session       *sshSession
	closed        bool

	closeOnce sync.Once
}

func (s *sshServer) handleWebSocket(w http.ResponseWriter, req *http.Request) {
	//if !strings.EqualFold(strings.TrimSpace(req.Header.Get(proxyAuthenticatedHeader)), "true") {
	//	http.Error(w, "authentication required", http.StatusUnauthorized)
	//	return
	//}
	conn, err := terminalUpgrader.Upgrade(w, req, nil)
	if err != nil {
		return
	}

	ctx, cancel := context.WithCancel(req.Context())
	client := &webTerminal{
		server: s,
		conn:   conn,
		ctx:    ctx,
		cancel: cancel,
	}
	defer client.shutdown()
	go client.keepAlive()
	client.readLoop()
}

func sameWebSocketOrigin(req *http.Request) bool {
	origin := strings.TrimSpace(req.Header.Get("Origin"))
	if origin == "" {
		return true
	}
	parsed, err := url.Parse(origin)
	if err != nil {
		return false
	}
	return strings.EqualFold(parsed.Host, req.Host)
}

func (c *webTerminal) readLoop() {
	c.conn.SetReadLimit(websocketMaxMessageSize)
	_ = c.conn.SetReadDeadline(time.Now().Add(websocketPongWait))
	c.conn.SetPongHandler(func(string) error {
		return c.conn.SetReadDeadline(time.Now().Add(websocketPongWait))
	})

	for {
		var message websocketEnvelope
		if err := c.conn.ReadJSON(&message); err != nil {
			return
		}
		if err := c.handle(message); err != nil {
			if sendErr := c.send(eventSSHError, err.Error()); sendErr != nil {
				return
			}
		}
	}
}

func (c *webTerminal) handle(message websocketEnvelope) error {
	switch strings.TrimSpace(message.Event) {
	case eventConnectSSH:
		var request connectRequest
		if err := json.Unmarshal(message.Data, &request); err != nil {
			return fmt.Errorf("invalid connection data: %w", err)
		}
		normalized, err := normalizeConnectRequest(request)
		if err != nil {
			return fmt.Errorf("invalid connection data: %w", err)
		}
		return c.beginConnect(normalized)

	case eventTerminalInput:
		var input string
		if err := json.Unmarshal(message.Data, &input); err != nil {
			return fmt.Errorf("invalid terminal input")
		}
		session := c.currentSession()
		if session == nil {
			return fmt.Errorf("no active SSH session")
		}
		if err := session.write([]byte(input)); err != nil {
			c.disconnectSession()
			return fmt.Errorf("failed to send input")
		}
		return nil

	case eventResize:
		var request resizeRequest
		if err := json.Unmarshal(message.Data, &request); err != nil {
			return fmt.Errorf("invalid terminal size")
		}
		if request.Cols <= 0 || request.Rows <= 0 ||
			request.Cols > maxTerminalDimension || request.Rows > maxTerminalDimension {
			return fmt.Errorf("invalid terminal size")
		}
		session := c.currentSession()
		if session == nil {
			return nil
		}
		if err := session.resize(request.Rows, request.Cols); err != nil &&
			!errors.Is(err, errSSHSessionInactive) {
			return fmt.Errorf("failed to resize terminal: %w", err)
		}
		return nil

	case eventDisconnect:
		c.disconnectSession()
		return c.send(eventSSHDisconnected, "SSH session closed")

	default:
		return fmt.Errorf("unsupported terminal event %q", message.Event)
	}
}

func (c *webTerminal) beginConnect(request connectRequest) error {
	hostConfig, authMethods, err := c.server.prepareSSH(request)
	if err != nil {
		return err
	}
	request.Password = ""
	request.Passphrase = ""

	c.stateMu.Lock()
	if c.closed {
		c.stateMu.Unlock()
		return fmt.Errorf("terminal connection is closed")
	}
	c.attempt++
	attempt := c.attempt
	previousCancel := c.connectCancel
	previousSession := c.session
	connectContext, cancel := context.WithTimeout(c.ctx, hostConfig.Timeout)
	c.connectCancel = cancel
	c.session = nil
	c.stateMu.Unlock()

	if previousCancel != nil {
		previousCancel()
	}
	if previousSession != nil {
		previousSession.close()
	}

	go c.connect(attempt, connectContext, cancel, request, hostConfig, authMethods)
	return nil
}

func (c *webTerminal) connect(
	attempt uint64,
	ctx context.Context,
	cancel context.CancelFunc,
	request connectRequest,
	hostConfig *sshc.Host,
	authMethods []ssh.AuthMethod,
) {
	defer cancel()
	session, stdout, err := openSSH(ctx, hostConfig, authMethods)
	if err != nil {
		c.finishConnectError(attempt, ctx, err)
		return
	}
	if !c.installSession(attempt, session) {
		session.close()
		return
	}

	if err := c.send(eventSSHConnected, map[string]any{
		"host": request.Host,
		"port": request.Port,
		"user": request.Username,
	}); err != nil {
		c.finishSession(attempt, session)
		c.shutdown()
		return
	}
	go c.pipeOutput(attempt, stdout, session)
}

func (c *webTerminal) finishConnectError(attempt uint64, ctx context.Context, connectErr error) {
	c.stateMu.Lock()
	current := !c.closed && c.attempt == attempt
	if current {
		c.connectCancel = nil
	}
	c.stateMu.Unlock()
	if !current || errors.Is(ctx.Err(), context.Canceled) {
		return
	}
	message := connectErr.Error()
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		message = "SSH connection timed out"
	}
	if err := c.send(eventSSHError, message); err != nil {
		c.shutdown()
	}
}

func (c *webTerminal) installSession(attempt uint64, session *sshSession) bool {
	c.stateMu.Lock()
	defer c.stateMu.Unlock()
	if c.closed || c.attempt != attempt {
		return false
	}
	c.connectCancel = nil
	c.session = session
	return true
}

func (c *webTerminal) currentSession() *sshSession {
	c.stateMu.Lock()
	defer c.stateMu.Unlock()
	return c.session
}

func (c *webTerminal) finishSession(attempt uint64, session *sshSession) bool {
	c.stateMu.Lock()
	if c.closed || c.attempt != attempt || c.session != session {
		c.stateMu.Unlock()
		return false
	}
	c.session = nil
	c.stateMu.Unlock()
	session.close()
	return true
}

func (c *webTerminal) disconnectSession() {
	c.stateMu.Lock()
	c.attempt++
	cancel := c.connectCancel
	session := c.session
	c.connectCancel = nil
	c.session = nil
	c.stateMu.Unlock()
	if cancel != nil {
		cancel()
	}
	if session != nil {
		session.close()
	}
}

func (c *webTerminal) pipeOutput(attempt uint64, stdout io.Reader, session *sshSession) {
	buffer := make([]byte, 32*1024)
	for {
		count, err := stdout.Read(buffer)
		if count > 0 {
			if sendErr := c.send(eventTerminalOutput, string(buffer[:count])); sendErr != nil {
				c.finishSession(attempt, session)
				c.shutdown()
				return
			}
		}
		if err != nil {
			if !c.finishSession(attempt, session) {
				return
			}
			if errors.Is(err, io.EOF) {
				err = c.send(eventSSHDisconnected, "SSH session closed")
			} else {
				err = c.send(eventSSHError, "SSH session closed: "+err.Error())
			}
			if err != nil {
				c.shutdown()
			}
			return
		}
	}
}

func (c *webTerminal) send(event string, data any) error {
	if c.ctx.Err() != nil {
		return c.ctx.Err()
	}
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	if err := c.conn.SetWriteDeadline(time.Now().Add(websocketWriteWait)); err != nil {
		return err
	}
	return c.conn.WriteJSON(websocketOutbound{Event: event, Data: data})
}

func (c *webTerminal) keepAlive() {
	ticker := time.NewTicker(websocketPingPeriod)
	defer ticker.Stop()
	for {
		select {
		case <-c.ctx.Done():
			return
		case <-ticker.C:
			c.writeMu.Lock()
			_ = c.conn.SetWriteDeadline(time.Now().Add(websocketWriteWait))
			err := c.conn.WriteMessage(websocket.PingMessage, nil)
			c.writeMu.Unlock()
			if err != nil {
				c.shutdown()
				return
			}
		}
	}
}

func (c *webTerminal) shutdown() {
	c.closeOnce.Do(func() {
		c.cancel()

		c.stateMu.Lock()
		c.closed = true
		c.attempt++
		cancel := c.connectCancel
		session := c.session
		c.connectCancel = nil
		c.session = nil
		c.stateMu.Unlock()
		if cancel != nil {
			cancel()
		}
		if session != nil {
			session.close()
		}

		c.writeMu.Lock()
		_ = c.conn.WriteControl(
			websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""),
			time.Now().Add(websocketWriteWait),
		)
		_ = c.conn.Close()
		c.writeMu.Unlock()
	})
}
