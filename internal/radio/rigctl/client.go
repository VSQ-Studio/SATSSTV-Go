// Package rigctl controls Hamlib rigctld through its TCP text protocol.
package rigctl

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Client contains connection settings for a running rigctld daemon.
type Client struct {
	Address string
	Timeout time.Duration
}

func New(address string) *Client {
	return &Client{Address: strings.TrimSpace(address), Timeout: 2 * time.Second}
}

// Session keeps a TCP connection open for the full SSTV transmission. A
// number of rigctld backends release PTT when the control socket is closed.
type Session struct {
	mu      sync.Mutex
	conn    net.Conn
	reader  *bufio.Reader
	timeout time.Duration
	closed  bool
}

// StartPTT connects to rigctld and asserts PTT on that connection.
func (c *Client) StartPTT(ctx context.Context, value int) (*Session, error) {
	if value < 1 || value > 3 {
		return nil, fmt.Errorf("Hamlib PTT value must be 1..3, got %d", value)
	}
	if c == nil || strings.TrimSpace(c.Address) == "" {
		return nil, errors.New("rigctld address is empty")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	timeout := c.timeout()
	conn, err := (&net.Dialer{Timeout: timeout}).DialContext(ctx, "tcp", c.Address)
	if err != nil {
		return nil, fmt.Errorf("connect to rigctld %s: %w", c.Address, err)
	}
	s := &Session{conn: conn, reader: bufio.NewReader(conn), timeout: timeout}
	if _, err := s.command(ctx, fmt.Sprintf("T %d\n", value)); err != nil {
		_ = s.Close()
		return nil, fmt.Errorf("assert Hamlib PTT: %w", err)
	}
	return s, nil
}

// SetPTT changes PTT while retaining the same TCP connection.
func (s *Session) SetPTT(ctx context.Context, enabled bool) error {
	if s == nil {
		return errors.New("nil rigctld session")
	}
	value := 0
	if enabled {
		value = 1
	}
	_, err := s.command(ctx, fmt.Sprintf("T %d\n", value))
	return err
}

func (s *Session) command(ctx context.Context, command string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || s.conn == nil {
		return "", errors.New("rigctld session is closed")
	}
	deadline := time.Now().Add(s.timeout)
	if ctx != nil {
		if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
			deadline = d
		}
	}
	if err := s.conn.SetDeadline(deadline); err != nil {
		return "", fmt.Errorf("set rigctld deadline: %w", err)
	}
	if _, err := s.conn.Write([]byte(command)); err != nil {
		_ = s.closeLocked()
		return "", fmt.Errorf("write rigctld command: %w", err)
	}
	line, err := s.reader.ReadString('\n')
	if err != nil {
		_ = s.closeLocked()
		return "", fmt.Errorf("read rigctld response: %w", err)
	}
	line = strings.TrimSpace(line)
	if strings.HasPrefix(line, "RPRT ") {
		code, parseErr := strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(line, "RPRT ")))
		if parseErr != nil {
			return line, fmt.Errorf("invalid rigctld response %q", line)
		}
		if code != 0 {
			return line, fmt.Errorf("rigctld returned error code %d", code)
		}
	}
	return line, nil
}

// Close closes the control socket. Callers should issue T 0 first.
func (s *Session) Close() error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.closeLocked()
}

func (s *Session) closeLocked() error {
	if s.closed {
		return nil
	}
	s.closed = true
	if s.conn == nil {
		return nil
	}
	err := s.conn.Close()
	s.conn = nil
	return err
}

func (c *Client) timeout() time.Duration {
	if c.Timeout <= 0 {
		return 2 * time.Second
	}
	return c.Timeout
}
