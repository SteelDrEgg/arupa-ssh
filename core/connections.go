package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"

	arupa "github.com/SteelDrEgg/arupa-sdk/golang"
)

const (
	savedConnectionsPath = "/ssh/api/connections"

	sshConfigPathParam    = "ssh_config_path"
	connectionParamPrefix = "connection."
	connectionHostField   = "host"
	connectionPortField   = "port"
	connectionUserField   = "username"
	connectionAuthField   = "auth"

	authTypePassword     = "password"
	authTypeKey          = "key"
	legacyAuthTypeSecret = "secret"
)

var connectionParamFields = []string{
	connectionHostField,
	connectionPortField,
	connectionUserField,
	connectionAuthField,
}

// savedConnection is the non-sensitive portion of a persisted SSH profile.
// Passwords, secret values, and passphrases deliberately have no field here.
type savedConnection struct {
	Name       string `json:"name"`
	Host       string `json:"host"`
	Port       string `json:"port"`
	Username   string `json:"username"`
	AuthType   string `json:"auth_type"`
	PrivateKey string `json:"private_key,omitempty"`
	SecretName string `json:"secret_name,omitempty"`
}

type savedConnectionsResponse struct {
	Success     bool              `json:"success"`
	Connections []savedConnection `json:"connections"`
}

type savedConnectionUpdateRequest struct {
	OriginalName string          `json:"original_name"`
	Connection   savedConnection `json:"connection"`
}

type savedConnectionDeleteRequest struct {
	Name string `json:"name"`
}

// sshSettings is the application configuration read from the Params snapshot.
type sshSettings struct {
	SSHConfigPath string
	Connections   map[string]savedConnection
}

// sshSettingsStore is the sole owner of the SSH plugin Params schema. A
// profile named "host1" is stored as readable entries such as
// connection.host1.host and connection.host1.auth.
type sshSettingsStore struct {
	params arupa.ParamsClient
}

// Load converts the initial plugin Params snapshot to validated SSH settings.
// Unrelated Params are deliberately ignored.
func (store sshSettingsStore) Load(params map[string]string) (sshSettings, error) {
	fields := make(map[string]map[string]string)
	for key, value := range params {
		name, field, isConnection, err := parseConnectionParamKey(key)
		if err != nil {
			return sshSettings{}, err
		}
		if !isConnection {
			continue
		}
		if fields[name] == nil {
			fields[name] = make(map[string]string)
		}
		fields[name][field] = value
	}

	connections := make(map[string]savedConnection, len(fields))
	for name, values := range fields {
		connection, err := connectionFromParams(name, values)
		if err != nil {
			return sshSettings{}, err
		}
		connections[connection.Name] = connection
	}
	return sshSettings{
		SSHConfigPath: params[sshConfigPathParam],
		Connections:   connections,
	}, nil
}

// Save persists one connection as four human-readable Params entries.
func (store sshSettingsStore) Save(ctx context.Context, connection savedConnection) error {
	if store.params == nil {
		return fmt.Errorf("persist SSH connection: plugin parameters are unavailable")
	}
	values, err := connectionParamValues(connection)
	if err != nil {
		return err
	}
	if err := store.params.PatchParams(ctx, arupa.ParamsPatch{Set: values}); err != nil {
		return fmt.Errorf("persist SSH connection: %w", err)
	}
	return nil
}

// Update replaces a saved connection. Renaming removes the old four Params in
// the same patch that writes the replacement, so no duplicate profile remains.
func (store sshSettingsStore) Update(ctx context.Context, originalName string, connection savedConnection) error {
	if store.params == nil {
		return fmt.Errorf("update SSH connection: plugin parameters are unavailable")
	}
	values, err := connectionParamValues(connection)
	if err != nil {
		return err
	}
	patch := arupa.ParamsPatch{Set: values}
	if originalName != connection.Name {
		patch.Delete = connectionParamKeys(originalName)
	}
	if err := store.params.PatchParams(ctx, patch); err != nil {
		return fmt.Errorf("update SSH connection: %w", err)
	}
	return nil
}

