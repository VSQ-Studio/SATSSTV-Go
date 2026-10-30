package sstv

import (
	"context"
	"errors"
	"fmt"
	"image"
	"image/color"
	"math"
	"strings"
)

// DecodeResult contains one reconstructed SSTV frame and the detected mode.
type DecodeResult struct {
	Image     *image.NRGBA
	Mode      ModeSpec
	VIS       uint8
	BodyStart int
}

// DetectVIS searches the beginning of an audio buffer for a standard 7-bit
// VIS header. It returns the code and the sample offset immediately after the
// 910 ms header. Input is signed mono PCM.
func DetectVIS(samples []int16, sampleRate int) (uint8, int, error) {
	code, bodyStart, _, err := detectVISFrom(samples, sampleRate, 0)
	return code, bodyStart, err
}

// detectVISFrom searches only start offsets at or after startAt. nextStart is
// the first aligned offset that could not yet be checked because a complete
// 910 ms header was unavailable, or the next offset after this search window.
// StreamDecoder uses it to avoid rescanning the same idle audio on every feed.
func detectVISFrom(samples []int16, sampleRate, startAt int) (code uint8, bodyStart, nextStart int, retErr error) {
	if sampleRate < 8_000 || sampleRate > 192_000 {
		return 0, 0, startAt, fmt.Errorf("invalid sample rate %d", sampleRate)
	}
	step := max(1, sampleRate/500) // Search every 2 ms.
	leaderN := int(math.Round(0.300 * float64(sampleRate)))
	breakN := int(math.Round(0.010 * float64(sampleRate)))
	bitN := int(math.Round(0.030 * float64(sampleRate)))
	headerN := int(math.Round(0.910 * float64(sampleRate)))
	if len(samples) < headerN {
		return 0, 0, startAt, errors.New("audio is too short to contain a VIS header")
	}
	maxStart := min(len(samples)-headerN, sampleRate*3)
	start := max(0, startAt)
	if remainder := start % step; remainder != 0 {
		start += step - remainder
	}
	for ; start <= maxStart; start += step {
		if bestTone(samples[start:start+leaderN], sampleRate, 1_900, 1_200) != 1_900 {
			continue
		}
		p := start + leaderN
		if bestTone(samples[p:p+breakN], sampleRate, 1_200, 1_900) != 1_200 {
			continue
		}
		p += breakN
		if bestTone(samples[p:p+leaderN], sampleRate, 1_900, 1_200) != 1_900 {
			continue
		}
		p += leaderN
		if bestTone(samples[p:p+bitN], sampleRate, 1_200, 1_900) != 1_200 {
			continue
		}
		p += bitN

		var code uint8
		ones := 0
		valid := true
		for bit := 0; bit < 7; bit++ {
			f := bestTone(samples[p:p+bitN], sampleRate, 1_100, 1_300)
			if f == 1_100 {
				code |= 1 << bit
				ones++
			} else if f != 1_300 {
				valid = false
			}
			p += bitN
		}
		parityFreq := bestTone(samples[p:p+bitN], sampleRate, 1_100, 1_300)
		parityOne := parityFreq == 1_100
		parityZero := parityFreq == 1_300
		if (!parityOne && !parityZero) || ((ones+boolInt(parityOne))%2 != 0) {
			valid = false
		}
		p += bitN
		stopFreq := bestTone(samples[p:p+bitN], sampleRate, 1_200, 1_100)
		if stopFreq != 1_200 {
			valid = false
		}
		if valid {
			return code, start + headerN, start, nil
		}
	}
	return 0, 0, maxStart + 1, errors.New("standard SSTV VIS header not detected")
}

func boolInt(v bool) int {
	if v {
		return 1
	}
	return 0
}

