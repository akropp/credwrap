// Package server implements the credwrap server.
package server

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/exec"
	"sync"
	"time"

	"github.com/openclaw/credwrap/internal/config"
	"github.com/openclaw/credwrap/internal/protocol"
)

// Server is the credwrap server.
type Server struct {
	cfg       *config.Config
	listener  net.Listener
	auditFile *os.File
	auditMu   sync.Mutex
}

// New creates a new server with the given configuration.
func New(cfg *config.Config) *Server {
	return &Server{cfg: cfg}
}

// Start starts the server.
func (s *Server) Start() error {
	// Open audit log if configured
	if s.cfg.Server.Audit != "" {
		f, err := os.OpenFile(s.cfg.Server.Audit, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
		if err != nil {
			return fmt.Errorf("opening audit log: %w", err)
		}
		s.auditFile = f
	}

	listener, err := net.Listen("tcp", s.cfg.Server.Listen)
	if err != nil {
		return fmt.Errorf("listening on %s: %w", s.cfg.Server.Listen, err)
	}
	s.listener = listener

	log.Printf("credwrap-server listening on %s", s.cfg.Server.Listen)
	log.Printf("Loaded %d tools, %d credentials", len(s.cfg.Tools), len(s.cfg.Credentials))

	for {
		conn, err := listener.Accept()
		if err != nil {
			log.Printf("accept error: %v", err)
			continue
		}
		go s.handleConnection(conn)
	}
}

// Stop stops the server.
func (s *Server) Stop() error {
	if s.listener != nil {
		s.listener.Close()
	}
	if s.auditFile != nil {
		s.auditFile.Close()
	}
	return nil
}

func (s *Server) handleConnection(conn net.Conn) {
	defer conn.Close()

	remoteAddr := conn.RemoteAddr().String()
	reader := bufio.NewReader(conn)
	encoder := json.NewEncoder(conn)

	for {
		line, err := reader.ReadBytes('\n')
		if err != nil {
			if err != io.EOF {
				log.Printf("[%s] read error: %v", remoteAddr, err)
			}
			return
		}

		// Parse the message type first
		var msg struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal(line, &msg); err != nil {
			s.sendError(encoder, "invalid JSON")
			continue
		}

		switch msg.Type {
		case protocol.TypePing:
			encoder.Encode(protocol.PongResponse{
				Type:    protocol.TypePong,
				Version: "0.1.0",
			})

		case protocol.TypeExec:
			var req protocol.ExecRequest
			if err := json.Unmarshal(line, &req); err != nil {
				s.sendError(encoder, "invalid exec request")
				continue
			}
			s.handleExec(conn, remoteAddr, &req, encoder, reader)

		default:
			s.sendError(encoder, fmt.Sprintf("unknown message type: %s", msg.Type))
		}
	}
}

func (s *Server) handleExec(conn net.Conn, remoteAddr string, req *protocol.ExecRequest, encoder *json.Encoder, reader *bufio.Reader) {
	startTime := time.Now()

	// Authenticate
	if !s.authenticate(req.Token, remoteAddr) {
		s.sendError(encoder, "authentication failed")
		s.audit(remoteAddr, req.Tool, req.Args, -1, time.Since(startTime), "auth_failed")
		return
	}

	// Look up tool
	tool, ok := s.cfg.Tools[req.Tool]
	if !ok {
		s.sendError(encoder, fmt.Sprintf("unknown tool: %s", req.Tool))
		s.audit(remoteAddr, req.Tool, req.Args, -1, time.Since(startTime), "unknown_tool")
		return
	}

	// Validate args
	if err := tool.ValidateArgs(req.Args); err != nil {
		s.sendError(encoder, err.Error())
		s.audit(remoteAddr, req.Tool, req.Args, -1, time.Since(startTime), "invalid_args")
		return
	}

	// Build environment with credentials
	env := os.Environ()
	for _, cred := range tool.Credentials {
		if cred.Env != "" {
			value, ok := s.cfg.Credentials[cred.Secret]
			if !ok {
				s.sendError(encoder, fmt.Sprintf("credential not found: %s", cred.Secret))
				return
			}
			env = append(env, fmt.Sprintf("%s=%s", cred.Env, value))
		}
	}

	// Add any extra env from request
	for k, v := range req.Env {
		env = append(env, fmt.Sprintf("%s=%s", k, v))
	}

	// Create command
	cmd := exec.Command(tool.Path, req.Args...)
	cmd.Env = env

	// Set up pipes
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		s.sendError(encoder, fmt.Sprintf("stdout pipe: %v", err))
		return
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		s.sendError(encoder, fmt.Sprintf("stderr pipe: %v", err))
		return
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		s.sendError(encoder, fmt.Sprintf("stdin pipe: %v", err))
		return
	}

	// Start the command
	if err := cmd.Start(); err != nil {
		s.sendError(encoder, fmt.Sprintf("start: %v", err))
		s.audit(remoteAddr, req.Tool, req.Args, -1, time.Since(startTime), "start_failed")
		return
	}

	// Send started response
	encoder.Encode(protocol.StartedResponse{
		Type: protocol.TypeStarted,
		PID:  cmd.Process.Pid,
	})

	// Stream stdout/stderr in goroutines
	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		s.streamOutput(encoder, stdout, protocol.TypeStdout)
	}()

	go func() {
		defer wg.Done()
		s.streamOutput(encoder, stderr, protocol.TypeStderr)
	}()

	// Handle stdin from client in a goroutine
	go func() {
		defer stdin.Close()
		for {
			line, err := reader.ReadBytes('\n')
			if err != nil {
				return
			}
			var msg struct {
				Type string `json:"type"`
				Data string `json:"data"`
			}
			if err := json.Unmarshal(line, &msg); err != nil {
				continue
			}
			switch msg.Type {
			case protocol.TypeStdin:
				stdin.Write([]byte(msg.Data))
			case protocol.TypeStdinClose:
				return
			}
		}
	}()

	// Wait for output to finish
	wg.Wait()

	// Wait for command to exit
	exitCode := 0
	if err := cmd.Wait(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			exitCode = -1
		}
	}

	encoder.Encode(protocol.ExitResponse{
		Type: protocol.TypeExit,
		Code: exitCode,
	})

	s.audit(remoteAddr, req.Tool, req.Args, exitCode, time.Since(startTime), "ok")
}

func (s *Server) streamOutput(encoder *json.Encoder, r io.Reader, outputType string) {
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		encoder.Encode(protocol.OutputResponse{
			Type: outputType,
			Data: scanner.Text(),
		})
	}
}

func (s *Server) authenticate(token string, remoteAddr string) bool {
	// Check token
	for _, t := range s.cfg.Auth.Tokens {
		if token == t {
			return true
		}
	}
	// TODO: Check Tailscale node identity
	return false
}

func (s *Server) sendError(encoder *json.Encoder, msg string) {
	encoder.Encode(protocol.ErrorResponse{
		Type:    protocol.TypeError,
		Message: msg,
	})
}

func (s *Server) audit(remoteAddr, tool string, args []string, exitCode int, duration time.Duration, status string) {
	if s.auditFile == nil {
		return
	}

	entry := map[string]interface{}{
		"ts":          time.Now().UTC().Format(time.RFC3339),
		"client":      remoteAddr,
		"tool":        tool,
		"args":        args,
		"exit_code":   exitCode,
		"duration_ms": duration.Milliseconds(),
		"status":      status,
	}

	s.auditMu.Lock()
	defer s.auditMu.Unlock()

	data, _ := json.Marshal(entry)
	s.auditFile.Write(data)
	s.auditFile.Write([]byte("\n"))
}
