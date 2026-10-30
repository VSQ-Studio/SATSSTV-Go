package sstv

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
)

const wavHeaderSize = 44

// WriteWAV writes mono signed 16-bit PCM in a standard little-endian RIFF/WAVE
// container. The complete sample slice is required so the RIFF size is known.
func WriteWAV(w io.Writer, samples []int16, sampleRate int) error {
	if w == nil {
		return errors.New("nil WAV writer")
	}
	if sampleRate < 8_000 || sampleRate > 192_000 {
		return fmt.Errorf("invalid WAV sample rate %d", sampleRate)
	}
	if uint64(len(samples))*2 > math.MaxUint32-wavHeaderSize {
		return errors.New("WAV data exceeds the RIFF 32-bit size limit")
	}
	dataSize := uint32(len(samples) * 2)
	byteRate := uint32(sampleRate * 2)

	header := make([]byte, wavHeaderSize)
	copy(header[0:4], "RIFF")
	binary.LittleEndian.PutUint32(header[4:8], 36+dataSize)
	copy(header[8:12], "WAVE")
	copy(header[12:16], "fmt ")
	binary.LittleEndian.PutUint32(header[16:20], 16)
	binary.LittleEndian.PutUint16(header[20:22], 1) // PCM
	binary.LittleEndian.PutUint16(header[22:24], 1) // mono
	binary.LittleEndian.PutUint32(header[24:28], uint32(sampleRate))
	binary.LittleEndian.PutUint32(header[28:32], byteRate)
	binary.LittleEndian.PutUint16(header[32:34], 2)  // block align
	binary.LittleEndian.PutUint16(header[34:36], 16) // bits per sample
	copy(header[36:40], "data")
	binary.LittleEndian.PutUint32(header[40:44], dataSize)
	if err := writeAll(w, header); err != nil {
		return fmt.Errorf("write WAV header: %w", err)
	}

	const chunkSamples = 16 * 1024
	buf := make([]byte, chunkSamples*2)
	for offset := 0; offset < len(samples); {
		count := min(chunkSamples, len(samples)-offset)
		for i := 0; i < count; i++ {
			binary.LittleEndian.PutUint16(buf[i*2:], uint16(samples[offset+i]))
		}
		if err := writeAll(w, buf[:count*2]); err != nil {
			return fmt.Errorf("write WAV PCM data: %w", err)
		}
		offset += count
	}
	return nil
}

func writeAll(w io.Writer, data []byte) error {
	for len(data) > 0 {
		n, err := w.Write(data)
		if err != nil {
			return err
		}
		if n == 0 {
			return io.ErrShortWrite
		}
		data = data[n:]
	}
	return nil
}

// ReadWAV reads mono or stereo 16-bit PCM RIFF/WAVE and converts stereo input
// to mono by averaging its channels. Compressed, floating-point and non-16-bit
// formats are rejected explicitly rather than silently misinterpreted.
func ReadWAV(r io.Reader) ([]int16, int, error) {
	if r == nil {
		return nil, 0, errors.New("nil WAV reader")
	}
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, 0, fmt.Errorf("read WAV file: %w", err)
	}
	if len(data) < 12 || string(data[:4]) != "RIFF" || string(data[8:12]) != "WAVE" {
		return nil, 0, errors.New("invalid RIFF/WAVE header")
	}
	var format, channels, bits, blockAlign uint16
	var sampleRate int
	var pcm []byte
	for offset := 12; offset+8 <= len(data); {
		chunkID := string(data[offset : offset+4])
		chunkSize := int(binary.LittleEndian.Uint32(data[offset+4 : offset+8]))
		chunkStart := offset + 8
		chunkEnd := chunkStart + chunkSize
		if chunkSize < 0 || chunkEnd > len(data) {
			return nil, 0, fmt.Errorf("truncated WAV %q chunk", chunkID)
		}
		switch chunkID {
		case "fmt ":
			if chunkSize < 16 {
				return nil, 0, errors.New("WAV fmt chunk is shorter than 16 bytes")
			}
			format = binary.LittleEndian.Uint16(data[chunkStart : chunkStart+2])
			channels = binary.LittleEndian.Uint16(data[chunkStart+2 : chunkStart+4])
			rate32 := binary.LittleEndian.Uint32(data[chunkStart+4 : chunkStart+8])
			if rate32 > 192_000 {
				return nil, 0, fmt.Errorf("unsupported WAV sample rate %d", rate32)
			}
			sampleRate = int(rate32)
			blockAlign = binary.LittleEndian.Uint16(data[chunkStart+12 : chunkStart+14])
			bits = binary.LittleEndian.Uint16(data[chunkStart+14 : chunkStart+16])
		case "data":
			pcm = data[chunkStart:chunkEnd]
		}
		offset = chunkEnd
		if offset%2 != 0 {
			offset++
		}
	}
	if format != 1 {
		return nil, 0, fmt.Errorf("unsupported WAV format %d; PCM is required", format)
	}
	if channels < 1 || channels > 2 || bits != 16 || blockAlign != channels*2 {
		return nil, 0, fmt.Errorf("unsupported WAV layout: channels=%d bits=%d block-align=%d; expected mono/stereo 16-bit PCM", channels, bits, blockAlign)
	}
	if sampleRate < 8_000 || sampleRate > 192_000 {
		return nil, 0, fmt.Errorf("unsupported WAV sample rate %d", sampleRate)
	}
	if pcm == nil || len(pcm) == 0 || len(pcm)%int(blockAlign) != 0 {
		return nil, 0, errors.New("WAV data chunk is empty or misaligned")
	}
	frames := len(pcm) / int(blockAlign)
	samples := make([]int16, frames)
	for frame := 0; frame < frames; frame++ {
		left := int16(binary.LittleEndian.Uint16(pcm[frame*int(blockAlign) : frame*int(blockAlign)+2]))
		if channels == 1 {
			samples[frame] = left
			continue
		}
		right := int16(binary.LittleEndian.Uint16(pcm[frame*int(blockAlign)+2 : frame*int(blockAlign)+4]))
		samples[frame] = int16((int32(left) + int32(right)) / 2)
	}
	return samples, sampleRate, nil
}
