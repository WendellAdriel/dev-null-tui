package audio

import (
	"bytes"
	"encoding/binary"
	"math"
	"os"
	"slices"
	"testing"
)

func readTone(t *testing.T) []byte {
	t.Helper()
	data, err := os.ReadFile("testdata/tone.mp4")
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// toneFrequency counts upward zero crossings, which is enough to tell a 440 Hz tone apart from noise or a wrong rate.
func toneFrequency(samples []float32, rate int) float64 {
	crossings := 0
	for i := 1; i < len(samples); i++ {
		if samples[i-1] < 0 && samples[i] >= 0 {
			crossings++
		}
	}
	return float64(crossings) * float64(rate) / float64(len(samples))
}

func sine(frequency float64, rate, count int, level float32) []float32 {
	samples := make([]float32, count)
	for i := range samples {
		samples[i] = level * float32(math.Sin(2*math.Pi*frequency*float64(i)/float64(rate)))
	}
	return samples
}

func TestDecodeReadsTheWAVTheServerStores(t *testing.T) {
	want := sine(440, 16000, 16000, 0.5)
	got, err := Decode(EncodeWAV(want))
	if err != nil {
		t.Fatal(err)
	}
	if got.Rate != 16000 || len(got.Samples) != len(want) {
		t.Fatalf("decoded %d samples at %d Hz, want %d at 16000", len(got.Samples), got.Rate, len(want))
	}
	for i := range want {
		// The web encodes with 0x7fff and decoding divides by 32768, so a sample can move by up to two steps.
		if math.Abs(float64(got.Samples[i]-want[i])) > 2.0/32768 {
			t.Fatalf("sample %d is %v, want %v", i, got.Samples[i], want[i])
		}
	}
}

func TestDecodeReadsOpusInMP4WithoutThePreSkip(t *testing.T) {
	got, err := Decode(readTone(t))
	if err != nil {
		t.Fatal(err)
	}
	if got.Rate != 48000 {
		t.Fatalf("rate is %d, want 48000", got.Rate)
	}
	// The fixture is a 2 s tone. Opus frames are 20 ms, so the decoded length may run one frame long.
	if seconds := float64(len(got.Samples)) / 48000; seconds < 2 || seconds > 2.02 {
		t.Fatalf("decoded %.3f s, want 2 s", seconds)
	}
	if f := toneFrequency(got.Samples[4800:], got.Rate); math.Abs(f-440) > 5 {
		t.Fatalf("decoded tone is %.1f Hz, want 440", f)
	}
	// A decoder that depends on earlier calls gave different results the second time, so decode again and compare.
	again, err := Decode(readTone(t))
	if err != nil || !slices.Equal(again.Samples, got.Samples) {
		t.Fatalf("a second decode differs: %v", err)
	}
}

func TestDecodeReadsTheServersSingleChunkLayout(t *testing.T) {
	ffmpeg := readTone(t)
	track, err := readMP4(ffmpeg)
	if err != nil {
		t.Fatal(err)
	}
	rebuilt := serverMP4(track.channels, track.preSkip, track.packets)

	want, err := Decode(ffmpeg)
	if err != nil {
		t.Fatal(err)
	}
	got, err := Decode(rebuilt)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(got.Samples, want.Samples) {
		t.Fatalf("the server's layout decoded to %d samples, ffmpeg's to %d", len(got.Samples), len(want.Samples))
	}
}

func TestDecodeRefusesBrokenOrUnexpectedAudio(t *testing.T) {
	tone := readTone(t)
	stereo := EncodeWAV(sine(440, 16000, 8000, 0.5))
	binary.LittleEndian.PutUint16(stereo[22:], 2)
	fast := EncodeWAV(sine(440, 16000, 8000, 0.5))
	binary.LittleEndian.PutUint32(fast[24:], 44100)
	noOpus := bytes.Replace(bytes.Clone(tone), []byte("dOps"), []byte("dOpX"), 1)
	huge := bytes.Clone(tone)
	if at := bytes.Index(huge, []byte("stsz")); at > 0 {
		binary.BigEndian.PutUint32(huge[at+12:], math.MaxUint32)
	}

	cases := map[string][]byte{
		"empty":           nil,
		"truncated mp4":   tone[:len(tone)/2],
		"no dOps":         noOpus,
		"huge stsz count": huge,
		"stereo wav":      stereo,
		"44.1 kHz wav":    fast,
		"text":            []byte("not audio at all"),
		"oversized":       make([]byte, MaxUploadBytes+1),
	}
	for name, data := range cases {
		if _, err := Decode(data); err == nil {
			t.Errorf("%s decoded without an error", name)
		}
	}
}

func TestEncodeWAVPassesTheServersFormatCheck(t *testing.T) {
	wav := EncodeWAV([]float32{0, 1, -1, 2, -2})
	// VoiceNotes::wavFormat is pack('vvVVvv', 1, 1, 16000, 32000, 2, 16).
	format := []byte{1, 0, 1, 0, 0x80, 0x3e, 0, 0, 0x00, 0x7d, 0, 0, 2, 0, 16, 0}
	if len(wav) != 44+10 || string(wav[:4]) != "RIFF" || string(wav[8:16]) != "WAVEfmt " || !bytes.Equal(wav[20:36], format) || string(wav[36:40]) != "data" {
		t.Fatalf("header is % x", wav[:44])
	}
	if size := binary.LittleEndian.Uint32(wav[40:]); size != 10 {
		t.Fatalf("data size is %d, want 10", size)
	}
	got := make([]int16, 5)
	if _, err := binary.Decode(wav[44:], binary.LittleEndian, got); err != nil {
		t.Fatal(err)
	}
	if want := []int16{0, 32767, -32768, 32767, -32768}; !slices.Equal(got, want) {
		t.Fatalf("samples are %v, want %v (clamped like the web)", got, want)
	}
}

func TestWaveformMatchesTheWeb(t *testing.T) {
	samples := make([]float32, 4800)
	for i := range samples {
		switch {
		case i < 100:
			samples[i] = 0
		case i < 200:
			samples[i] = -0.25
		default:
			samples[i] = 1.5
		}
	}
	bars := Waveform(samples)
	if len(bars) != Bars {
		t.Fatalf("%d bars, want %d", len(bars), Bars)
	}
	// round(sqrt(min(peak, 1)) * 100): silence 0, a quarter 50, past full scale 100.
	if bars[0] != 0 || bars[1] != 50 || bars[2] != 100 || bars[47] != 100 {
		t.Fatalf("bars are %v", bars)
	}
}