func bestTone(samples []int16, sampleRate int, candidates ...float64) float64 {
	if len(samples) == 0 || sampleRate <= 0 || len(candidates) == 0 {
		return 0
	}
	best, bestPower := 0.0, -1.0
	var energy float64
	for candidateIndex, frequency := range candidates {
		omega := 2 * math.Pi * frequency / float64(sampleRate)
		coeff := 2 * math.Cos(omega)
		s1, s2 := 0.0, 0.0
		for _, sample := range samples {
			x := float64(sample) / 32768
			if candidateIndex == 0 {
				energy += x * x
			}
			s0 := x + coeff*s1 - s2
			s2, s1 = s1, s0
		}
		power := s1*s1 + s2*s2 - coeff*s1*s2
		if power > bestPower {
			best, bestPower = frequency, power
		}
	}
	// Goertzel returns equal zero power for every candidate on digital
	// silence. Without an energy gate that tie is resolved in favour of the
	// first frequency and silence can look exactly like a valid VIS header.
	// 1e-12 mean-square is -120 dBFS, below any meaningful 16-bit capture.
	if energy/float64(len(samples)) <= 1e-12 {
		return 0
	}
	return best
}

// Decode performs standard VIS detection followed by mode-specific image
// reconstruction. It is a whole-frame reference decoder; the live RX worker
// can use the same per-line demodulation functions incrementally.
func Decode(ctx context.Context, samples []int16, sampleRate int) (*DecodeResult, error) {
	spec, vis, bodyStart, err := DetectModeHeader(samples, sampleRate)
	if err != nil {
		return nil, err
	}
	img, err := DecodeMode(ctx, samples, sampleRate, spec.ID, bodyStart)
	if err != nil {
		return nil, err
	}
	return &DecodeResult{Image: img, Mode: spec, VIS: vis, BodyStart: bodyStart}, nil
}

// DecodeMode decodes a known mode from a body offset. bodyStart is measured in
// samples and should point immediately after the VIS stop bit. Pass 0 to use
// the standard 910 ms VIS header offset.
func DecodeMode(ctx context.Context, samples []int16, sampleRate int, modeID string, bodyStart int) (*image.NRGBA, error) {
	spec, err := LookupMode(modeID)
	if err != nil {
		return nil, err
	}
	if sampleRate < 8_000 || sampleRate > 192_000 {
		return nil, fmt.Errorf("invalid sample rate %d", sampleRate)
	}
	if bodyStart == 0 {
		headerMS := 910.0
		switch spec.Header {
		case HeaderMMSSTV:
			headerMS = 1_150
		case HeaderNarrowVIS:
			headerMS = 950
		case HeaderAVT:
			headerMS = avt90HeaderMS
		}
		bodyStart = int(math.Round(headerMS / 1000 * float64(sampleRate)))
	}
	if bodyStart < 0 || bodyStart >= len(samples) {
		return nil, errors.New("invalid SSTV image body offset")
	}
	freq := instantaneousFrequency(samples, sampleRate)
	clock := sampleClock{rate: sampleRate, pos: bodyStart}
	img := image.NewNRGBA(image.Rect(0, 0, spec.Width, spec.Height))

	switch spec.Family {
	case FamilyRobot:
		if err := decodeRobot(ctx, freq, &clock, img, spec); err != nil {
			return nil, err
		}
	case FamilyMartin, FamilyScottie, FamilyWraase, FamilyPasokon:
		if err := decodeRGB(ctx, freq, &clock, img, spec); err != nil {
			return nil, err
		}
	case FamilyPD:
		if err := decodePD(ctx, freq, &clock, img, spec); err != nil {
			return nil, err
		}
	case FamilyMP:
		if err := decodeMP(ctx, freq, &clock, img, spec); err != nil {
			return nil, err
		}
	case FamilyMRML:
		if err := decodeMRML(ctx, freq, &clock, img, spec); err != nil {
			return nil, err
		}
	case FamilyMCN, FamilyAVT:
		if err := decodeRGBProfile(ctx, freq, &clock, img, spec); err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("decoder for family %q is not implemented", spec.Family)
	}
	return img, nil
}

type sampleClock struct {
	rate       int
	pos        int
	fractional float64
}

func (c *sampleClock) skip(ms float64) {
	exact := ms/1000*float64(c.rate) + c.fractional
	n := int(math.Round(exact))
	c.fractional = exact - float64(n)
	c.pos += n
}

