// Package audio decodes, plays, and records Remotes voice notes. Audio stays in memory.
package audio

import (
	"bytes"
	"errors"
	"time"

	"github.com/WendellAdriel/dev-null-cli/internal/api"
)

const (
	// MaxUploadBytes and Bars are the server's voice note limits, defined once in the API contract.
	MaxUploadBytes = api.MaxVoiceBytes
	Bars           = api.VoiceBars
	MinDuration    = 500 * time.Millisecond
	MaxDuration    = 61 * time.Second
	// RecordRate is the sample rate of the 16-bit mono WAV the client records and uploads.
	RecordRate = 16000
)

// Audio is mono PCM from -1 to 1.
type Audio struct {
	Samples []float32
	Rate    int
}

func (a Audio) Duration() time.Duration {
	if a.Rate == 0 {
		return 0
	}
	return time.Duration(len(a.Samples)) * time.Second / time.Duration(a.Rate)
}

var errNotVoice = errors.New("not a voice note")

// Decode reads a voice note as the server serves it: 16 kHz mono WAV, or Opus in MP4.
func Decode(data []byte) (Audio, error) {
	if len(data) > MaxUploadBytes {
		return Audio{}, errNotVoice
	}
	if bytes.HasPrefix(data, []byte("RIFF")) {
		return readWAV(data)
	}
	track, err := readMP4(data)
	if err != nil {
		return Audio{}, err
	}
	return track.decode()
}
