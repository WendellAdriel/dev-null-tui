package audio

import (
	"math"
	"testing"
	"time"
)

// drain pulls a stream the way a sound server does, in small buffers, until it reports the end.
func drain(s *Stream, limit int) []float32 {
	var played []float32
	buffer := make([]float32, 100)
	for len(played) < limit {
		n, done := s.Read(buffer)
		played = append(played, buffer[:n]...)
		if done {
			break
		}
	}
	return played
}

func ramp(count int) Audio {
	samples := make([]float32, count)
	for i := range samples {
		samples[i] = float32(i) / float32(count)
	}
	return Audio{Samples: samples, Rate: 1000}
}

func TestStreamPlaysFromAnOffsetToTheEnd(t *testing.T) {
	s := NewStream(ramp(2000), StreamOptions{From: 500 * time.Millisecond, Speed: 1, Volume: 1})
	played := drain(s, 10000)
	if len(played) != 1500 || played[0] != 0.25 {
		t.Fatalf("played %d samples from %v, want 1500 from 0.25", len(played), played[0])
	}
	if !s.Finished(0) || s.Position() != 2*time.Second {
		t.Fatalf("finished %v at %v, want finished at 2s", s.Finished(0), s.Position())
	}
}

func TestStreamPositionFollowsTheNoteAtAnySpeed(t *testing.T) {
	s := NewStream(ramp(4000), StreamOptions{From: time.Second, Speed: 2, Volume: 1})
	drain(s, 500)
	// 500 samples at 2× cover a second of the note, so it is two seconds in.
	if got := s.Position(); got != 2*time.Second {
		t.Fatalf("position is %v, want 2s", got)
	}
}

func TestStreamAppliesVolumeLive(t *testing.T) {
	audio := Audio{Samples: []float32{1, 1, 1, 1}, Rate: 1000}
	s := NewStream(audio, StreamOptions{Speed: 1, Volume: 0.5})
	first := make([]float32, 2)
	s.Read(first)
	s.SetVolume(0.2)
	second := make([]float32, 2)
	s.Read(second)
	if first[0] != 0.5 || math.Abs(float64(second[0])-0.2) > 1e-6 {
		t.Fatalf("volumes played %v then %v", first, second)
	}
}

func TestStreamResamplesForAFixedRateDevice(t *testing.T) {
	s := NewStream(Audio{Samples: make([]float32, 16000), Rate: 16000}, StreamOptions{Speed: 1, Volume: 1, Rate: 48000})
	if played := drain(s, 100000); len(played) != 48000 || s.Position() != time.Second {
		t.Fatalf("played %d samples to %v, want 48000 to 1s", len(played), s.Position())
	}
}

func TestStreamClampsAnOffsetPastTheEnd(t *testing.T) {
	s := NewStream(ramp(1000), StreamOptions{From: 5 * time.Second, Speed: 1, Volume: 1})
	if n, done := s.Read(make([]float32, 10)); n != 0 || !done {
		t.Fatalf("read %d, done %v past the end", n, done)
	}
}

func TestStreamPadsSilenceAfterTheEndSoBackendsNeverStall(t *testing.T) {
	s := NewStream(Audio{Samples: []float32{1, 1, 1}, Rate: 1000}, StreamOptions{Speed: 1, Volume: 1})
	out := []float32{9, 9, 9, 9, 9}
	n, done := s.Read(out)
	if n != 3 || !done || out[3] != 0 || out[4] != 0 {
		t.Fatalf("read %d, done %v, out %v: the rest must be silence", n, done, out)
	}
	empty := NewStream(Audio{Samples: []float32{1}, Rate: 1000}, StreamOptions{From: time.Second, Speed: 1, Volume: 1})
	if n, done := empty.Read(out); n != 0 || !done || out[0] != 0 {
		t.Fatalf("an empty stream reads %d, done %v, out %v", n, done, out)
	}
}

func TestStreamCountsOnlyWhatTheDeviceHasPlayed(t *testing.T) {
	clock := time.Unix(0, 0)
	s := NewStream(ramp(2000), StreamOptions{Speed: 2, Volume: 1})
	s.now = func() time.Time { return clock }
	drain(s, 400)
	// 400 samples at 2× are 0.8 s of note, but 0.1 s of output still waits in the device: 0.2 s of note at 2×.
	if got := s.Heard(100 * time.Millisecond); got != 600*time.Millisecond {
		t.Fatalf("heard %v, want 600ms", got)
	}
	drain(s, 10000)
	if s.Finished(100*time.Millisecond) || s.Heard(100*time.Millisecond) != 1800*time.Millisecond {
		t.Fatalf("right after the last read, the buffer still plays: finished %v at %v", s.Finished(100*time.Millisecond), s.Heard(100*time.Millisecond))
	}
	clock = clock.Add(100 * time.Millisecond)
	if !s.Finished(100*time.Millisecond) || s.Heard(100*time.Millisecond) != 2*time.Second {
		t.Fatalf("once the buffer drains the note has ended: finished %v at %v", s.Finished(100*time.Millisecond), s.Heard(100*time.Millisecond))
	}
}
