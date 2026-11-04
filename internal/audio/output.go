package audio

import (
	"errors"
	"time"
)

// ErrUnavailable means this system has no sound server or device the client can use.
var ErrUnavailable = errors.New("sound output unavailable")

// Device plays one stream at a time. Play replaces whatever was playing and returns once the new stream started.
type Device interface {
	// Rate is the sample rate streams must use, or 0 when the device takes each stream's own.
	Rate() int
	// Buffered is how much output the device has taken but not played yet.
	Buffered() time.Duration
	Play(s *Stream) error
	Stop()
	Close()
}

// OpenDevice connects to the platform's sound output, or returns ErrUnavailable.
func OpenDevice() (Device, error) { return openDevice() }
