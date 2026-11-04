package audio

import "math"

// Stretch plays mono samples faster without raising their pitch, as the web's audio element does, using WSOLA: each
// 20 ms window is taken near where the speed puts it, shifted up to 2.5 ms to line up with the previous one.
func Stretch(samples []float32, rate int, speed float64) []float32 {
	if speed == 1 || len(samples) == 0 {
		return samples
	}
	frame := rate / 50
	half := frame / 2
	search := rate / 400
	window := make([]float32, frame)
	for i := range window {
		window[i] = float32(0.5 - 0.5*math.Cos(2*math.Pi*float64(i)/float64(frame)))
	}
	length := int(float64(len(samples)) / speed)
	out := make([]float32, length+frame)
	weight := make([]float32, length+frame)
	previous := 0
	for at := 0; at < length; at += half {
		from := min(int(float64(at)*speed), max(0, len(samples)-frame))
		if at > 0 {
			from = bestMatch(samples, previous+half, from, search, half)
		}
		for i, w := range window {
			if from+i >= len(samples) {
				break
			}
			out[at+i] += samples[from+i] * w
			weight[at+i] += w
		}
		previous = from
	}
	for i := range out {
		if weight[i] > 1e-3 {
			out[i] /= weight[i]
		}
	}
	return out[:length]
}

// bestMatch is the start near ideal whose next half window looks most like the one that naturally follows the
// previous window, so the overlap adds up without cancelling. Every fourth sample is enough to compare them.
func bestMatch(samples []float32, natural, ideal, search, half int) int {
	if natural+half > len(samples) {
		return ideal
	}
	best, bestScore := ideal, float32(math.Inf(-1))
	for candidate := max(0, ideal-search); candidate <= min(ideal+search, len(samples)-half); candidate++ {
		var score float32
		for i := 0; i < half; i += 4 {
			score += samples[natural+i] * samples[candidate+i]
		}
		if score > bestScore {
			best, bestScore = candidate, score
		}
	}
	return best
}
