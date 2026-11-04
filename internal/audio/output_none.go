//go:build !linux && !darwin && !windows

package audio

func openDevice() (Device, error) { return nil, ErrUnavailable }
