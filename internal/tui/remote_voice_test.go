package tui

import (
	"context"
	"errors"
	"fmt"
	"math"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/WendellAdriel/dev-null-cli/internal/api"
	"github.com/WendellAdriel/dev-null-cli/internal/audio"
	"github.com/WendellAdriel/dev-null-cli/internal/auth"
)

// fakeSound records what the TUI asks the sound device to do, without playing anything.
type fakeSound struct {
	plays    []*audio.Stream
	stops    int
	closed   bool
	refused  bool
	buffered time.Duration
}

func (f *fakeSound) Rate() int               { return 0 }
func (f *fakeSound) Buffered() time.Duration { return f.buffered }
func (f *fakeSound) Play(s *audio.Stream) error {
	f.plays = append(f.plays, s)
	return nil
}
func (f *fakeSound) Stop()  { f.stops++ }
func (f *fakeSound) Close() { f.closed = true }

func (f *fakeSound) playing() *audio.Stream {
	if len(f.plays) == 0 {
		return nil
	}
	return f.plays[len(f.plays)-1]
}

type voiceFixture struct {
	sound     *fakeSound
	downloads []string
	failNext  bool
}

func voiceNote(id int64, seconds int) api.Message {
	note := remoteMessage(id, "grace", "", time.Date(2026, 10, 23, 11, 0, 0, 0, time.UTC))
	note.User = &api.User{ID: "grace", Handle: "grace", Name: "Grace Hopper"}
	waveform := make([]int, audio.Bars)
	for i := range waveform {
		waveform[i] = (i * 7) % 101
	}
	note.Voice = &api.Voice{ID: fmt.Sprint("voice-", id), DurationMS: seconds * 1000, Waveform: waveform, URL: fmt.Sprintf("https://example.test/api/v1/remotes/voices/%d", id)}
	return note
}

// voiceApp opens the org thread with two voice notes from Grace, 10 s and 3 s long, and selects the first.
func voiceApp(t *testing.T) (AppModel, *voiceFixture) {
	t.Helper()
	m, _ := messagingApp(t)
	f := &voiceFixture{sound: &fakeSound{}}
	m.voice.openSound = func() (audio.Device, error) {
		if f.sound.refused {
			return nil, audio.ErrUnavailable
		}
		return f.sound, nil
	}
	m.voice.download = func(_ context.Context, url string) ([]byte, error) {
		f.downloads = append(f.downloads, url)
		if f.failNext {
			f.failNext = false
			return nil, errors.New("media unavailable")
		}
		seconds := 10
		if strings.HasSuffix(url, "/41") {
			seconds = 3
		}
		samples := make([]float32, seconds*audio.RecordRate)
		for i := range samples {
			samples[i] = float32(0.3 * math.Sin(float64(i)/10))
		}
		return audio.EncodeWAV(samples), nil
	}
	m = openOrg(t, m)
	thread := m.remotes.thread
	thread.messages = append(thread.messages, voiceNote(40, 10), voiceNote(41, 3))
	thread.selected = "m:40"
	m.renderRemoteThread()
	return m, f
}

func voiceKeys(t *testing.T, m AppModel, keys ...string) AppModel {
	t.Helper()
	return settle(t, m, keys...)
}

func TestVoiceNoteRowShowsClockAndWaveform(t *testing.T) {
	m, _ := voiceApp(t)
	text := threadText(m)
	if !strings.Contains(text, "▶") || !strings.Contains(text, "0:10") || !strings.Contains(text, "0:03") {
		t.Fatalf("voice notes need a play glyph and their lengths:\n%s", text)
	}
	if !strings.ContainsAny(text, "▁▂▃▄▅▆▇█") {
		t.Fatalf("voice notes need a waveform:\n%s", text)
	}
	for _, size := range [][2]int{{160, 45}, {60, 18}} {
		next, _ := m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		narrow := next.(AppModel)
		narrow.renderRemoteThread()
		pane := narrow.threadPaneWidth()
		for _, line := range strings.Split(narrow.remoteThreadPane(pane, narrow.mainHeight()-remotesHeaderHeight, colors(true)), "\n") {
			if w := ansi.StringWidth(line); w > pane {
				t.Fatalf("at %dx%d a line is %d cells, wider than the %d-cell pane: %q", size[0], size[1], w, pane, ansi.Strip(line))
			}
		}
	}
}

