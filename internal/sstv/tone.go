package sstv

import (
	"errors"
	"fmt"
	"math"
	"time"
)

// ToneGenerator builds mono signed 16-bit PCM while preserving oscillator
// phase across frequency changes. Fractional sample carry prevents timing
// drift when a pixel dwell does not contain an integer number of samples.
type ToneGenerator struct {
	SampleRate int
	Amplitude  float64

	phase      float64
	fractional float64
	samples    []int16
}

func NewToneGenerator(sampleRate int, amplitude float64) (*ToneGenerator, error) {
	if sampleRate < 8_000 || sampleRate > 192_000 {
		return nil, fmt.Errorf("sample rate %d is outside supported range 8000..192000 Hz", sampleRate)
	}
	if amplitude <= 0 || amplitude > 1 {
		return nil, errors.New("amplitude must be greater than 0 and at most 1")
	}
	return &ToneGenerator{
		SampleRate: sampleRate,
		Amplitude:  amplitude,
		samples:    make([]int16, 0, sampleRate*38),
	}, nil
}

// Tone appends a constant-frequency segment. Phase remains continuous at
// segment boundaries to avoid introducing an unnecessary discontinuity.
func (g *ToneGenerator) Tone(frequencyHz float64, duration time.Duration) error {
	return g.ToneMS(frequencyHz, float64(duration)/float64(time.Millisecond))
}

// ToneMS appends a segment with a fractional-millisecond duration. Keeping
// the mode timing as float64 until sample conversion avoids cumulative
// nanosecond rounding across hundreds of thousands of SSTV pixels.
func (g *ToneGenerator) ToneMS(frequencyHz, durationMS float64) error {
	if g == nil {
		return errors.New("nil tone generator")
	}
	if g.SampleRate < 8_000 || g.SampleRate > 192_000 {
		return fmt.Errorf("invalid generator sample rate %d", g.SampleRate)
	}
	if frequencyHz <= 0 || frequencyHz >= float64(g.SampleRate)/2 {
		return fmt.Errorf("tone frequency %.2f Hz must be between 0 and Nyquist %.2f Hz", frequencyHz, float64(g.SampleRate)/2)
	}
	if math.IsNaN(durationMS) || math.IsInf(durationMS, 0) || durationMS < 0 {
		return errors.New("tone duration must be finite and non-negative")
	}
	if durationMS == 0 {
		return nil
	}

	exact := durationMS/1000*float64(g.SampleRate) + g.fractional
	count := int(math.Round(exact))
	g.fractional = exact - float64(count)
	if count <= 0 {
		return nil
	}

	g.samples = append(g.samples, make([]int16, count)...)
	start := len(g.samples) - count
	phaseStep := 2 * math.Pi * frequencyHz / float64(g.SampleRate)
	scale := g.Amplitude * 32767
	for i := start; i < len(g.samples); i++ {
		g.samples[i] = int16(math.Round(math.Sin(g.phase) * scale))
		g.phase += phaseStep
		if g.phase >= 2*math.Pi || g.phase <= -2*math.Pi {
			g.phase = math.Mod(g.phase, 2*math.Pi)
		}
	}
	return nil
}

// Samples returns a copy so callers cannot mutate the generator's buffer.
func (g *ToneGenerator) Samples() []int16 {
	if g == nil {
		return nil
	}
	return append([]int16(nil), g.samples...)
}

// Duration returns the duration represented by the generated PCM samples.
func (g *ToneGenerator) Duration() time.Duration {
	if g == nil || g.SampleRate <= 0 {
		return 0
	}
	return time.Duration(int64(len(g.samples)) * int64(time.Second) / int64(g.SampleRate))
}
