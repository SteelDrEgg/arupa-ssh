package main

// connectRequest is the browser payload for the connect_ssh transport event.
type connectRequest struct {
	Host       string `json:"host"`
	Port       string `json:"port"`
	Username   string `json:"username"`
	Password   string `json:"password"`
	PrivateKey string `json:"privateKey"`
	Passphrase string `json:"passphrase"`
}

// resizeRequest is the browser payload for PTY resize events.
type resizeRequest struct {
	Cols int `json:"cols"`
	Rows int `json:"rows"`
}