func TestSpacePlaysPausesAndResumesWithOneDownload(t *testing.T) {
	m, f := voiceApp(t)
	m = voiceKeys(t, m, "space")
	if len(f.downloads) != 1 || len(f.sound.plays) != 1 {
		t.Fatalf("first space: %d downloads, %d plays", len(f.downloads), len(f.sound.plays))
	}
	if !strings.Contains(threadText(m), "‖") {
		t.Fatalf("a playing note shows a pause glyph:\n%s", threadText(m))
	}
	m = voiceKeys(t, m, "space")
	if f.sound.stops == 0 || m.voice.playback.state != "paused" {
		t.Fatalf("second space must pause: stops %d, state %q", f.sound.stops, m.voice.playback.state)
	}
	m = voiceKeys(t, m, "space")
	if len(f.downloads) != 1 || len(f.sound.plays) != 2 || m.voice.playback.state != "playing" {
		t.Fatalf("third space must resume without downloading: %d downloads, %d plays, %q", len(f.downloads), len(f.sound.plays), m.voice.playback.state)
	}
}

func TestPlayingAnotherNoteStopsTheFirst(t *testing.T) {
	m, f := voiceApp(t)
	m = voiceKeys(t, m, "space")
	m.remotes.thread.selected = "m:41"
	m = voiceKeys(t, m, "space")
	if m.voice.playback.messageID != 41 || len(f.sound.plays) != 2 || f.sound.stops == 0 {
		t.Fatalf("the second note must replace the first: playing %d, %d plays, %d stops", m.voice.playback.messageID, len(f.sound.plays), f.sound.stops)
	}
}

func TestLeavingTheThreadStopsPlaybackAndDropsTheAudio(t *testing.T) {
	m, f := voiceApp(t)
	m = voiceKeys(t, m, "space")
	stops := f.sound.stops
	m = voiceKeys(t, m, "esc")
	if f.sound.stops == stops || m.voice.playback.messageID != 0 || len(m.voice.playback.audio.Samples) != 0 {
		t.Fatalf("closing the thread must stop and forget the note: stops %d→%d, playback %+v", stops, f.sound.stops, m.voice.playback.messageID)
	}
	m, f = voiceApp(t)
	m = voiceKeys(t, m, "space")
	m.leaveApplication()
	if m.voice.playback.messageID != 0 || !f.sound.closed {
		t.Fatal("signing out must stop playback and close the device")
	}
}

