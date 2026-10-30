//go:build !windows

// Package serialptt contains the non-Windows serial PTT fallback.
package serialptt

import "errors"

type Controller struct{}

func New() *Controller { return &Controller{} }

func (c *Controller) Set(port, mode string, enabled, activeHigh bool) error {
	return errors.New("RTS/DTR serial PTT is currently implemented for Windows only")
}

func (c *Controller) Close() error { return nil }
