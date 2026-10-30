package rigctl

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

// PTTController adapts a rigctld Client to a long-lived PTT controller. It
// retains the Session between SetPTT(true) and SetPTT(false), so the radio sees
// both commands on the same TCP connection during a long SSTV image.
type PTTController struct {
	mu      sync.Mutex
	client  *Client
	session *Session
	closed  bool
}

func NewPTTController(client *Client) (*PTTController, error) {
	if client == nil {
		return nil, errors.New("nil rigctld client")
	}
	return &PTTController{client: client}, nil
}

func (c *PTTController) SetPTT(ctx context.Context, enabled bool) error {
	if c == nil {
		return errors.New("nil Hamlib PTT controller")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return errors.New("Hamlib PTT controller is closed")
	}
	if enabled {
		if c.session != nil {
			return nil // idempotent while the transmission is keyed
		}
		session, err := c.client.StartPTT(ctx, 1)
		if err != nil {
			return err
		}
		c.session = session
		return nil
	}
	if c.session == nil {
		return nil
	}
	if err := c.session.SetPTT(ctx, false); err != nil {
		return fmt.Errorf("send rigctld PTT-off: %w", err)
	}
	if err := c.session.Close(); err != nil {
		return fmt.Errorf("close rigctld session after PTT-off: %w", err)
	}
	c.session = nil
	return nil
}

// Close makes a best-effort attempt to release PTT before closing the socket.
func (c *PTTController) Close() error {
	if c == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return nil
	}
	c.closed = true
	if c.session == nil {
		return nil
	}
	releaseErr := c.session.SetPTT(ctx, false)
	closeErr := c.session.Close()
	c.session = nil
	if releaseErr != nil {
		return fmt.Errorf("release PTT during controller close: %w", releaseErr)
	}
	if closeErr != nil {
		return fmt.Errorf("close rigctld session: %w", closeErr)
	}
	return nil
}