func TestSeekSpeedAndVolumeKeys(t *testing.T) {
	m, f := voiceApp(t)
	m = voiceKeys(t, m, "space", "right")
	if got := f.sound.playing().Position(); got != 2*time.Second {
		t.Fatalf("right seeks to %v, want 2s", got)
	}
	m = voiceKeys(t, m, "left", "left")
	if got := f.sound.playing().Position(); got != 0 {
		t.Fatalf("left past the start seeks to %v, want 0", got)
	}
	m = voiceKeys(t, m, "right", "right", "right")
	if got := f.sound.playing().Position(); got != 6*time.Second {
		t.Fatalf("three rights from 0 seek to %v, want 6s", got)
	}
	m = voiceKeys(t, m, "x")
	if m.voice.playback.speed != 1.5 || !strings.Contains(threadText(m), "1.5×") {
		t.Fatalf("x must switch to 1.5×: %v", m.voice.playback.speed)
	}
	if got := f.sound.playing().Position(); got != 6*time.Second {
		t.Fatalf("changing speed keeps the position: %v", got)
	}
	m = voiceKeys(t, m, "x", "x")
	if m.voice.playback.speed != 1 {
		t.Fatalf("x cycles back to 1×: %v", m.voice.playback.speed)
	}
	m = voiceKeys(t, m, "-", "-")
	if math.Abs(m.tuiSettings.VoiceVolume-0.8) > 1e-9 {
		t.Fatalf("two presses of - give %v, want 0.8", m.tuiSettings.VoiceVolume)
	}
	saveVolume := func(m AppModel, generation int) AppModel {
		next, cmd := m.update(volumeSaveTick{m.session, generation})
		return settleCmd(next.(AppModel), cmd)
	}
	m = saveVolume(m, m.voice.volumeGeneration)
	if !savedSettings(m, func(s auth.TUISettings) bool { return math.Abs(s.VoiceVolume-0.8) <= 1e-9 }) {
		t.Fatalf("the settled volume is saved as %v, want 0.8", m.account.store.TUISettings().VoiceVolume)
	}
	m = voiceKeys(t, m, "+", "+", "+", "+")
	m = saveVolume(m, m.voice.volumeGeneration-1)
	if got := m.account.store.TUISettings().VoiceVolume; math.Abs(got-0.8) > 1e-9 {
		t.Fatalf("a superseded save tick wrote %v", got)
	}
	m = saveVolume(m, m.voice.volumeGeneration)
	if m.tuiSettings.VoiceVolume != 1 || !savedSettings(m, func(s auth.TUISettings) bool { return s.VoiceVolume == 1 }) {
		t.Fatalf("volume stops at 1 and is saved: %v, saved %v", m.tuiSettings.VoiceVolume, m.account.store.TUISettings().VoiceVolume)
	}
}

func TestVoiceNoteClockFollowsTheSpeed(t *testing.T) {
	m, _ := voiceApp(t)
	m = voiceKeys(t, m, "space", "x", "x")
	// 10 s at 2× reads 0:05, as the web divides the clock by the speed.
	if !strings.Contains(threadText(m), "0:05") {
		t.Fatalf("at 2× the 10 s note reads 0:05:\n%s", threadText(m))
	}
}

func TestFailedDownloadOffersRetryWithoutAToast(t *testing.T) {
	m, f := voiceApp(t)
	f.failNext = true
	m = voiceKeys(t, m, "space")
	if m.voice.playback.state != "failed" || !strings.Contains(threadText(m), "Retry") || m.toast.visible {
		t.Fatalf("a failed download shows Retry and no toast: %q, toast %v", m.voice.playback.state, m.toast.visible)
	}
	m = voiceKeys(t, m, "space")
	if len(f.downloads) != 2 || len(f.sound.plays) != 1 {
		t.Fatalf("space retries: %d downloads, %d plays", len(f.downloads), len(f.sound.plays))
	}
}

func TestLateDownloadForAClosedThreadIsDropped(t *testing.T) {
	m, f := voiceApp(t)
	next, cmd := m.handleKey("space")
	m = next
	m.closeRemoteThread()
	m = settleCmd(m, cmd)
	if len(f.sound.plays) != 0 {
		t.Fatal("a note downloaded after its thread closed must not play")
	}
}

func TestWithoutSoundTheNoteLinksToTheWeb(t *testing.T) {
	m, f := voiceApp(t)
	f.sound.refused = true
	var copied []string
	m.copyLink = func(_ context.Context, raw string) error { copied = append(copied, raw); return nil }
	m = voiceKeys(t, m, "space")
	if !strings.Contains(threadText(m), "Open on the web") || len(f.downloads) != 0 {
		t.Fatalf("without sound the note offers the web: downloads %d\n%s", len(f.downloads), threadText(m))
	}
	if len(copied) != 1 || copied[0] != "https://example.test/remotes/orgs/"+fixtureOrg {
		t.Fatalf("space copies the conversation's web link: %v", copied)
	}
}

