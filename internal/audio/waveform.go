package audio

import "math"

// Waveform is the web's waveformOf: the loudest sample of each of 48 slices, as round(sqrt(peak) * 100).
func Waveform(samples []float32) []int {
	bars := make([]int, Bars)
	for bar := range bars {
		from, to := len(samples)*bar/Bars, len(samples)*(bar+1)/Bars
		var peak float64
		for _, sample := range samples[from:to] {
			peak = max(peak, math.Abs(float64(sample)))
		}
		bars[bar] = int(math.Round(math.Sqrt(min(peak, 1)) * 100))
	}
	return bars
}