func (c *sampleClock) scan(freq []float64, width int, totalMS float64) ([]uint8, error) {
	return c.scanRange(freq, width, totalMS, VideoMinHz, VideoMaxHz)
}

func (c *sampleClock) scanMode(freq []float64, width int, totalMS float64, spec ModeSpec) ([]uint8, error) {
	lo, hi := videoRange(spec)
	return c.scanRange(freq, width, totalMS, lo, hi)
}

func (c *sampleClock) scanRange(freq []float64, width int, totalMS, minHz, maxHz float64) ([]uint8, error) {
	if width <= 0 {
		return nil, errors.New("invalid scan width")
	}
	out := make([]uint8, width)
	pixelSeconds := totalMS / 1000 / float64(width)
	for x := 0; x < width; x++ {
		exact := pixelSeconds*float64(c.rate) + c.fractional
		n := int(math.Round(exact))
		c.fractional = exact - float64(n)
		start, end := c.pos, c.pos+n
		c.pos = end
		if start < 0 || end > len(freq) || n <= 0 {
			return nil, errors.New("SSTV audio ended before the image scan completed")
		}
		lo, hi := start+n/4, end-n/4
		if hi <= lo {
			lo, hi = start, end
		}
		sum, count := 0.0, 0
		for i := lo; i < hi; i++ {
			if freq[i] > 500 && freq[i] < 3_500 {
				sum += freq[i]
				count++
			}
		}
		if count == 0 {
			return nil, fmt.Errorf("unable to estimate SSTV pixel frequency at pixel %d (samples %d..%d)", x, start, end)
		}
		out[x] = frequencyToByteRange(sum/float64(count), minHz, maxHz)
	}
	return out, nil
}

func frequencyToByteRange(frequency, minHz, maxHz float64) uint8 {
	v := (frequency - minHz) * 255 / (maxHz - minHz)
	return uint8(math.Max(0, math.Min(255, math.Round(v))))
}

func frequencyToByte(frequency float64) uint8 {
	v := (frequency - VideoMinHz) * 255 / (VideoMaxHz - VideoMinHz)
	return uint8(math.Max(0, math.Min(255, math.Round(v))))
}

// instantaneousFrequency down-converts the SSTV video band around 1900 Hz,
// smooths the I/Q streams with a short box filter and differentiates phase.
// Pixel centers are sampled later by the per-mode line readers.
func instantaneousFrequency(samples []int16, sampleRate int) []float64 {
	// A second-order digital PLL tracks the continuous phase of the SSTV
	// carrier. The loop starts at the VIS video centre frequency and is already
	// locked by the 1900 Hz VIS leaders before image pixels arrive. A Costas-like
	// quadrature detector avoids trying to estimate a full sine period inside
	// every short pixel dwell (some are only 6-7 samples at 48 kHz).
	freq := make([]float64, len(samples))
	const (
		phaseGain = 0.15
		freqGain  = 0.002
		loopLPF   = 0.30
	)
	phase, estimate := 0.0, VideoCenterHz
	lowI, lowQ := 0.0, 0.0
	for n, sample := range samples {
		x := float64(sample) / 32768
		localI := x * math.Cos(phase)
		localQ := -x * math.Sin(phase)
		lowI += loopLPF * (localI - lowI)
		lowQ += loopLPF * (localQ - lowQ)
		phaseError := math.Atan2(lowI, -lowQ)
		estimate += freqGain * phaseError * float64(sampleRate) / (2 * math.Pi)
		// Clamp the loop to the normal SSTV audio band so an isolated clipped
		// sample cannot send the NCO outside the signal acquisition range.
		estimate = math.Max(900, math.Min(2_600, estimate))
		phase += 2*math.Pi*estimate/float64(sampleRate) + phaseGain*phaseError
		phase = math.Mod(phase, 2*math.Pi)
		if phase < 0 {
			phase += 2 * math.Pi
		}
		freq[n] = estimate
	}
	return freq
}