// Delete removes every persisted field belonging to a saved connection.
func (store sshSettingsStore) Delete(ctx context.Context, name string) error {
	if store.params == nil {
		return fmt.Errorf("delete SSH connection: plugin parameters are unavailable")
	}
	if err := store.params.PatchParams(ctx, arupa.ParamsPatch{Delete: connectionParamKeys(name)}); err != nil {
		return fmt.Errorf("delete SSH connection: %w", err)
	}
	return nil
}

func parseConnectionParamKey(key string) (name, field string, isConnection bool, err error) {
	if !strings.HasPrefix(key, connectionParamPrefix) {
		return "", "", false, nil
	}
	for _, candidate := range connectionParamFields {
		suffix := "." + candidate
		if strings.HasSuffix(key, suffix) {
			name = strings.TrimSuffix(strings.TrimPrefix(key, connectionParamPrefix), suffix)
			if strings.TrimSpace(name) == "" {
				return "", "", true, fmt.Errorf("parse %q: connection name is required", key)
			}
			return name, candidate, true, nil
		}
	}
	return "", "", true, fmt.Errorf("parse %q: unsupported connection field", key)
}

func connectionFromParams(name string, values map[string]string) (savedConnection, error) {
	authType, reference, err := parseConnectionAuth(values[connectionAuthField])
	if err != nil {
		return savedConnection{}, fmt.Errorf("parse connection %q: %w", name, err)
	}
	connection := savedConnection{
		Name:     name,
		Host:     values[connectionHostField],
		Port:     values[connectionPortField],
		Username: values[connectionUserField],
		AuthType: authType,
	}
	if authType == authTypeKey {
		connection.PrivateKey = reference
	} else {
		connection.SecretName = reference
	}
	normalized, err := normalizeSavedConnection(connection)
	if err != nil {
		return savedConnection{}, fmt.Errorf("parse connection %q: %w", name, err)
	}
	return normalized, nil
}

func connectionParamValues(connection savedConnection) (map[string]string, error) {
	normalized, err := normalizeSavedConnection(connection)
	if err != nil {
		return nil, err
	}
	prefix := connectionParamPrefix + normalized.Name + "."
	return map[string]string{
		prefix + connectionHostField: normalized.Host,
		prefix + connectionPortField: normalized.Port,
		prefix + connectionUserField: normalized.Username,
		prefix + connectionAuthField: connectionAuth(normalized),
	}, nil
}

func connectionParamKeys(name string) []string {
	prefix := connectionParamPrefix + name + "."
	return []string{
		prefix + connectionHostField,
		prefix + connectionPortField,
		prefix + connectionUserField,
		prefix + connectionAuthField,
	}
}

// connectionAuth is intentionally readable in a Params file. Its second
// value is a secret reference for password auth or a private-key path for key
// auth; it is never a credential value.
func connectionAuth(connection savedConnection) string {
	reference := connection.SecretName
	if connection.AuthType == authTypeKey {
		reference = connection.PrivateKey
	}
	if reference == "" {
		return "{" + connection.AuthType + "}"
	}
	return "{" + connection.AuthType + ", " + reference + "}"
}

func parseConnectionAuth(value string) (authType, reference string, err error) {
	value = strings.TrimSpace(value)
	if len(value) < 3 || value[0] != '{' || value[len(value)-1] != '}' {
		return "", "", fmt.Errorf("auth must use {type} or {type, reference} syntax")
	}
	parts := strings.SplitN(value[1:len(value)-1], ",", 2)
	authType = strings.TrimSpace(parts[0])
	if len(parts) == 2 {
		reference = strings.TrimSpace(parts[1])
	}
	if authType == "" {
		return "", "", fmt.Errorf("auth type is required")
	}
	return authType, reference, nil
}

