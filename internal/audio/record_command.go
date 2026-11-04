package audio

import "regexp"

var dshowAudio = regexp.MustCompile(`"([^"]+)"\s+\(audio\)`)

// recordCommand is the command that writes 16 kHz mono float samples from the default microphone to stdout: ffmpeg
// when installed and it finds a microphone, else sox, whose -d already means the system's default input.
func recordCommand(goos string, lookPath func(string) (string, error), listDevices func(ffmpeg string) string) ([]string, error) {
	err := ErrNoRecorder
	if ffmpeg, found := lookPath("ffmpeg"); found == nil {
		input, inputErr := ffmpegInput(goos, ffmpeg, listDevices)
		if inputErr == nil {
			command := append([]string{ffmpeg, "-hide_banner", "-loglevel", "error", "-nostdin"}, input...)
			return append(command, "-ac", "1", "-ar", "16000", "-f", "f32le", "pipe:1"), nil
		}
		err = inputErr
	}
	if sox, found := lookPath("sox"); found == nil {
		return []string{sox, "-q", "-d", "-t", "raw", "-r", "16000", "-c", "1", "-e", "floating-point", "-b", "32", "-L", "-"}, nil
	}
	return nil, err
}

// ffmpegInput picks AVFoundation's first audio device on macOS, or DirectShow's first listed microphone on Windows.
func ffmpegInput(goos, ffmpeg string, listDevices func(string) string) ([]string, error) {
	if goos == "darwin" {
		return []string{"-f", "avfoundation", "-i", ":0"}, nil
	}
	device := dshowAudio.FindStringSubmatch(listDevices(ffmpeg))
	if device == nil {
		return nil, ErrNoMicrophone
	}
	return []string{"-f", "dshow", "-i", "audio=" + device[1]}, nil
}
