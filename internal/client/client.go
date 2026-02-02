// Package client implements the credwrap client.
package client

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"

	"github.com/openclaw/credwrap/internal/protocol"
)

// Client is the credwrap client.
type Client struct {
	addr  string
	token string
	conn  net.Conn
}

// ClientConfig holds client configuration.
type ClientConfig struct {
	Server string `yaml:"server"` // e.g., "127.0.0.1:9876"
	Token  string `yaml:"token"`
}

// New creates a new client.
func New(addr, token string) *Client {
	return &Client{
		addr:  addr,
		token: token,
	}
}

// Connect establishes connection to the server.
func (c *Client) Connect() error {
	conn, err := net.Dial("tcp", c.addr)
	if err != nil {
		return fmt.Errorf("connecting to %s: %w", c.addr, err)
	}
	c.conn = conn
	return nil
}

// Close closes the connection.
func (c *Client) Close() error {
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}

// Ping checks if the server is alive.
func (c *Client) Ping() (string, error) {
	encoder := json.NewEncoder(c.conn)
	encoder.Encode(protocol.PingRequest{Type: protocol.TypePing})

	reader := bufio.NewReader(c.conn)
	line, err := reader.ReadBytes('\n')
	if err != nil {
		return "", err
	}

	var resp protocol.PongResponse
	if err := json.Unmarshal(line, &resp); err != nil {
		return "", err
	}

	return resp.Version, nil
}

// Exec executes a tool and streams output to stdout/stderr.
func (c *Client) Exec(tool string, args []string) (int, error) {
	encoder := json.NewEncoder(c.conn)
	reader := bufio.NewReader(c.conn)

	// Send exec request
	req := protocol.ExecRequest{
		Type:  protocol.TypeExec,
		Token: c.token,
		Tool:  tool,
		Args:  args,
	}
	if err := encoder.Encode(req); err != nil {
		return -1, fmt.Errorf("sending request: %w", err)
	}

	// Read responses
	for {
		line, err := reader.ReadBytes('\n')
		if err != nil {
			if err == io.EOF {
				return -1, fmt.Errorf("connection closed unexpectedly")
			}
			return -1, fmt.Errorf("reading response: %w", err)
		}

		// Parse message type
		var msg struct {
			Type    string `json:"type"`
			Data    string `json:"data"`
			Code    int    `json:"code"`
			Message string `json:"message"`
			PID     int    `json:"pid"`
		}
		if err := json.Unmarshal(line, &msg); err != nil {
			return -1, fmt.Errorf("parsing response: %w", err)
		}

		switch msg.Type {
		case protocol.TypeStarted:
			// Process started, continue reading

		case protocol.TypeStdout:
			fmt.Fprintln(os.Stdout, msg.Data)

		case protocol.TypeStderr:
			fmt.Fprintln(os.Stderr, msg.Data)

		case protocol.TypeExit:
			return msg.Code, nil

		case protocol.TypeError:
			return -1, fmt.Errorf("server error: %s", msg.Message)

		default:
			// Unknown message type, ignore
		}
	}
}

// ExecInteractive executes a tool with stdin forwarding.
func (c *Client) ExecInteractive(tool string, args []string) (int, error) {
	encoder := json.NewEncoder(c.conn)
	reader := bufio.NewReader(c.conn)

	// Send exec request
	req := protocol.ExecRequest{
		Type:  protocol.TypeExec,
		Token: c.token,
		Tool:  tool,
		Args:  args,
	}
	if err := encoder.Encode(req); err != nil {
		return -1, fmt.Errorf("sending request: %w", err)
	}

	// Forward stdin in a goroutine
	stdinDone := make(chan struct{})
	go func() {
		defer close(stdinDone)
		stdinReader := bufio.NewReader(os.Stdin)
		for {
			line, err := stdinReader.ReadString('\n')
			if err != nil {
				encoder.Encode(protocol.StdinData{Type: protocol.TypeStdinClose})
				return
			}
			encoder.Encode(protocol.StdinData{Type: protocol.TypeStdin, Data: line})
		}
	}()

	// Read responses
	for {
		line, err := reader.ReadBytes('\n')
		if err != nil {
			if err == io.EOF {
				return -1, fmt.Errorf("connection closed unexpectedly")
			}
			return -1, fmt.Errorf("reading response: %w", err)
		}

		var msg struct {
			Type    string `json:"type"`
			Data    string `json:"data"`
			Code    int    `json:"code"`
			Message string `json:"message"`
		}
		if err := json.Unmarshal(line, &msg); err != nil {
			return -1, fmt.Errorf("parsing response: %w", err)
		}

		switch msg.Type {
		case protocol.TypeStarted:
			// Continue

		case protocol.TypeStdout:
			fmt.Fprintln(os.Stdout, msg.Data)

		case protocol.TypeStderr:
			fmt.Fprintln(os.Stderr, msg.Data)

		case protocol.TypeExit:
			return msg.Code, nil

		case protocol.TypeError:
			return -1, fmt.Errorf("server error: %s", msg.Message)
		}
	}
}