func TestClickingThePlayGlyphTogglesAndTheWaveformSeeks(t *testing.T) {
	m, f := voiceApp(t)
	area, ok := m.voiceArea(40)
	if !ok {
		t.Fatal("the note has no clickable area")
	}
	m = settleCmd(m, m.threadClick(area.line, area.play))
	if len(f.sound.plays) != 1 {
		t.Fatal("clicking the play glyph plays")
	}
	m = settleCmd(m, m.threadClick(area.line, area.bars+area.width/2))
	if got := f.sound.playing().Position(); got < 4*time.Second || got > 6*time.Second {
		t.Fatalf("clicking the middle of the waveform seeks to %v, want about 5s", got)
	}
}

// fakeRecorder stands in for the microphone, recording a tone for as long as the test says.
type fakeRecorder struct {
	elapsed  time.Duration
	stopped  bool
	canceled bool
	failed   error
}

func (r *fakeRecorder) Err() error { return r.failed }

func (r *fakeRecorder) Level() float64         { return 0.5 }
func (r *fakeRecorder) Elapsed() time.Duration { return r.elapsed }
func (r *fakeRecorder) Cancel()                { r.canceled = true }
func (r *fakeRecorder) Stop() []float32 {
	r.stopped = true
	samples := make([]float32, int(r.elapsed.Seconds()*audio.RecordRate))
	for i := range samples {
		samples[i] = float32(0.25 * math.Sin(float64(i)/8))
	}
	return samples
}

func recordingApp(t *testing.T) (AppModel, *messaging, *fakeRecorder, *fakeSound) {
	t.Helper()
	m, state := messagingApp(t)
	recorder := &fakeRecorder{}
	sound := &fakeSound{}
	m.voice.record = func() (audio.Recorder, error) { return recorder, nil }
	m.voice.openSound = func() (audio.Device, error) { return sound, nil }
	m = openOrg(t, m)
	m = send(m, tea.KeyPressMsg{Code: 'w', Text: "w"})
	return m, state, recorder, sound
}

var ctrlT = tea.KeyPressMsg{Code: 't', Mod: tea.ModCtrl}

func composerText(m AppModel) string {
	view, _ := m.messageComposerView(m.threadPaneWidth(), colors(true))
	return ansi.Strip(view)
}

func recordTick(m AppModel) AppModel {
	next, cmd := m.update(recordingTick{session: m.session, generation: m.remotes.thread.composer.recording.generation})
	return settleCmd(next.(AppModel), cmd)
}

func TestCtrlTRecordsWithAClockAndStopsAtAMinute(t *testing.T) {
	m, _, recorder, _ := recordingApp(t)
	m = send(m, ctrlT)
	if m.remotes.thread.composer.recording == nil {
		t.Fatal("ctrl+t must start recording")
	}
	// A Bluetooth headset takes about a second to switch to its microphone.
	if text := composerText(m); !strings.Contains(text, "waiting for the microphone") || strings.Contains(text, "0:00/1:00") {
		t.Fatalf("before the microphone is live, the clock waits:\n%s", text)
	}
	m = recordTick(m)
	if levels := m.remotes.thread.composer.recording.levels; len(levels) != 0 {
		t.Fatalf("the meter waits with the clock, but has %d bars", len(levels))
	}
	recorder.elapsed = 12 * time.Second
	m = recordTick(m)
	if text := composerText(m); !strings.Contains(text, "0:12/1:00") || !strings.Contains(text, "enter stop") {
		t.Fatalf("recording shows its clock and keys:\n%s", text)
	}
	if m.upgradeVisible() && m.upgrade.open {
		t.Fatal("the update modal waits while recording")
	}
	recorder.elapsed = audio.MaxRecording
	m = recordTick(m)
	if !recorder.stopped || m.remotes.thread.composer.recording.samples == nil {
		t.Fatal("a recording stops by itself at a minute")
	}
	if text := composerText(m); !strings.Contains(text, "space listen") || !strings.Contains(text, "enter push") {
		t.Fatalf("a stopped recording waits for review:\n%s", text)
	}
}

