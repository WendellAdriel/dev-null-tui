//go:build darwin || windows

package audio

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"io"
	"math"
	"os/exec"
	"regexp"
	"runtime"
	"sync/atomic"
	"time"
)

// toolRecorder reads 16 kHz mono float samples from ffmpeg's or sox's stdout, so nothing is written to disk.
type toolRecorder struct {
	takes
	command  *exec.Cmd
	stderr   *bytes.Buffer
	done     chan struct{}
	released atomic.Bool
}

func startRecording() (Recorder, error) {
	args, err := recordCommand(runtime.GOOS, exec.LookPath, listDshowDevices)
	if err != nil {
		return nil, err
	}
	// #nosec G204: recordCommand builds only ffmpeg or sox with fixed arguments and no shell.
	command := exec.Command(args[0], args[1:]...)
	stdout, err := command.StdoutPipe()
	if err != nil {
		return nil, ErrMicrophone
	}
	stderr := &bytes.Buffer{}
	command.Stderr = stderr
	if err := command.Start(); err != nil {
		return nil, ErrMicrophone
	}
	r := &toolRecorder{command: command, stderr: stderr, done: make(chan struct{})}
	go r.read(stdout)
	return r, nil
}

func (r *toolRecorder) read(stdout io.Reader) {
	defer close(r.done)
	reader := bufio.NewReader(stdout)
	frame := make([]byte, 4*RecordRate/50)
	samples := make([]float32, len(frame)/4)
	for {
		n, err := io.ReadFull(reader, frame)
		for i := range n / 4 {
			samples[i] = math.Float32frombits(binary.LittleEndian.Uint32(frame[4*i:]))
		}
		r.add(samples[:n/4])
		if err != nil {
			return
		}
	}
}

func (r *toolRecorder) Level() float64         { return r.level() }
func (r *toolRecorder) Elapsed() time.Duration { return r.elapsed() }

// Err reports the tool exiting before it was stopped. A refused microphone gets its own error, as on the web.
func (r *toolRecorder) Err() error {
	select {
	case <-r.done:
	default:
		return r.err()
	}
	if r.released.Swap(true) {
		return nil
	}
	// Wait finishes copying stderr, so the message can be read after it.
	_ = r.command.Wait()
	if denied.MatchString(r.stderr.String()) {
		return ErrMicrophoneDenied
	}
	return ErrMicrophone
}

var denied = regexp.MustCompile(`(?i)denied|not authorized|permission`)

func (r *toolRecorder) Stop() []float32 {
	r.release()
	return r.all()
}

func (r *toolRecorder) Cancel() { r.release() }

func (r *toolRecorder) release() {
	if r.released.Swap(true) {
		return
	}
	_ = r.command.Process.Kill()
	<-r.done
	_ = r.command.Wait()
}

// listDshowDevices is ffmpeg's DirectShow device list, which it prints to stderr.
func listDshowDevices(ffmpeg string) string {
	var listing bytes.Buffer
	list := exec.Command(ffmpeg, "-hide_banner", "-list_devices", "true", "-f", "dshow", "-i", "dummy")
	list.Stderr = &listing
	_ = list.Run()
	return listing.String()
}
