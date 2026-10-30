package sstv

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"image"
	"image/color"
	"math"
	"testing"
	"time"
)

func TestRobot36ModeSpec(t *testing.T) {
	spec, err := LookupMode("r36")
	if err != nil {
		t.Fatal(err)
	}
	if spec.VIS != 8 || spec.Width != 320 || spec.Height != 240 || spec.BodyTimeMS != 36_000 {
		t.Fatalf("unexpected Robot 36 spec: %+v", spec)
	}
}

func TestVISHeaderDuration(t *testing.T) {
	generator, err := NewToneGenerator(48_000, 0.5)
	if err != nil {
		t.Fatal(err)
	}
	if err := WriteVIS(generator, 8); err != nil {
		t.Fatal(err)
	}
	if got, want := len(generator.Samples()), 48_000*910/1000; got != want {
		t.Fatalf("VIS sample count = %d, want %d", got, want)
	}
}

func TestRobot36EncodeDurationAndRaster(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 320, 240))
	for y := 0; y < 240; y++ {
		for x := 0; x < 320; x++ {
			img.SetRGBA(x, y, color.RGBA{R: uint8(x * 255 / 319), G: uint8(y * 255 / 239), B: 96, A: 255})
		}
	}
	const sampleRate = 8_000
	samples, err := EncodeRobot36(context.Background(), img, sampleRate)
	if err != nil {
		t.Fatal(err)
	}
	want := sampleRate * (36_000 + 910) / 1000
	if len(samples) != want {
		t.Fatalf("encoded sample count = %d, want %d", len(samples), want)
	}
	if len(samples) == 0 {
		t.Fatal("encoder returned no samples")
	}
	peak := int16(0)
	for _, sample := range samples {
		if sample == -32768 {
			t.Fatalf("unexpected full-scale negative sample")
		}
		if sample > peak {
			peak = sample
		}
	}
	if peak < 20_000 {
		t.Fatalf("unexpectedly low peak amplitude: %d", peak)
	}
}

func TestEncodeRejectsUnknownMode(t *testing.T) {
	_, err := NewEncoder(8_000).Encode(context.Background(), image.NewRGBA(image.Rect(0, 0, 1, 1)), "not-a-real-mode")
	if err == nil {
		t.Fatal("expected unsupported mode error")
	}
}

func TestModeCatalogContainsCoreFamilies(t *testing.T) {
	want := []string{
		"B/W 8", "B/W 12", "Robot 24", "Robot 36", "Robot 72", "AVT 90",
		"Scottie 1", "Scottie 2", "ScottieDX", "Martin 1", "Martin 2",
		"SC2 180", "SC2 120", "SC2 60", "PD50", "PD90", "PD120", "PD160", "PD180", "PD240", "PD290",
		"P3", "P5", "P7", "MP73", "MP115", "MP140", "MP175", "MR73", "MR90", "MR115", "MR140", "MR175",
		"ML180", "ML240", "ML280", "ML320", "MP73-N", "MP110-N", "MP140-N", "MC110-N", "MC140-N", "MC180-N",
	}
	modes := Modes()
	if len(modes) != len(want) {
		t.Fatalf("mode catalog contains %d entries, want %d", len(modes), len(want))
	}
	for i, name := range want {
		if modes[i].Name != name {
			t.Errorf("mode %d = %q, want %q", i+1, modes[i].Name, name)
		}
		if !modes[i].CodecReady {
			t.Errorf("%s is listed as not codec-ready", name)
		}
		if _, err := LookupMode(modes[i].ID); err != nil {
			t.Errorf("mode %q lookup failed: %v", name, err)
		}
	}
}

func TestAllRegisteredModesCanBeEncoded(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 48, 48))
	for y := 0; y < 48; y++ {
		for x := 0; x < 48; x++ {
			img.SetRGBA(x, y, color.RGBA{R: uint8(x * 5), G: uint8(y * 5), B: uint8((x + y) * 2), A: 255})
		}
	}
	encoder := NewEncoder(8_000)
	for _, spec := range Modes() {
		t.Run(spec.ID, func(t *testing.T) {
			if !spec.CodecReady {
				if _, err := encoder.Encode(context.Background(), img, spec.ID); err == nil {
					t.Fatal("expected explicit not-yet-implemented codec error")
				}
				return
			}
			samples, err := encoder.Encode(context.Background(), img, spec.ID)
			if err != nil {
				t.Fatal(err)
			}
			if len(samples) < 8_000 {
				t.Fatalf("encoder returned too few samples: %d", len(samples))
			}
			headerMS := 910.0
			switch spec.Header {
			case HeaderMMSSTV:
				headerMS = mmsstvExtendedHeaderMS
			case HeaderNarrowVIS:
				headerMS = narrowVISHeaderMS
			case HeaderAVT:
				headerMS = avt90HeaderMS
			}
			wantSamples := int(math.Round((headerMS + BodyDurationMS(spec)) * float64(encoder.SampleRate) / 1000))
			if len(samples) != wantSamples {
				t.Fatalf("waveform length = %d samples, want %d for VIS + mode timing", len(samples), wantSamples)
			}
		})
	}
}

func TestEncodeHonorsCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := NewEncoder(8_000).Encode(ctx, image.NewRGBA(image.Rect(0, 0, 320, 240)), Robot36ID)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
}

func TestToneGeneratorPreservesCumulativeTiming(t *testing.T) {
	g, err := NewToneGenerator(48_000, 0.5)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 320; i++ {
		if err := g.Tone(1_900, 44*time.Millisecond/320); err != nil {
			t.Fatal(err)
		}
	}
	if got, want := len(g.Samples()), 48_000*44/1000; got != want {
		t.Fatalf("sample count = %d, want %d", got, want)
	}
}

func TestWriteWAVHeader(t *testing.T) {
	samples := []int16{0, 1000, -1000, 32767}
	var output bytes.Buffer
	if err := WriteWAV(&output, samples, 48_000); err != nil {
		t.Fatal(err)
	}
	data := output.Bytes()
	if len(data) != 44+len(samples)*2 {
		t.Fatalf("WAV length = %d", len(data))
	}
	if string(data[0:4]) != "RIFF" || string(data[8:12]) != "WAVE" || string(data[36:40]) != "data" {
		t.Fatalf("invalid WAV header: %q %q %q", data[0:4], data[8:12], data[36:40])
	}
	if got := binary.LittleEndian.Uint32(data[24:28]); got != 48_000 {
		t.Fatalf("sample rate = %d", got)
	}
	if got := binary.LittleEndian.Uint32(data[40:44]); got != uint32(len(samples)*2) {
		t.Fatalf("data size = %d", got)
	}
}

func TestReadWAVMonoRoundTrip(t *testing.T) {
	want := []int16{0, 1000, -1000, 32767, -32768}
	var encoded bytes.Buffer
	if err := WriteWAV(&encoded, want, 96_000); err != nil {
		t.Fatal(err)
	}
	got, rate, err := ReadWAV(bytes.NewReader(encoded.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	if rate != 96_000 {
		t.Fatalf("sample rate = %d, want 96000", rate)
	}
	if len(got) != len(want) {
		t.Fatalf("sample count = %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("sample %d = %d, want %d", i, got[i], want[i])
		}
	}
}

func TestReadWAVStereoDownmixesToMono(t *testing.T) {
	var data bytes.Buffer
	data.WriteString("RIFF")
	_ = binary.Write(&data, binary.LittleEndian, uint32(40))
	data.WriteString("WAVEfmt ")
	_ = binary.Write(&data, binary.LittleEndian, uint32(16))
	_ = binary.Write(&data, binary.LittleEndian, uint16(1))
	_ = binary.Write(&data, binary.LittleEndian, uint16(2))
	_ = binary.Write(&data, binary.LittleEndian, uint32(48_000))
	_ = binary.Write(&data, binary.LittleEndian, uint32(48_000*4))
	_ = binary.Write(&data, binary.LittleEndian, uint16(4))
	_ = binary.Write(&data, binary.LittleEndian, uint16(16))
	data.WriteString("data")
	_ = binary.Write(&data, binary.LittleEndian, uint32(4))
	_ = binary.Write(&data, binary.LittleEndian, int16(-1000))
	_ = binary.Write(&data, binary.LittleEndian, int16(2000))
	got, rate, err := ReadWAV(bytes.NewReader(data.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	if rate != 48_000 || len(got) != 1 || got[0] != 500 {
		t.Fatalf("stereo downmix = %v at %d Hz, want [500] at 48000 Hz", got, rate)
	}
}

func TestModeLookupAcceptsFullWidthHyphenAliases(t *testing.T) {
	for _, alias := range []string{"MP73－N", "MP110－N", "MP140－N", "MC110－N", "MC140－N", "MC180－N"} {
		t.Run(alias, func(t *testing.T) {
			if _, err := LookupMode(alias); err != nil {
				t.Fatalf("LookupMode(%q): %v", alias, err)
			}
		})
	}
}
