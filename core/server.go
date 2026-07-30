package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	arupa "github.com/SteelDrEgg/arupa-sdk/golang"
	arupagrpc "github.com/SteelDrEgg/arupa-sdk/golang/grpc"
)

const (
	pluginDisplayName = "ssh"

	proxyListenerID  = "proxy"
	proxyTransportID = "ssh-proxy"
	proxyRouteID     = "ssh-proxy-route"
	proxyRoutePath   = "/ssh/"
	websocketPath    = "/ssh/ws"
	uiPath           = "/ssh/pages/"
	entryRouteID     = "entry"

	// socketNamespace is retained for the legacy Socket.IO adapter. The v2
	// runtime serves the terminal through websocketPath instead.
	socketNamespace = "/ssh"

	eventConnectSSH    = "connect_ssh"
	eventTerminalInput = "terminal_input"
	eventResize        = "resize"
	eventDisconnect    = "disconnect"

	eventSSHConnected    = "ssh_connected"
	eventSSHDisconnected = "ssh_disconnected"
	eventSSHError        = "ssh_error"
	eventTerminalOutput  = "terminal_output"
)

// sshServer owns the SSH core and the HTTP server exposed through the
// v2 inherited proxy listener. The legacy Socket.IO listener remains available
// as an application adapter, but no Socket.IO transport is registered.
type sshServer struct {
	sdk           *arupagrpc.Service
	httpServer    *http.Server
	contentRoot   string
	settingsStore sshSettingsStore
	events        *arupa.SocketListener
	mu            sync.RWMutex
	sshConfigPath string
	sessions      map[string]*sshSession
	pending       map[string]*pendingConnection

	settingsMu      sync.RWMutex
	settings        map[string]savedConnection
	settingsWriteMu sync.Mutex
}

// newSSHServer constructs an SSH application and its SDK gRPC adapter.
func newSSHServer() *sshServer {
	s := &sshServer{
		contentRoot: findContentRoot(),
		sessions:    make(map[string]*sshSession),
		pending:     make(map[string]*pendingConnection),
		settings:    make(map[string]savedConnection),
	}
	events := arupa.NewSocketListener()
	_ = events.On(eventConnectSSH, s.connectSSH)
	_ = events.On(eventTerminalInput, s.writeInput)
	_ = events.On(eventResize, s.resize)
	_ = events.On(eventDisconnect, func(_ context.Context, event arupa.SocketEvent, _ arupa.Emitter) error {
		s.cleanup(event.SocketID)
		return nil
	})

	s.sdk = &arupagrpc.Service{
		Info: arupa.ServiceInfo{
			Name:    pluginDisplayName,
			Version: pluginVersion,
		},
		Handler:    s.httpHandler(),
		Events:     events,
		OnRegister: s.configure,
	}
	s.httpServer = &http.Server{
		Handler:           s.sdk.Handler,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       90 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}
	s.settingsStore = sshSettingsStore{params: s.sdk}
	s.events = events
	return s
}

// configure initializes SSH application state after the SDK has connected the
// host callback and captured the initial Params snapshot.
func (s *sshServer) configure(ctx context.Context) error {
	if s.sdk == nil {
		return fmt.Errorf("ssh: SDK service is unavailable during registration")
	}
	if err := s.configureParams(s.sdk.InitialParams()); err != nil {
		return err
	}

	listener, ok := s.sdk.InheritedListener(proxyListenerID)
	if !ok {
		return fmt.Errorf("ssh: inherited listener %q is unavailable", proxyListenerID)
	}
	go s.serveHTTP(listener)

	result, err := s.sdk.RegisterTransport(ctx, arupa.Transport{
		ID:   proxyTransportID,
		Type: arupa.TransportProxy,
		Proxy: &arupa.ProxyTarget{
			Network: arupa.ProxyInherited,
			Address: proxyListenerID,
			Scheme:  "http",
		},
	})
	if err := requireRegistration("register inherited HTTP transport", result, err); err != nil {
		return err
	}

	result, err = s.sdk.RegisterRoutes(ctx, sshRoutes())
	if err := requireRegistration("register SSH HTTP routes", result, err); err != nil {
		return err
	}

	// Registration must not fail merely because host logging is unavailable.
	_ = s.sdk.LogInfo(ctx, "ssh inherited HTTP service registered")
	return nil
}

func sshRoutes() []arupa.Route {
	return []arupa.Route{
		{
			ID:          proxyRouteID,
			TransportID: proxyTransportID,
			HTTP: &arupa.HTTPRoute{
				Pattern: proxyRoutePath,
				Access:  arupa.AccessPolicy{RequireAuth: false},
			},
		},
		{
			ID:          entryRouteID,
			TransportID: proxyTransportID,
			HTTP: &arupa.HTTPRoute{
				Method:  http.MethodGet,
				Pattern: uiPath,
				Access:  arupa.AccessPolicy{RequireAuth: false},
			},
		},
	}
}

func (s *sshServer) serveHTTP(listener net.Listener) {
	err := s.httpServer.Serve(listener)
	if err == nil || errors.Is(err, http.ErrServerClosed) || errors.Is(err, net.ErrClosed) {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = s.sdk.LogError(ctx, "ssh inherited HTTP server stopped: "+err.Error())
}

func (s *sshServer) httpHandler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc(savedConnectionsPath, s.handleConnectionsHTTP)
	mux.HandleFunc(websocketPath, s.handleWebSocket)
	mux.Handle(uiPath, http.StripPrefix(
		uiPath,
		http.FileServer(http.Dir(filepath.Join(s.contentRoot, "ui", "build"))),
	))
	mux.HandleFunc(proxyRoutePath, func(w http.ResponseWriter, req *http.Request) {
		if req.URL.Path != proxyRoutePath {
			http.NotFound(w, req)
			return
		}
		if req.Method != http.MethodGet && req.Method != http.MethodHead {
			w.Header().Set("Allow", http.MethodGet+", "+http.MethodHead)
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		http.Redirect(w, req, uiPath+"index.html", http.StatusTemporaryRedirect)
	})
	return mux
}

func findContentRoot() string {
	if executable, err := os.Executable(); err == nil {
		root := filepath.Dir(executable)
		if _, err := os.Stat(filepath.Join(root, "ui", "build", "index.html")); err == nil {
			return root
		}
	}
	if workingDirectory, err := os.Getwd(); err == nil {
		if _, err := os.Stat(filepath.Join(workingDirectory, "ui", "build", "index.html")); err == nil {
			return workingDirectory
		}
	}
	return "."
}

func requireRegistration(operation string, result arupa.RegistrationResult, err error) error {
	if err != nil {
		return fmt.Errorf("ssh: %s: %w", operation, err)
	}
	if result.Successful() {
		return nil
	}
	details := strings.TrimSpace(result.Message)
	if details == "" {
		details = fmt.Sprintf("degraded=%t failures=%v", result.Degraded, result.Failures)
	}
	return fmt.Errorf("ssh: %s: %s", operation, details)
}

// configureParams applies the SSH-specific subset of the initial plugin
// parameters. Keeping it separate from the SDK hook makes the application
// configuration independently testable.
func (s *sshServer) configureParams(params map[string]string) error {
	settings, err := s.settingsStore.Load(params)
	if err != nil {
		return err
	}
	s.sshConfigPath = settings.SSHConfigPath
	s.settingsMu.Lock()
	s.settings = settings.Connections
	s.settingsMu.Unlock()
	return nil
}