func TestRecordingStopsOnEnterAndDiscardsOnEsc(t *testing.T) {
	m, _, recorder, _ := recordingApp(t)
	m = send(m, ctrlT)
	recorder.elapsed = 3 * time.Second
	m = send(m, escKey)
	if !recorder.canceled || m.remotes.thread.composer.recording != nil {
		t.Fatal("esc discards a recording in progress")
	}
	m = send(m, ctrlT)
	recorder.elapsed = 300 * time.Millisecond
	m = send(m, enterKey)
	if m.remotes.thread.composer.recording != nil || !strings.Contains(m.remotes.thread.composer.status, "shorter than half a second") {
		t.Fatalf("a short recording is dropped with the web's message: %q", m.remotes.thread.composer.status)
	}
}

func TestRecordingOverADraftAsksFirst(t *testing.T) {
	m, _, _, _ := recordingApp(t)
	m = send(m, typed("unsent words")...)
	m = send(m, ctrlT)
	if m.remotes.thread.composer.recording != nil || !strings.Contains(composerText(m), "replaces the draft") {
		t.Fatalf("recording over text asks first:\n%s", composerText(m))
	}
	m = send(m, escKey)
	if m.remotes.thread.composer.input.Value() != "unsent words" || m.remotes.thread.composer.voiceConfirm {
		t.Fatal("esc keeps the draft")
	}
	m = send(m, ctrlT, enterKey)
	if m.remotes.thread.composer.input.Value() != "" || m.remotes.thread.composer.recording == nil {
		t.Fatal("enter replaces the draft with a recording")
	}
}

func TestReviewedRecordingListensPushesAndRetriesTheSameBytes(t *testing.T) {
	m, state, recorder, sound := recordingApp(t)
	m.remotes.thread.composer.reply, m.remotes.thread.composer.replyID = &m.remotes.thread.messages[0], m.remotes.thread.messages[0].ID
	m = send(m, ctrlT)
	recorder.elapsed = 2 * time.Second
	m = send(m, enterKey)
	m = send(m, tea.KeyPressMsg{Code: tea.KeySpace, Text: " "})
	if len(sound.plays) != 1 {
		t.Fatal("space plays the recording back")
	}
	state.pushErr = errors.New("connection reset")
	m = send(m, enterKey)
	if len(state.pushes) != 1 {
		t.Fatalf("enter pushes once: %d", len(state.pushes))
	}
	first := state.pushes[0]
	if first.Voice == nil || first.Body != "" || len(first.Voice.Waveform) != audio.Bars || first.ReplyToID != m.remotes.thread.messages[0].ID {
		t.Fatalf("the push carries the voice note and reply only: %+v", first)
	}
	if decoded, err := audio.Decode(first.Voice.WAV); err != nil || decoded.Duration() != 2*time.Second {
		t.Fatalf("the voice is 2 s of WAV: %v %v", decoded.Duration(), err)
	}
	if m.remotes.thread.composer.recording != nil || sound.stops == 0 {
		t.Fatal("pushing clears the recording and stops its playback")
	}
	state.pushErr = nil
	m = send(m, escKey)
	m.remotes.thread.selected = "p:" + first.ClientID
	m = send(m, tea.KeyPressMsg{Code: 'r', Text: "r"})
	if len(state.pushes) != 2 || state.pushes[1].ClientID != first.ClientID || !slices.Equal(state.pushes[1].Voice.WAV, first.Voice.WAV) {
		t.Fatal("a retry resends the same client id and bytes")
	}
}

func TestStoppingBeforeTheMicrophoneSendsSound(t *testing.T) {
	m, _, _, _ := recordingApp(t)
	m = send(m, ctrlT, enterKey)
	if m.remotes.thread.composer.recording != nil || m.remotes.thread.composer.status != "fatal: no sound from the microphone. Is it muted?" {
		t.Fatalf("stopping while waiting shows %q", m.remotes.thread.composer.status)
	}
}

