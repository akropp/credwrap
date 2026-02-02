// Package protocol defines the wire protocol for credwrap client-server communication.
package protocol

// Request types
const (
	TypeExec       = "exec"
	TypeStdin      = "stdin"
	TypeStdinClose = "stdin_close"
	TypePing       = "ping"
)

// Response types
const (
	TypeStarted = "started"
	TypeStdout  = "stdout"
	TypeStderr  = "stderr"
	TypeExit    = "exit"
	TypeError   = "error"
	TypePong    = "pong"
)

// ExecRequest is sent by client to execute a tool.
type ExecRequest struct {
	Type  string            `json:"type"`
	Token string            `json:"token"`
	Tool  string            `json:"tool"`
	Args  []string          `json:"args,omitempty"`
	Env   map[string]string `json:"env,omitempty"`
}

// StdinData is sent by client to write to the process stdin.
type StdinData struct {
	Type string `json:"type"`
	Data string `json:"data,omitempty"`
}

// StartedResponse indicates the process has started.
type StartedResponse struct {
	Type string `json:"type"`
	PID  int    `json:"pid"`
}

// OutputResponse carries stdout or stderr data.
type OutputResponse struct {
	Type string `json:"type"`
	Data string `json:"data"`
}

// ExitResponse indicates the process has exited.
type ExitResponse struct {
	Type string `json:"type"`
	Code int    `json:"code"`
}

// ErrorResponse indicates an error occurred.
type ErrorResponse struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}

// PingRequest is a health check.
type PingRequest struct {
	Type string `json:"type"`
}

// PongResponse is the health check response.
type PongResponse struct {
	Type    string `json:"type"`
	Version string `json:"version"`
}