func decodeRobot(ctx context.Context, freq []float64, clock *sampleClock, img *image.NRGBA, spec ModeSpec) error {
	if spec.Monochrome {
		for row := 0; row < spec.Height; row++ {
			if err := ctxErr(ctx); err != nil {
				return err
			}
			clock.skip(spec.SyncMS)
			luma, err := clock.scanMode(freq, spec.Width, spec.ScanMS, spec)
			if err != nil {
				return err
			}
			for x, value := range luma {
				img.SetNRGBA(x, row, color.NRGBA{R: value, G: value, B: value, A: 255})
			}
		}
		return nil
	}
	if spec.RobotSequential {
		for row := 0; row < spec.Height; row++ {
			if err := ctxErr(ctx); err != nil {
				return err
			}
			clock.skip(spec.SyncMS + spec.SyncPorchMS)
			y, err := clock.scanMode(freq, spec.Width, spec.LumaMS, spec)
			if err != nil {
				return err
			}
			clock.skip(spec.SeparatorMS + spec.ColorPorchMS)
			cr, err := clock.scanMode(freq, spec.Width, spec.ChromaMS, spec)
			if err != nil {
				return err
			}
			clock.skip(spec.SeparatorMS + spec.ColorPorchMS)
			cb, err := clock.scanMode(freq, spec.Width, spec.ChromaMS, spec)
			if err != nil {
				return err
			}
			for x := 0; x < spec.Width; x++ {
				r, g, b := color.YCbCrToRGB(y[x], cb[x], cr[x])
				img.SetNRGBA(x, row, color.NRGBA{R: r, G: g, B: b, A: 255})
			}
		}
		return nil
	}

	yRows := make([][]uint8, spec.Height)
	chroma := make([][]uint8, spec.Height)
	for row := 0; row < spec.Height; row++ {
		if err := ctxErr(ctx); err != nil {
			return err
		}
		clock.skip(spec.SyncMS)
		clock.skip(spec.SyncPorchMS)
		y, err := clock.scanMode(freq, spec.Width, spec.LumaMS, spec)
		if err != nil {
			return err
		}
		yRows[row] = y
		clock.skip(spec.SeparatorMS + spec.ColorPorchMS)
		c, err := clock.scanMode(freq, spec.Width, spec.ChromaMS, spec)
		if err != nil {
			return err
		}
		chroma[row] = c
	}
	for row := 0; row < spec.Height; row += 2 {
		if row+1 >= spec.Height {
			break
		}
		cr, cb := chroma[row], chroma[row+1]
		for x := 0; x < spec.Width; x++ {
			r, g, b := color.YCbCrToRGB(yRows[row][x], cb[x], cr[x])
			img.SetNRGBA(x, row, color.NRGBA{R: r, G: g, B: b, A: 255})
			r, g, b = color.YCbCrToRGB(yRows[row+1][x], cb[x], cr[x])
			img.SetNRGBA(x, row+1, color.NRGBA{R: r, G: g, B: b, A: 255})
		}
	}
	return nil
}

func decodeRGB(ctx context.Context, freq []float64, clock *sampleClock, img *image.NRGBA, spec ModeSpec) error {
	for row := 0; row < spec.Height; row++ {
		if err := ctxErr(ctx); err != nil {
			return err
		}
		values := map[byte][]uint8{'R': nil, 'G': nil, 'B': nil}
		scan := func(channel byte, duration float64) error {
			if duration <= 0 {
				duration = spec.ScanMS
			}
			v, err := clock.scan(freq, spec.Width, duration)
			if err == nil {
				values[channel] = v
			}
			return err
		}
		porch := func() { clock.skip(spec.PorchMS) }
		switch spec.Family {
		case FamilyMartin:
			clock.skip(spec.SyncMS)
			porch()
			for _, channel := range []byte("GBR") {
				if err := scan(channel, spec.ScanMS); err != nil {
					return err
				}
				porch()
			}
		case FamilyScottie:
			porch()
			if err := scan('G', spec.ScanMS); err != nil {
				return err
			}
			porch()
			if err := scan('B', spec.ScanMS); err != nil {
				return err
			}
			clock.skip(spec.SyncMS)
			porch()
			if err := scan('R', spec.ScanMS); err != nil {
				return err
			}
		case FamilyWraase:
			clock.skip(spec.SyncMS)
			for i, channel := range []byte("RGB") {
				if i == 0 || spec.PorchEachChannel {
					porch()
				}
				duration := spec.RGBScanMS[i]
				if err := scan(channel, duration); err != nil {
					return err
				}
			}
			if spec.PorchEachChannel {
				porch()
			}
		case FamilyPasokon:
			clock.skip(spec.SyncMS)
			for _, channel := range []byte("RGB") {
				porch()
				if err := scan(channel, spec.ScanMS); err != nil {
					return err
				}
			}
			porch()
		}
		for x := 0; x < spec.Width; x++ {
			img.SetNRGBA(x, row, color.NRGBA{R: values['R'][x], G: values['G'][x], B: values['B'][x], A: 255})
		}
	}
	return nil
}

