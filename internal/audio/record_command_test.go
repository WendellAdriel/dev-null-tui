package audio

import (
	"errors"
	"slices"
	"testing"
)

func lookFor(installed ...string) func(string) (string, error) {
	return func(tool string) (string, error) {
		if slices.Contains(installed, tool) {
			return "/bin/" + tool, nil
		}
		return "", errors.New("not found")
	}
}

const dshowListing = `[dshow @ 0001] "Integrated Camera" (video)
[dshow @ 0001] "Microphone Array (Realtek Audio)" (audio)
[dshow @ 0001] "Stereo Mix" (audio)`

func TestRecordCommandPrefersFFmpegThenSox(t *testing.T) {
	list := func(string) string { return dshowListing }
	tail := []string{"-ac", "1", "-ar", "16000", "-f", "f32le", "pipe:1"}
	for _, c := range []struct {
		goos      string
		installed []string
		want      []string
	}{
		{"darwin", []string{"ffmpeg", "sox"}, append([]string{"/bin/ffmpeg", "-hide_banner", "-loglevel", "error", "-nostdin", "-f", "avfoundation", "-i", ":0"}, tail...)},
		{"windows", []string{"ffmpeg"}, append([]string{"/bin/ffmpeg", "-hide_banner", "-loglevel", "error", "-nostdin", "-f", "dshow", "-i", "audio=Microphone Array (Realtek Audio)"}, tail...)},
		{"darwin", []string{"sox"}, []string{"/bin/sox", "-q", "-d", "-t", "raw", "-r", "16000", "-c", "1", "-e", "floating-point", "-b", "32", "-L", "-"}},
		{"windows", []string{"sox"}, []string{"/bin/sox", "-q", "-d", "-t", "raw", "-r", "16000", "-c", "1", "-e", "floating-point", "-b", "32", "-L", "-"}},
	} {
		got, err := recordCommand(c.goos, lookFor(c.installed...), list)
		if err != nil || !slices.Equal(got, c.want) {
			t.Errorf("%s with %v: %q, %v", c.goos, c.installed, got, err)
		}
	}
}

func TestRecordCommandNeedsATool(t *testing.T) {
	if _, err := recordCommand("darwin", lookFor(), nil); !errors.Is(err, ErrNoRecorder) {
		t.Fatalf("no ffmpeg or sox gives %v, want ErrNoRecorder", err)
	}
	// ffmpeg that lists no microphone on Windows still leaves sox to try.
	got, err := recordCommand("windows", lookFor("ffmpeg", "sox"), func(string) string { return "" })
	if err != nil || got[0] != "/bin/sox" {
		t.Fatalf("an ffmpeg without microphones falls back to sox: %q, %v", got, err)
	}
	if _, err := recordCommand("windows", lookFor("ffmpeg"), func(string) string { return "" }); !errors.Is(err, ErrNoMicrophone) {
		t.Fatalf("ffmpeg alone without microphones gives %v, want ErrNoMicrophone", err)
	}
}
