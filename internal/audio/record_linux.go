package audio

import (
	"sync/atomic"
	"time"

	"github.com/jfreymuth/pulse"
)

type pulseRecorder struct {
	takes
	client   *pulse.Client
	stream   *pulse.RecordStream
	released atomic.Bool
}

func startRecording() (Recorder, error) {
	client, err := pulse.NewClient(pulse.ClientApplicationName("devnull"))
	if err != nil {
		return nil, ErrMicrophone
	}
	if _, err := client.DefaultSource(); err != nil {
		client.Close()
		return nil, ErrNoMicrophone
	}
	r := &pulseRecorder{client: client}
	writer := pulse.Float32Writer(func(samples []float32) (int, error) {
		r.add(samples)
		return len(samples), nil
	})
	r.stream, err = client.NewRecord(writer, pulse.RecordMono, pulse.RecordSampleRate(RecordRate), pulse.RecordLatency(0.05), pulse.RecordMediaName("Voice note"))
	if err != nil {
		client.Close()
		return nil, ErrMicrophone
	}
	r.stream.Start()
	return r, nil
}

func (r *pulseRecorder) Level() float64         { return r.level() }
func (r *pulseRecorder) Elapsed() time.Duration { return r.elapsed() }

// Err catches the server dropping the stream, which the library reports only as a closed stream.
func (r *pulseRecorder) Err() error {
	if !r.released.Load() && r.stream.Closed() {
		return ErrMicrophone
	}
	return r.err()
}

func (r *pulseRecorder) Stop() []float32 {
	r.release()
	return r.all()
}

func (r *pulseRecorder) Cancel() { r.release() }

func (r *pulseRecorder) release() {
	r.released.Store(true)
	r.stream.Stop()
	r.stream.Close()
	r.client.Close()
}
