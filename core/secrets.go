package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

const (
	secretManagerService = "secret-manager"
	secretListPath       = "/ssh/api/secrets"
)

type secretManager interface {
	List(context.Context) ([]secretMetadata, error)
	Reveal(context.Context, string, string) (string, error)
}

type serviceJSONClient interface {
	SendServiceJSON(context.Context, string, string, any) (string, error)
}

type secretManagerClient struct {
	sender serviceJSONClient
}

type secretMetadata struct {
	Name           string   `json:"name"`
	Description    string   `json:"description"`
	AllowedPlugins []string `json:"allowed_plugins"`
	Protection     string   `json:"protection"`
	UpdatedAt      string   `json:"updated_at"`
}

type secretManagerReply struct {
	Success bool             `json:"success"`
	Message string           `json:"message"`
	Error   json.RawMessage  `json:"error"`
	Keys    []secretMetadata `json:"keys"`
	Name    string           `json:"name"`
	Value   *string          `json:"value"`
}

func (client secretManagerClient) List(ctx context.Context) ([]secretMetadata, error) {
	var reply secretManagerReply
	if err := client.request(ctx, "list", map[string]any{}, &reply); err != nil {
		return nil, err
	}
	if !reply.Success {
		return nil, secretManagerReplyError(reply)
	}
	if reply.Keys == nil {
		reply.Keys = []secretMetadata{}
	}
	return reply.Keys, nil
}

func (client secretManagerClient) Reveal(ctx context.Context, name, passphrase string) (string, error) {
	var reply secretManagerReply
	if err := client.request(ctx, "reveal", map[string]string{
		"name":       name,
		"passphrase": passphrase,
	}, &reply); err != nil {
		return "", err
	}
	if !reply.Success {
		return "", secretManagerReplyError(reply)
	}
	if reply.Name != "" && reply.Name != name {
		return "", fmt.Errorf("Secret Manager returned a different entry")
	}
	if reply.Value == nil {
		return "", fmt.Errorf("Secret Manager returned no value")
	}
	return *reply.Value, nil
}

func (client secretManagerClient) request(ctx context.Context, topic string, payload any, reply *secretManagerReply) error {
	if client.sender == nil {
		return fmt.Errorf("Secret Manager service messaging is unavailable")
	}
	message, err := client.sender.SendServiceJSON(ctx, secretManagerService, topic, payload)
	if err != nil {
		return fmt.Errorf("send %s request: %w", topic, err)
	}
	if err := json.Unmarshal([]byte(message), reply); err != nil {
		return fmt.Errorf("decode Secret Manager %s response: %w", topic, err)
	}
	return nil
}

func secretManagerReplyError(reply secretManagerReply) error {
	message := strings.TrimSpace(reply.Message)
	if message == "" {
		message = secretManagerErrorMessage(reply.Error)
	}
	if message == "" {
		message = "request failed"
	}
	return fmt.Errorf("Secret Manager: %s", message)
}

func secretManagerErrorMessage(raw json.RawMessage) string {
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	var message string
	if json.Unmarshal(raw, &message) == nil {
		return strings.TrimSpace(message)
	}
	var details struct {
		Message string `json:"message"`
		Detail  string `json:"detail"`
		Error   string `json:"error"`
	}
	if json.Unmarshal(raw, &details) == nil {
		for _, candidate := range []string{details.Message, details.Detail, details.Error} {
			if candidate = strings.TrimSpace(candidate); candidate != "" {
				return candidate
			}
		}
	}
	return ""
}

func (s *sshServer) handleSecretListHTTP(w http.ResponseWriter, req *http.Request) {
	if strings.TrimRight(req.URL.Path, "/") != secretListPath {
		writeConnectionJSON(w, http.StatusNotFound, map[string]any{
			"success": false,
			"message": "Not found",
		})
		return
	}
	if req.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		writeConnectionJSON(w, http.StatusMethodNotAllowed, map[string]any{
			"success": false,
			"message": "Method not allowed",
		})
		return
	}
	if s.app == nil || s.app.secrets == nil {
		writeConnectionJSON(w, http.StatusServiceUnavailable, map[string]any{
			"success": false,
			"message": "Secret Manager is unavailable",
		})
		return
	}
	keys, err := s.app.secrets.List(req.Context())
	if err != nil {
		writeConnectionJSON(w, http.StatusBadGateway, map[string]any{
			"success": false,
			"message": err.Error(),
		})
		return
	}
	writeConnectionJSON(w, http.StatusOK, map[string]any{
		"success": true,
		"keys":    keys,
	})
}
