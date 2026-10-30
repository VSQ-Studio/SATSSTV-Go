package audio

// PCM16ToFloat32 converts signed 16-bit mono PCM to normalized float32 samples
// suitable for MalgoDuplex playback. Values are clamped to [-1, 1).
func PCM16ToFloat32(input []int16) []float32 {
	output := make([]float32, len(input))
	for i, sample := range input {
		output[i] = float32(sample) / 32768.0
	}
	return output
}

// Float32ToPCM16 converts normalized mono float samples to signed 16-bit PCM.
func Float32ToPCM16(input []float32) []int16 {
	output := make([]int16, len(input))
	for i, sample := range input {
		if sample > 1 {
			sample = 1
		} else if sample < -1 {
			sample = -1
		}
		value := int(sample * 32768)
		if value > 32767 {
			value = 32767
		} else if value < -32768 {
			value = -32768
		}
		output[i] = int16(value)
	}
	return output
}
