package audio

import "testing"

func TestPCMConversionPreservesFullScaleAndZero(t *testing.T) {
	input := []int16{-32768, -16384, 0, 16384, 32767}
	floatSamples := PCM16ToFloat32(input)
	if floatSamples[0] != -1 || floatSamples[2] != 0 || floatSamples[4] <= 0.99 {
		t.Fatalf("unexpected float conversion: %v", floatSamples)
	}
	back := Float32ToPCM16(floatSamples)
	if back[0] != -32768 || back[2] != 0 || back[4] != 32767 {
		t.Fatalf("unexpected PCM round trip: %v", back)
	}
}

func TestFloat32ToPCM16ClampsOutOfRangeValues(t *testing.T) {
	got := Float32ToPCM16([]float32{-2, -1, 1, 2})
	want := []int16{-32768, -32768, 32767, 32767}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("sample %d = %d, want %d", i, got[i], want[i])
		}
	}
}
