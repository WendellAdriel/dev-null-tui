//go:build darwin || windows

package audio

import (
	"encoding/binary"
	"math"
	"sync"
	"time"

	"github.com/ebitengine/oto/v3"
)

// otoRate is fixed because Oto allows one context per process, and every stream is resampled to it.
const otoRate = 48000

var (
	otoOnce    sync.Once
	otoContext *oto.Context
)

type otoDevice struct {
	mu      sync.Mutex
	playing *oto.Player
}

func openDevice() (Device, error) {
	otoOnce.Do(func() {
		context, ready, err := oto.NewContext(&oto.NewContextOptions{SampleRate: otoRate, ChannelCount: 1, Format: oto.FormatFloat32LE})
		if err != nil {
			return
		}
		<-ready
		otoContext = context
	})
	if otoContext == nil {
		return nil, ErrUnavailable
	}
	return &otoDevice{}, nil
}

func (d *otoDevice) Rate() int { return otoRate }

func (d *otoDevice) Buffered() time.Duration {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.playing == nil {
		return 0
	}
	return time.Duration(d.playing.BufferedSize()/4) * time.Second / otoRate
}

func (d *otoDevice) Play(s *Stream) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.stopLocked()
	d.playing = otoContext.NewPlayer(&streamBytes{stream: s})
	d.playing.Play()
	return nil
}

func (d *otoDevice) Stop() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.stopLocked()
}

func (d *otoDevice) stopLocked() {
	if d.playing == nil {
		return
	}
	// Oto v3.4 frees a player without Close, which it deprecates.
	d.playing.Pause()
	d.playing = nil
}

// Close keeps the context, since Oto can't create another one in the same process.
func (d *otoDevice) Close() { d.Stop() }

// streamBytes hands a stream to Oto as little-endian float32 bytes. It never reports io.EOF: the stream pads silence
// past its end, and the TUI stops the player once Oto's buffer has played out.
type streamBytes struct {
	stream  *Stream
	samples []float32
}

func (r *streamBytes) Read(out []byte) (int, error) {
	count := len(out) / 4
	if cap(r.samples) < count {
		r.samples = make([]float32, count)
	}
	r.stream.Read(r.samples[:count])
	for i, sample := range r.samples[:count] {
		binary.LittleEndian.PutUint32(out[4*i:], math.Float32bits(sample))
	}
	return 4 * count, nil
}