func TestDiscardingAReviewedRecording(t *testing.T) {
	m, state, recorder, _ := recordingApp(t)
	m = send(m, ctrlT)
	recorder.elapsed = time.Second
	m = send(m, enterKey, tea.KeyPressMsg{Code: 'd', Text: "d"})
	if m.remotes.thread.composer.recording != nil || len(state.pushes) != 0 {
		t.Fatal("d discards the recording without pushing")
	}
}

func TestRecordingErrorsUseTheWebsWords(t *testing.T) {
	for err, want := range map[error]string{
		audio.ErrNoRecorder:   "Voice notes need ffmpeg or sox. Record on the web.",
		audio.ErrNoMicrophone: "fatal: no microphone found",
		audio.ErrMicrophone:   "fatal: could not open the microphone",
		audio.ErrNoSound:      "fatal: no sound from the microphone. Is it muted?",
	} {
		m, _, _, _ := recordingApp(t)
		m.voice.record = func() (audio.Recorder, error) { return nil, err }
		m = send(m, ctrlT)
		if m.remotes.thread.composer.status != want || m.remotes.thread.composer.recording != nil {
			t.Fatalf("%v shows %q, want %q", err, m.remotes.thread.composer.status, want)
		}
	}
}

func TestClosingTheThreadReleasesTheMicrophone(t *testing.T) {
	m, _, recorder, _ := recordingApp(t)
	m = send(m, ctrlT)
	m.closeRemoteThread()
	if !recorder.canceled {
		t.Fatal("closing the thread must release the microphone")
	}
}

func TestSeekingToTheEndFinishesTheNoteWithoutPlaying(t *testing.T) {
	m, f := voiceApp(t)
	m = voiceKeys(t, m, "space", "right", "right", "right", "right")
	plays := len(f.sound.plays)
	m = voiceKeys(t, m, "right")
	// A stream with nothing left to play froze the TUI on Linux, so the end is handled without the device.
	if len(f.sound.plays) != plays || m.voice.playback.state != "paused" || m.voiceAt() != 0 {
		t.Fatalf("seeking to the end must rewind without playing: %d→%d plays, %q at %v", plays, len(f.sound.plays), m.voice.playback.state, m.voiceAt())
	}
}

func TestANoteEndsOnlyOnceTheDeviceHasPlayedItsBuffer(t *testing.T) {
	m, f := voiceApp(t)
	f.sound.buffered = time.Hour
	m.remotes.thread.selected = "m:41"
	m = voiceKeys(t, m, "space")
	stream := f.sound.playing()
	stream.Read(make([]float32, 10*audio.RecordRate))
	next, _ := m.update(voiceTick{m.session, m.voice.playback.generation})
	m = next.(AppModel)
	if m.voice.playback.state != "playing" {
		t.Fatal("a note whose end is still in the device buffer keeps playing")
	}
	f.sound.buffered = 0
	next, _ = m.update(voiceTick{m.session, m.voice.playback.generation})
	m = next.(AppModel)
	if m.voice.playback.state != "paused" || m.voiceAt() != 0 {
		t.Fatalf("once played out the note rewinds: %q at %v", m.voice.playback.state, m.voiceAt())
	}
}

func TestAFailedRecorderKeepsTheDraftItWasToReplace(t *testing.T) {
	m, _, _, _ := recordingApp(t)
	m.voice.record = func() (audio.Recorder, error) { return nil, audio.ErrNoRecorder }
	m = send(m, typed("keep me")...)
	m = send(m, ctrlT, enterKey)
	if got := m.remotes.thread.composer.input.Value(); got != "keep me" {
		t.Fatalf("the draft was replaced before the recorder opened: %q", got)
	}
}

