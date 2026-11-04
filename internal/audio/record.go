package audio

import (
	"errors"
	"math"
	"slices"
	"sync"
	"time"
)

// MaxRecording is the web's maxVoiceMs. The server allows a second more for the recorder to stop.
const MaxRecording = 60 * time.Second

// MaxSilence is how long a microphone may send only exact zeros. A Bluetooth headset took 1.5 s to switch profile.
const MaxSilence = 10 * time.Second

var (
	ErrNoRecorder       = errors.New("no recorder")
	ErrNoMicrophone     = errors.New("no microphone found")
	ErrMicrophoneDenied = errors.New("microphone access denied")
	ErrMicrophone       = errors.New("could not open the microphone")
	ErrNoSound          = errors.New("no sound from the microphone")
)

// Recorder captures 16 kHz mono from the default microphone until it stops, and releases the microphone then.
type Recorder interface {
	// Level is how loud the last 50 ms were, from 0 to 1, as the square root of the peak, like the web's meter.
	Level() float64
	Elapsed() time.Duration
	// Err reports a recorder that stopped by itself, such as a microphone taken away or refused, or one that stays silent.
	Err() error
	Stop() []float32
	Cancel()
}

// StartRecording opens the microphone through the platform's recorder.
func StartRecording() (Recorder, error) { return startRecording() }

// takes collects samples from a backend's goroutine, capped at MaxRecording.
type takes struct {
	mu      sync.Mutex
	samples []float32
	waited  int
}

func (t *takes) add(samples []float32) {
	t.mu.Lock()
	defer t.mu.Unlock()
	// A microphone that is still waking up, like a Bluetooth headset switching profile, sends exact zeros.
	if len(t.samples) == 0 {
		live := slices.IndexFunc(samples, func(s float32) bool { return s != 0 })
		if live < 0 {
			t.waited += len(samples)
			return
		}
		t.waited += live
		samples = samples[live:]
	}
	room := int(MaxRecording/time.Second)*RecordRate - len(t.samples)
	t.samples = append(t.samples, samples[:max(0, min(room, len(samples)))]...)
}

func (t *takes) level() float64 {
	t.mu.Lock()
	defer t.mu.Unlock()
	var peak float64
	for _, sample := range t.samples[max(0, len(t.samples)-RecordRate/20):] {
		peak = max(peak, math.Abs(float64(sample)))
	}
	return math.Sqrt(min(peak, 1))
}

func (t *takes) elapsed() time.Duration {
	t.mu.Lock()
	defer t.mu.Unlock()
	return time.Duration(len(t.samples)) * time.Second / RecordRate
}

// err reports a microphone that sent only exact zeros for MaxSilence, so it never woke up.
func (t *takes) err() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if len(t.samples) == 0 && t.waited >= int(MaxSilence/time.Second)*RecordRate {
		return ErrNoSound
	}
	return nil
}

func (t *takes) all() []float32 {
	t.mu.Lock()
	defer t.mu.Unlock()
	return append([]float32(nil), t.samples...)
}
