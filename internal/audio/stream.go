package audio

import (
	"sync"
	"time"
)

// StreamOptions place a stream in its note. Rate is the device's fixed sample rate, or 0 to play at the note's own.
type StreamOptions struct {
	From   time.Duration
	Speed  float64
	Volume float64
	Rate   int
}

// Stream is one play of a note from a position at a speed. The sound backend pulls it on its own goroutine while the
// TUI reads its position, so every access locks. Seeking or changing speed starts a new stream.
type Stream struct {
	mu      sync.Mutex
	samples []float32
	rate    int
	speed   float64
	start   time.Duration
	at      int
	volume  float32
	// endedAt is when the device read the last sample, which it plays once its buffer drains.
	endedAt time.Time
	now     func() time.Time
}

func NewStream(a Audio, options StreamOptions) *Stream {
	from := min(len(a.Samples), max(0, int(options.From.Seconds()*float64(a.Rate))))
	samples := Stretch(a.Samples[from:], a.Rate, options.Speed)
	rate := a.Rate
	if options.Rate != 0 && options.Rate != a.Rate {
		samples, rate = resample(samples, a.Rate, options.Rate), options.Rate
	}
	start := time.Duration(from) * time.Second / time.Duration(a.Rate)
	return &Stream{samples: samples, rate: rate, speed: options.Speed, start: start, volume: float32(options.Volume), now: time.Now}
}

func (s *Stream) Rate() int { return s.rate }

// Read fills out with the next samples at the current volume, and with silence past the end, so a backend always has
// a full buffer and never stalls on a short stream. It returns how many were note samples and whether the note ended.
func (s *Stream) Read(out []float32) (int, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := copy(out, s.samples[s.at:])
	for i := range n {
		out[i] *= s.volume
	}
	clear(out[n:])
	s.at += n
	done := s.at == len(s.samples)
	if done && s.endedAt.IsZero() {
		s.endedAt = s.now()
	}
	return n, done
}

func (s *Stream) SetVolume(volume float64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.volume = float32(volume)
}

// Position is how far into the note the device has read, in the note's own time.
func (s *Stream) Position() time.Duration {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.positionLocked()
}

func (s *Stream) positionLocked() time.Duration {
	played := float64(s.at) * s.speed / float64(s.rate)
	return s.start + time.Duration(played*float64(time.Second))
}

// Heard is how far into the note has actually been played, given how much output the device still holds.
func (s *Stream) Heard(buffered time.Duration) time.Duration {
	s.mu.Lock()
	defer s.mu.Unlock()
	position := s.positionLocked()
	lag := time.Duration(float64(buffered) * s.speed)
	if !s.endedAt.IsZero() {
		drained := time.Duration(float64(s.now().Sub(s.endedAt)) * s.speed)
		lag = max(0, lag-drained)
	}
	return max(s.start, position-lag)
}

// Finished reports whether the device has played the note's last sample, not only read it.
func (s *Stream) Finished(buffered time.Duration) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return !s.endedAt.IsZero() && s.now().Sub(s.endedAt) >= buffered
}

// resample converts by linear interpolation, which is enough to bring 16 kHz speech up to a 48 kHz device.
func resample(samples []float32, from, to int) []float32 {
	out := make([]float32, len(samples)*to/from)
	for i := range out {
		position := float64(i) * float64(from) / float64(to)
		index := int(position)
		next := min(index+1, len(samples)-1)
		fraction := float32(position - float64(index))
		out[i] = samples[index]*(1-fraction) + samples[next]*fraction
	}
	return out
}