func TestARecorderThatDiesEndsTheRecordingWithItsError(t *testing.T) {
	m, _, recorder, _ := recordingApp(t)
	m = send(m, ctrlT)
	recorder.failed = audio.ErrMicrophoneDenied
	m = recordTick(m)
	if m.remotes.thread.composer.recording != nil || !recorder.canceled || m.remotes.thread.composer.status != "fatal: microphone access denied" {
		t.Fatalf("a dead recorder must end the recording with the web's text: %q", m.remotes.thread.composer.status)
	}
}

func TestReviewShowsWhenThereIsNoSoundToListenWith(t *testing.T) {
	m, _, recorder, sound := recordingApp(t)
	sound.refused = true
	m.voice.openSound = func() (audio.Device, error) { return nil, audio.ErrUnavailable }
	m = send(m, ctrlT)
	recorder.elapsed = time.Second
	m = send(m, enterKey, tea.KeyPressMsg{Code: tea.KeySpace, Text: " "})
	if !strings.Contains(composerText(m), "Sound output is unavailable here.") {
		t.Fatalf("the review hides why space did nothing:\n%s", composerText(m))
	}
}

func TestClicksWaitWhileTheMicrophoneIsOpen(t *testing.T) {
	m, _, _, sound := recordingApp(t)
	thread := m.remotes.thread
	thread.messages = append(thread.messages, voiceNote(40, 10))
	m.renderRemoteThread()
	m = send(m, ctrlT)
	area, _ := m.voiceArea(40)
	m = settleCmd(m, m.threadClick(area.line, area.play))
	if len(sound.plays) != 0 || !m.remotes.thread.composer.focused {
		t.Fatal("a click while recording must not play a note into the microphone or leave the composer")
	}
}

func TestAPausedNoteSeeksUpToItsEndAndReplaysFromThere(t *testing.T) {
	m, f := voiceApp(t)
	m = voiceKeys(t, m, "space", "space")
	for range 7 {
		m = voiceKeys(t, m, "right")
	}
	if m.voiceAt() != 10*time.Second || m.voice.playback.state != "paused" {
		t.Fatalf("seeking a paused note stops at its end: %v %q", m.voiceAt(), m.voice.playback.state)
	}
	m = voiceKeys(t, m, "space")
	if got := f.sound.playing().Position(); m.voice.playback.state != "playing" || got != 0 {
		t.Fatalf("space on a note at its end replays it from the start: %q at %v", m.voice.playback.state, got)
	}
}

func statusText(m AppModel) string { return ansi.Strip(m.remotesStatus()) }

func TestASelectedVoiceNoteShowsItsKeys(t *testing.T) {
	m, _ := voiceApp(t)
	if got := statusText(m) + composerText(m); !strings.Contains(got, "space play") {
		t.Fatalf("a selected voice note must say how to play it:\n%s", got)
	}
	m = voiceKeys(t, m, "space")
	for _, hint := range []string{"space pause", "←/→ seek", "x speed", "-/+ volume"} {
		if got := statusText(m) + composerText(m); !strings.Contains(got, hint) {
			t.Fatalf("a playing note must show %q:\n%s", hint, got)
		}
	}
	m.remotes.thread.selected = "m:8"
	if got := statusText(m); strings.Contains(got, "space play") {
		t.Fatalf("a text message must not show the player keys: %s", got)
	}
}

func TestANewestVoiceNoteNobodySelectedKeepsTheMessageKeys(t *testing.T) {
	m, f := voiceApp(t)
	// No selection follows new messages, and nothing is highlighted, so the newest note is not picked.
	m.remotes.thread.selected = ""
	if got := statusText(m) + composerText(m); strings.Contains(got, "space play") || !strings.Contains(got, "j/k select") || !strings.Contains(got, "ctrl+t voice") {
		t.Fatalf("an unselected voice note must leave the message keys:\n%s", got)
	}
	m = voiceKeys(t, m, "space")
	if len(f.sound.plays) != 0 {
		t.Fatal("space must not play a note nobody selected")
	}
}