func (s *sshServer) handleConnectionsHTTP(w http.ResponseWriter, req *http.Request) {
	if strings.TrimRight(req.URL.Path, "/") != savedConnectionsPath {
		writeConnectionJSON(w, http.StatusNotFound, map[string]any{
			"success": false,
			"message": "Not found",
		})
		return
	}

	switch req.Method {
	case http.MethodGet:
		writeConnectionJSON(w, http.StatusOK, savedConnectionsResponse{
			Success:     true,
			Connections: s.savedConnections(),
		})
	case http.MethodPost:
		s.saveConnectionResponse(w, req)
	case http.MethodPut:
		s.updateConnectionResponse(w, req)
	case http.MethodDelete:
		s.deleteConnectionResponse(w, req)
	default:
		w.Header().Set("Allow", http.MethodGet+", "+http.MethodPost+", "+http.MethodPut+", "+http.MethodDelete)
		writeConnectionJSON(w, http.StatusMethodNotAllowed, map[string]any{
			"success": false,
			"message": "Method not allowed",
		})
	}
}

func (s *sshServer) savedConnections() []savedConnection {
	s.settingsMu.RLock()
	connections := make([]savedConnection, 0, len(s.settings))
	for _, connection := range s.settings {
		connections = append(connections, connection)
	}
	s.settingsMu.RUnlock()

	sort.Slice(connections, func(i, j int) bool {
		return strings.ToLower(connections[i].Name) < strings.ToLower(connections[j].Name)
	})
	return connections
}