func decodePD(ctx context.Context, freq []float64, clock *sampleClock, img *image.NRGBA, spec ModeSpec) error {
	for row := 0; row < spec.Height; row += 2 {
		if err := ctxErr(ctx); err != nil {
			return err
		}
		clock.skip(spec.SyncMS)
		clock.skip(spec.PorchMS)
		y0, err := clock.scanMode(freq, spec.Width, spec.ChannelScanMS, spec)
		if err != nil {
			return err
		}
		cr, err := clock.scanMode(freq, spec.Width, spec.ChannelScanMS, spec)
		if err != nil {
			return err
		}
		cb, err := clock.scanMode(freq, spec.Width, spec.ChannelScanMS, spec)
		if err != nil {
			return err
		}
		y1, err := clock.scanMode(freq, spec.Width, spec.ChannelScanMS, spec)
		if err != nil {
			return err
		}
		for x := 0; x < spec.Width; x++ {
			r, g, b := color.YCbCrToRGB(y0[x], cb[x], cr[x])
			img.SetNRGBA(x, row, color.NRGBA{R: r, G: g, B: b, A: 255})
			r, g, b = color.YCbCrToRGB(y1[x], cb[x], cr[x])
			img.SetNRGBA(x, row+1, color.NRGBA{R: r, G: g, B: b, A: 255})
		}
	}
	return nil
}

func decodeMP(ctx context.Context, freq []float64, clock *sampleClock, img *image.NRGBA, spec ModeSpec) error {
	for row := 0; row < spec.Height; row += 2 {
		if err := ctxErr(ctx); err != nil {
			return err
		}
		clock.skip(spec.SyncMS + spec.PorchMS)
		y0, err := clock.scanMode(freq, spec.Width, spec.ChannelScanMS, spec)
		if err != nil {
			return err
		}
		cr, err := clock.scanMode(freq, spec.Width, spec.ChannelScanMS, spec)
		if err != nil {
			return err
		}
		cb, err := clock.scanMode(freq, spec.Width, spec.ChannelScanMS, spec)
		if err != nil {
			return err
		}
		y1, err := clock.scanMode(freq, spec.Width, spec.ChannelScanMS, spec)
		if err != nil {
			return err
		}
		for x := 0; x < spec.Width; x++ {
			r, g, b := color.YCbCrToRGB(y0[x], cb[x], cr[x])
			img.SetNRGBA(x, row, color.NRGBA{R: r, G: g, B: b, A: 255})
			r, g, b = color.YCbCrToRGB(y1[x], cb[x], cr[x])
			img.SetNRGBA(x, row+1, color.NRGBA{R: r, G: g, B: b, A: 255})
		}
	}
	return nil
}

