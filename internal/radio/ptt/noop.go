package ptt

import (
	"context"
	"fmt"
)

// NoopController represents VOX or external/manual keying. The software does
// not send a hardware PTT command; audio playback is responsible for the TX
// trigger when Mode is VOX.
type NoopController struct {
	Mode Mode
}

func NewNoopController(mode Mode) (*NoopController, error) {
	if mode != ModeVOX && mode != ModeManual {
		return nil, fmt.Errorf("noop PTT controller only supports VOX or manual mode, got %q", mode)
	}
	return &NoopController{Mode: mode}, nil
}

func (c *NoopController) SetPTT(_ context.Context, _ bool) error { return nil }
func (c *NoopController) Close() error                           { return nil }