func (s *sshServer) saveConnectionResponse(w http.ResponseWriter, req *http.Request) {
	body, err := io.ReadAll(req.Body)
	if err != nil {
		writeConnectionJSON(w, http.StatusBadRequest, map[string]any{
			"success": false,
			"message": "Invalid request body",
		})
		return
	}

	var connection savedConnection
	if err := json.Unmarshal(body, &connection); err != nil {
		writeConnectionJSON(w, http.StatusBadRequest, map[string]any{
			"success": false,
			"message": "Invalid JSON body",
		})
		return
	}

	normalized, err := normalizeSavedConnection(connection)
	if err != nil {
		writeConnectionJSON(w, http.StatusBadRequest, map[string]any{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	s.settingsWriteMu.Lock()
	defer s.settingsWriteMu.Unlock()

	if err := s.settingsStore.Save(req.Context(), normalized); err != nil {
		writeConnectionJSON(w, http.StatusInternalServerError, map[string]any{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	s.settingsMu.Lock()
	s.settings[normalized.Name] = normalized
	s.settingsMu.Unlock()
	writeConnectionJSON(w, http.StatusOK, map[string]any{
		"success":    true,
		"connection": normalized,
	})
}

func (s *sshServer) updateConnectionResponse(w http.ResponseWriter, req *http.Request) {
	body, err := io.ReadAll(req.Body)
	if err != nil {
		writeConnectionJSON(w, http.StatusBadRequest, map[string]any{"success": false, "message": "Invalid request body"})
		return
	}

	var update savedConnectionUpdateRequest
	if err := json.Unmarshal(body, &update); err != nil {
		writeConnectionJSON(w, http.StatusBadRequest, map[string]any{"success": false, "message": "Invalid JSON body"})
		return
	}
	update.OriginalName = strings.TrimSpace(update.OriginalName)
	if update.OriginalName == "" {
		writeConnectionJSON(w, http.StatusBadRequest, map[string]any{"success": false, "message": "Original connection name is required"})
		return
	}
	normalized, err := normalizeSavedConnection(update.Connection)
	if err != nil {
		writeConnectionJSON(w, http.StatusBadRequest, map[string]any{"success": false, "message": err.Error()})
		return
	}

	s.settingsWriteMu.Lock()
	defer s.settingsWriteMu.Unlock()

	s.settingsMu.RLock()
	_, exists := s.settings[update.OriginalName]
	_, replacementExists := s.settings[normalized.Name]
	s.settingsMu.RUnlock()
	if !exists {
		writeConnectionJSON(w, http.StatusNotFound, map[string]any{"success": false, "message": "Saved connection not found"})
		return
	}
	if update.OriginalName != normalized.Name && replacementExists {
		writeConnectionJSON(w, http.StatusConflict, map[string]any{"success": false, "message": "A saved connection with this name already exists"})
		return
	}
	if err := s.settingsStore.Update(req.Context(), update.OriginalName, normalized); err != nil {
		writeConnectionJSON(w, http.StatusInternalServerError, map[string]any{"success": false, "message": err.Error()})
		return
	}

	s.settingsMu.Lock()
	delete(s.settings, update.OriginalName)
	s.settings[normalized.Name] = normalized
	s.settingsMu.Unlock()
	writeConnectionJSON(w, http.StatusOK, map[string]any{"success": true, "connection": normalized})
}

func (s *sshServer) deleteConnectionResponse(w http.ResponseWriter, req *http.Request) {
	body, err := io.ReadAll(req.Body)
	if err != nil {
		writeConnectionJSON(w, http.StatusBadRequest, map[string]any{"success": false, "message": "Invalid request body"})
		return
	}

	var deletion savedConnectionDeleteRequest
	if err := json.Unmarshal(body, &deletion); err != nil {
		writeConnectionJSON(w, http.StatusBadRequest, map[string]any{"success": false, "message": "Invalid JSON body"})
		return
	}
	deletion.Name = strings.TrimSpace(deletion.Name)
	if deletion.Name == "" {
		writeConnectionJSON(w, http.StatusBadRequest, map[string]any{"success": false, "message": "Connection name is required"})
		return
	}

	s.settingsWriteMu.Lock()
	defer s.settingsWriteMu.Unlock()

	s.settingsMu.RLock()
	_, exists := s.settings[deletion.Name]
	s.settingsMu.RUnlock()
	if !exists {
		writeConnectionJSON(w, http.StatusNotFound, map[string]any{"success": false, "message": "Saved connection not found"})
		return
	}
	if err := s.settingsStore.Delete(req.Context(), deletion.Name); err != nil {
		writeConnectionJSON(w, http.StatusInternalServerError, map[string]any{"success": false, "message": err.Error()})
		return
	}

	s.settingsMu.Lock()
	delete(s.settings, deletion.Name)
	s.settingsMu.Unlock()
	writeConnectionJSON(w, http.StatusOK, map[string]any{"success": true})
}

func normalizeSavedConnection(connection savedConnection) (savedConnection, error) {
	connection.Name = strings.TrimSpace(connection.Name)
	connection.Host = strings.TrimSpace(connection.Host)
	connection.Port = strings.TrimSpace(connection.Port)
	connection.Username = strings.TrimSpace(connection.Username)
	connection.AuthType = strings.TrimSpace(connection.AuthType)
	connection.PrivateKey = strings.TrimSpace(connection.PrivateKey)
	connection.SecretName = strings.TrimSpace(connection.SecretName)

	if connection.Name == "" || len(connection.Name) > 80 {
		return savedConnection{}, fmt.Errorf("connection name is required and must not exceed 80 characters")
	}
	if connection.Host == "" || connection.Username == "" {
		return savedConnection{}, fmt.Errorf("host and username are required")
	}
	if connection.Port == "" {
		connection.Port = "22"
	}
	port, err := strconv.Atoi(connection.Port)
	if err != nil || port < 1 || port > 65535 {
		return savedConnection{}, fmt.Errorf("port must be between 1 and 65535")
	}

	switch connection.AuthType {
	case authTypePassword:
		connection.PrivateKey = ""
	case authTypeKey:
		connection.SecretName = ""
	case legacyAuthTypeSecret:
		connection.AuthType = authTypePassword
		connection.PrivateKey = ""
		if connection.SecretName == "" {
			return savedConnection{}, fmt.Errorf("secret name is required for a Secret Manager password")
		}
	default:
		return savedConnection{}, fmt.Errorf("unsupported authentication type %q", connection.AuthType)
	}
	return connection, nil
}

func writeConnectionJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		return
	}
}
