//go:build !linux && !darwin && !windows

package audio

func startRecording() (Recorder, error) { return nil, ErrNoRecorder }
