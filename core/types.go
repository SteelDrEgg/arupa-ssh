package main

// sshConnectInput is the transport-independent request consumed by the SSH
// application.
type sshConnectInput struct {
	TargetType string
	Host       string
	Port       string
	Username   string
	Auth       sshAuthenticationInput
}

// sshAuthenticationInput selects exactly one credential source.
type sshAuthenticationInput struct {
	Method           string
	Password         string
	SecretName       string
	SecretPassphrase string
	PrivateKey       string
	Passphrase       string
}

// resizeRequest is the browser payload for PTY resize events.
type resizeRequest struct {
	Cols int `json:"cols"`
	Rows int `json:"rows"`
}
