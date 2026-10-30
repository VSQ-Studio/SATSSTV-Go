package sstv

import (
	"errors"
	"time"
)

const (
	visLeaderHz = 1_900.0
	visBreakHz  = 1_200.0
	visOneHz    = 1_100.0
	visZeroHz   = 1_300.0
)

// VISHeaderDuration is the standard Robot 36 calibration and VIS header
// duration: 300 ms leader, 10 ms break, 300 ms leader, then 10 x 30 ms bits.
const VISHeaderDuration = 910 * time.Millisecond

// WriteVIS emits the standard 8-bit VIS header. The seven data bits are sent
// least-significant bit first, followed by an even-parity bit and stop bit.
func WriteVIS(g *ToneGenerator, code uint8) error {
	if g == nil {
		return errors.New("nil tone generator")
	}
	if code > 0x7f {
		return errors.New("VIS code must fit in 7 data bits")
	}
	if err := g.Tone(visLeaderHz, 300*time.Millisecond); err != nil {
		return err
	}
	if err := g.Tone(visBreakHz, 10*time.Millisecond); err != nil {
		return err
	}
	if err := g.Tone(visLeaderHz, 300*time.Millisecond); err != nil {
		return err
	}
	if err := g.Tone(visBreakHz, 30*time.Millisecond); err != nil { // start bit
		return err
	}

	ones := 0
	for bit := 0; bit < 7; bit++ {
		one := (code>>bit)&1 == 1
		frequency := visZeroHz
		if one {
			frequency = visOneHz
			ones++
		}
		if err := g.Tone(frequency, 30*time.Millisecond); err != nil {
			return err
		}
	}

	// An even-parity bit makes the total number of one bits even.
	parityFrequency := visZeroHz
	if ones%2 == 1 {
		parityFrequency = visOneHz
	}
	if err := g.Tone(parityFrequency, 30*time.Millisecond); err != nil {
		return err
	}
	return g.Tone(visBreakHz, 30*time.Millisecond) // stop bit
}

// WriteMMSSTVHeader emits the documented 16-bit VIS used by MP/MR/ML modes.
// The low byte is transmitted first, each byte least-significant bit first.
func WriteMMSSTVHeader(g *ToneGenerator, code uint16) error {
	if g == nil {
		return errors.New("nil tone generator")
	}
	if err := g.ToneMS(visLeaderHz, 300); err != nil {
		return err
	}
	if err := g.ToneMS(visBreakHz, 10); err != nil {
		return err
	}
	if err := g.ToneMS(visLeaderHz, 300); err != nil {
		return err
	}
	if err := g.ToneMS(visBreakHz, 30); err != nil {
		return err
	}
	for bit := 0; bit < 16; bit++ {
		frequency := visZeroHz
		if (code>>bit)&1 != 0 {
			frequency = visOneHz
		}
		if err := g.ToneMS(frequency, 30); err != nil {
			return err
		}
	}
	return g.ToneMS(visBreakHz, 30)
}

// WriteNarrowVIS emits the MMSSTV 24-bit FSK N-VIS header.
func WriteNarrowVIS(g *ToneGenerator, code uint8) error {
	if g == nil {
		return errors.New("nil tone generator")
	}
	if code > 0x3f {
		return errors.New("N-VIS code must fit in 6 bits")
	}
	if err := g.ToneMS(1_900, 300); err != nil {
		return err
	}
	if err := g.ToneMS(2_100, 100); err != nil {
		return err
	}
	if err := g.ToneMS(1_900, 22); err != nil {
		return err
	}
	groups := [4]uint8{0b101101, 0b010101, code, 0b010101 ^ code}
	for _, group := range groups {
		for bit := 5; bit >= 0; bit-- {
			frequency := 2_100.0
			if (group>>bit)&1 != 0 {
				frequency = 1_900
			}
			if err := g.ToneMS(frequency, 22); err != nil {
				return err
			}
		}
	}
	return nil
}

// WriteAVT90Header emits three VIS 68 frames followed by the AVT countdown
// header. Each countdown word is 101 + a 5-bit counter, followed by its
// bitwise inverse. AVT90 uses 1600/2200 Hz data tones at 102.4 baud.
func WriteAVT90Header(g *ToneGenerator) error {
	if g == nil {
		return errors.New("nil tone generator")
	}
	for i := 0; i < 3; i++ {
		if err := WriteVIS(g, 68); err != nil {
			return err
		}
	}
	const symbolMS = 1000.0 / 102.4
	for count := 0; count < 32; count++ {
		if err := g.ToneMS(1_900, symbolMS); err != nil {
			return err
		}
		word := uint8(0b10100000 | count)
		for _, data := range []uint8{word, ^word} {
			for bit := 7; bit >= 0; bit-- {
				frequency := 1_600.0
				if (data>>bit)&1 != 0 {
					frequency = 2_200
				}
				if err := g.ToneMS(frequency, symbolMS); err != nil {
					return err
				}
			}
		}
	}
	return nil
}
