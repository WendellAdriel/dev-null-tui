package audio

import (
	"math"
	"slices"
	"testing"
)

func TestStretchKeepsPitchWhileShortening(t *testing.T) {
	for _, rate := range []int{16000, 48000} {
		tone := sine(440, rate, 2*rate, 0.5)
		for _, speed := range []float64{1.5, 2} {
			got := Stretch(tone, rate, speed)
			want := float64(len(tone)) / speed
			if math.Abs(float64(len(got))-want) > want/100 {
				t.Fatalf("%d Hz at %v× is %d samples, want about %.0f", rate, speed, len(got), want)
			}
			// Skip the edges, where the first and last windows fade in and out.
			edge := rate / 20
			if f := toneFrequency(got[edge:len(got)-edge], rate); math.Abs(f-440) > 440*0.02 {
				t.Fatalf("%d Hz at %v× plays at %.1f Hz, want 440", rate, speed, f)
			}
		}
	}
}

func TestStretchLeavesNormalSpeedAlone(t *testing.T) {
	tone := sine(440, 16000, 16000, 0.5)
	if got := Stretch(tone, 16000, 1); !slices.Equal(got, tone) {
		t.Fatal("1× changed the samples")
	}
}