func decodeMRML(ctx context.Context, freq []float64, clock *sampleClock, img *image.NRGBA, spec ModeSpec) error {
	for row := 0; row < spec.Height; row++ {
		if err := ctxErr(ctx); err != nil {
			return err
		}
		clock.skip(spec.SyncMS + spec.PorchMS)
		y, err := clock.scanMode(freq, spec.Width, spec.ChannelScanMS, spec)
		if err != nil {
			return err
		}
		clock.skip(spec.PixelHoldMS)
		cr, err := clock.scanMode(freq, spec.Width/2, spec.ChannelScanMS/2, spec)
		if err != nil {
			return err
		}
		clock.skip(spec.PixelHoldMS)
		cb, err := clock.scanMode(freq, spec.Width/2, spec.ChannelScanMS/2, spec)
		if err != nil {
			return err
		}
		clock.skip(spec.PixelHoldMS)
		for x := 0; x < spec.Width; x++ {
			r, g, b := color.YCbCrToRGB(y[x], cb[x/2], cr[x/2])
			img.SetNRGBA(x, row, color.NRGBA{R: r, G: g, B: b, A: 255})
		}
	}
	return nil
}

func decodeRGBProfile(ctx context.Context, freq []float64, clock *sampleClock, img *image.NRGBA, spec ModeSpec) error {
	for row := 0; row < spec.Height; row++ {
		if err := ctxErr(ctx); err != nil {
			return err
		}
		if spec.Family == FamilyMCN {
			clock.skip(spec.SyncMS + spec.PorchMS)
		}
		var channels [3][]uint8
		for i := 0; i < 3; i++ {
			duration := spec.RGBScanMS[i]
			if duration <= 0 {
				duration = spec.ScanMS
			}
			var err error
			channels[i], err = clock.scanMode(freq, spec.Width, duration, spec)
			if err != nil {
				return err
			}
		}
		for x := 0; x < spec.Width; x++ {
			img.SetNRGBA(x, row, color.NRGBA{R: channels[0][x], G: channels[1][x], B: channels[2][x], A: 255})
		}
	}
	return nil
}

// StreamEvent is emitted when a complete SSTV frame has been reconstructed.
// Frame delivery is independent of the radio TX state, so the caller can feed
// this decoder from a continuously running capture stream during transmission.
type StreamEvent struct {
	Image *image.NRGBA
	Mode  ModeSpec
	VIS   uint8
}

// StreamDecoder accepts arbitrary-sized PCM chunks and automatically detects
// sequential VIS-framed transmissions. It retains at most one active frame
// plus a short idle search window. Images are emitted at frame completion;
// progressive row events can be added without changing this API.
type StreamDecoder struct {
	rate          int
	pending       []int16
	mode          *ModeSpec
	bodyStart     int
	maxIdle       int
	visSearchFrom int
	skipUntil     int // end offset of a complete frame rejected by fixed-mode filtering
	fixedMode     string
	activeVIS     uint8
}

func NewStreamDecoder(sampleRate int) (*StreamDecoder, error) {
	if sampleRate < 8_000 || sampleRate > 192_000 {
		return nil, fmt.Errorf("invalid sample rate %d", sampleRate)
	}
	return &StreamDecoder{rate: sampleRate, maxIdle: sampleRate * 12}, nil
}

