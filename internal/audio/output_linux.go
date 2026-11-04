package audio

import (
	"sync"
	"time"

	"github.com/jfreymuth/pulse"
)

// pulseDevice talks the PulseAudio native protocol, which PipeWire also serves, so it needs no cgo or ALSA library.
type pulseDevice struct {
	mu      sync.Mutex
	client  *pulse.Client
	playing *pulse.PlaybackStream
}

func openDevice() (Device, error) {
	client, err := pulse.NewClient(pulse.ClientApplicationName("devnull"))
	if err != nil {
		return nil, ErrUnavailable
	}
	return &pulseDevice{client: client}, nil
}

// pulseLatency is the output buffer asked of the server, which is about how far playback trails reading.
const pulseLatency = 100 * time.Millisecond

func (d *pulseDevice) Rate() int { return 0 }

func (d *pulseDevice) Buffered() time.Duration { return pulseLatency }

func (d *pulseDevice) Play(s *Stream) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.stopLocked()
	// The stream pads silence past its end and the TUI stops it once played, because the library drops a chunk sent
	// with EndOfData, and Start never returns for a stream that ends before the server starts it.
	reader := pulse.Float32Reader(func(out []float32) (int, error) {
		s.Read(out)
		return len(out), nil
	})
	playback, err := d.client.NewPlayback(reader, pulse.PlaybackMono, pulse.PlaybackSampleRate(s.Rate()), pulse.PlaybackLatency(pulseLatency.Seconds()))
	if err != nil {
		return ErrUnavailable
	}
	playback.Start()
	d.playing = playback
	return nil
}

func (d *pulseDevice) Stop() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.stopLocked()
}

func (d *pulseDevice) stopLocked() {
	if d.playing == nil {
		return
	}
	d.playing.Stop()
	d.playing.Close()
	d.playing = nil
}

func (d *pulseDevice) Close() {
	d.Stop()
	d.client.Close()
}