func TestANotePlayingAfterTheSelectionReturnsToNewMessagesKeepsItsKeys(t *testing.T) {
	m, f := voiceApp(t)
	m.remotes.thread.selected = "m:41"
	m = voiceKeys(t, m, "space")
	// G, j past the newest message, and pushing all return the selection to following new messages.
	m.remotes.thread.selected = ""
	if got := statusText(m); !strings.Contains(got, "space pause") {
		t.Fatalf("a note still playing must keep its keys: %s", got)
	}
	m = voiceKeys(t, m, "space")
	if f.sound.stops == 0 || m.voice.playback.state != "paused" || m.remotes.thread.selected != "m:41" {
		t.Fatalf("space pauses the playing note and selects it, so it can resume: %q, %q", m.voice.playback.state, m.remotes.thread.selected)
	}
}

func TestRecordingIsDiscoverableFromTheMessages(t *testing.T) {
	m, _, recorder, _ := recordingApp(t)
	m = send(m, escKey)
	if got := composerText(m); !strings.Contains(got, "ctrl+t voice") {
		t.Fatalf("the messages hint must offer recording:\n%s", got)
	}
	m = send(m, ctrlT)
	if m.remotes.thread.composer.recording == nil || !m.remotes.thread.composer.focused {
		t.Fatal("ctrl+t from the messages focuses the composer and records")
	}
	recorder.elapsed = time.Second
	m = send(m, enterKey, tea.KeyPressMsg{Code: 'd', Text: "d"})
	if got := composerText(m); !m.remotes.thread.composer.focused || !strings.Contains(got, "ctrl+t voice") {
		t.Fatalf("the writing hint must offer recording:\n%s", got)
	}
}

func TestWritingHintKeepsVoiceVisibleWhenNarrow(t *testing.T) {
	m, _, _, _ := recordingApp(t)
	next, _ := m.Update(tea.WindowSizeMsg{Width: 60, Height: 18})
	m = next.(AppModel)
	if got := composerText(m); !strings.Contains(got, "ctrl+t voice") {
		t.Fatalf("at 60 columns the writing hint must still show ctrl+t:\n%s", got)
	}
}

// TestTheWaveformStandsOutOnEveryBubble keeps the unplayed bars readable on a blue own bubble and a grey one, in both
// themes, while the played bars stay brighter.
func TestTheWaveformStandsOutOnEveryBubble(t *testing.T) {
	m, _ := voiceApp(t)
	note := voiceNote(1, 10)
	colour := regexp.MustCompile(`38;2;(\d+);(\d+);(\d+)`)
	for _, dark := range []bool{true, false} {
		p := colors(dark)
		for _, bubble := range [][2]string{{p.shell, p.blue}, {p.ink, p.wash}} {
			line, _, start, _ := m.voiceLine(note, 60, bubble[0], bubble[1], p)
			wave := strings.Index(line, string([]rune(voiceLevels)[0]))
			if wave < 0 || start == 0 {
				t.Fatalf("no waveform in %q", line)
			}
			found := colour.FindAllStringSubmatch(line[max(0, wave-40):wave], -1)
			if len(found) == 0 {
				t.Fatalf("waveform has no colour: %q", line)
			}
			last := found[len(found)-1]
			var rgb [3]int
			for i := range rgb {
				rgb[i], _ = strconv.Atoi(last[i+1])
			}
			unplayed := fmt.Sprintf("#%02x%02x%02x", rgb[0], rgb[1], rgb[2])
			if got := contrast(unplayed, bubble[1]); got < 2.5 {
				t.Errorf("dark=%v: unplayed bars %s on %s have contrast %.2f, want at least 2.5", dark, unplayed, bubble[1], got)
			}
			if contrast(bubble[0], bubble[1]) <= contrast(unplayed, bubble[1]) {
				t.Errorf("dark=%v: played bars must stand out more than unplayed ones", dark)
			}
		}
	}
}