// Feed appends one captured PCM block and returns every complete image now
// available. It is safe to use during a simultaneous audio playback session.
func (d *StreamDecoder) Feed(ctx context.Context, samples []int16) ([]StreamEvent, error) {
	if d == nil {
		return nil, errors.New("nil SSTV stream decoder")
	}
	if err := ctxErr(ctx); err != nil {
		return nil, err
	}
	if len(samples) > 0 {
		d.pending = append(d.pending, samples...)
	}
	var events []StreamEvent
	for {
		if err := ctxErr(ctx); err != nil {
			return events, err
		}

		// A fixed RX slot rejects an entire non-matching image. Do not resume
		// VIS hunting inside its pixel data, where noise or image tones could
		// accidentally resemble another header.
		if d.skipUntil > 0 {
			if len(d.pending) < d.skipUntil {
				return events, nil
			}
			d.pending = append([]int16(nil), d.pending[d.skipUntil:]...)
			d.skipUntil = 0
			d.visSearchFrom = 0
		}

		if d.mode == nil {
			spec, vis, bodyStart, _, nextSearch, err := detectModeHeaderFrom(d.pending, d.rate, d.visSearchFrom)
			if err != nil {
				d.visSearchFrom = max(d.visSearchFrom, nextSearch)
				// Header probes are deliberately bounded to a 3 s search window.
				// Once every candidate in that window has been examined, discard
				// already-scanned idle audio while retaining 1 s before the next
				// candidate. This keeps memory bounded and lets short frames be
				// detected after arbitrarily long receiver idle periods.
				if d.visSearchFrom > d.rate*2 {
					trim := min(d.visSearchFrom-d.rate, len(d.pending))
					if trim > 0 {
						d.pending = append([]int16(nil), d.pending[trim:]...)
						d.visSearchFrom -= trim
					}
				}
				if len(d.pending) > d.maxIdle {
					trim := len(d.pending) - d.maxIdle
					d.pending = append([]int16(nil), d.pending[trim:]...)
					d.visSearchFrom = max(0, d.visSearchFrom-trim)
				}
				return events, nil
			}
			if d.fixedMode != "" && spec.ID != d.fixedMode {
				bodySamples := int(math.Round(BodyDurationMS(spec) * float64(d.rate) / 1000))
				frameEnd := bodyStart + bodySamples
				if len(d.pending) < frameEnd {
					d.skipUntil = frameEnd
					return events, nil
				}
				d.pending = append([]int16(nil), d.pending[frameEnd:]...)
				d.visSearchFrom = 0
				continue
			}
			d.mode, d.bodyStart, d.activeVIS = &spec, bodyStart, vis
		}
		bodyMS := BodyDurationMS(*d.mode)
		bodySamples := int(math.Round(bodyMS * float64(d.rate) / 1000))
		frameEnd := d.bodyStart + bodySamples
		if len(d.pending) < frameEnd {
			return events, nil
		}
		img, err := DecodeMode(ctx, d.pending[:frameEnd], d.rate, d.mode.ID, d.bodyStart)
		if err != nil {
			if ctxErr(ctx) != nil {
				return events, err
			}
			// A complete but undecodable frame must not poison the stream forever.
			// Discard its known duration and resume header hunting on the next feed.
			failedMode := d.mode.Name
			d.pending = append([]int16(nil), d.pending[frameEnd:]...)
			d.mode = nil
			d.bodyStart = 0
			d.activeVIS = 0
			d.visSearchFrom = 0
			return events, fmt.Errorf("decode streamed %s frame: %w", failedMode, err)
		}
		events = append(events, StreamEvent{Image: img, Mode: *d.mode, VIS: d.activeVIS})
		d.pending = append([]int16(nil), d.pending[frameEnd:]...)
		d.mode = nil
		d.bodyStart = 0
		d.activeVIS = 0
		d.visSearchFrom = 0
	}
}

// SetMode selects a fixed receive mode. An empty string restores Auto detection.
func (d *StreamDecoder) SetMode(modeID string) error {
	if d == nil {
		return errors.New("nil SSTV stream decoder")
	}
	if strings.TrimSpace(modeID) == "" {
		d.fixedMode = ""
		d.mode = nil
		d.bodyStart = 0
		d.skipUntil = 0
		d.visSearchFrom = 0
		d.activeVIS = 0
		return nil
	}
	spec, err := LookupMode(modeID)
	if err != nil {
		return err
	}
	if !spec.CodecReady {
		return fmt.Errorf("%s decoder is not implemented", spec.Name)
	}
	d.fixedMode = spec.ID
	d.mode = nil
	d.bodyStart = 0
	d.skipUntil = 0
	d.visSearchFrom = 0
	d.activeVIS = 0
	return nil
}

// Reset discards the current partial image and returns to VIS hunting.
func (d *StreamDecoder) Reset() {
	if d == nil {
		return
	}
	d.pending = nil
	d.mode = nil
	d.bodyStart = 0
	d.skipUntil = 0
	d.visSearchFrom = 0
	d.activeVIS = 0
}

// ActiveMode reports the currently detected in-progress image, if any.
func (d *StreamDecoder) ActiveMode() (ModeSpec, bool) {
	if d == nil || d.mode == nil {
		return ModeSpec{}, false
	}
	return *d.mode, true
}
