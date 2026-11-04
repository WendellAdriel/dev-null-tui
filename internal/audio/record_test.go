package audio

import (
	"errors"
	"slices"
	"testing"
	"time"
)

func TestRecordingStartsWhenTheMicrophoneIsLive(t *testing.T) {
	var takes takes
	// PipeWire sends exact zeros while a Bluetooth headset switches to its microphone.
	takes.add(make([]float32, RecordRate))
	if takes.elapsed() != 0 {
		t.Fatalf("digital silence before the microphone is live counts as %v", takes.elapsed())
	}
	takes.add([]float32{0, 0, 0.01, 0, -0.02})
	takes.add([]float32{0, 0})
	if got, want := takes.all(), []float32{0.01, 0, -0.02, 0, 0}; !slices.Equal(got, want) {
		t.Fatalf("kept %v, want %v: once live, every sample counts", got, want)
	}
	if takes.err() != nil {
		t.Fatalf("a live microphone is fine, got %v", takes.err())
	}
}

func TestAMicrophoneThatNeverSendsSoundFails(t *testing.T) {
	var takes takes
	for range int(MaxSilence/time.Second) - 1 {
		takes.add(make([]float32, RecordRate))
	}
	if takes.err() != nil {
		t.Fatalf("a microphone still waking up is not an error yet: %v", takes.err())
	}
	takes.add(make([]float32, RecordRate))
	if !errors.Is(takes.err(), ErrNoSound) {
		t.Fatalf("after %v of exact zeros, got %v, want ErrNoSound", MaxSilence, takes.err())
	}
}
