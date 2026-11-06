package tui

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/WendellAdriel/dev-null-cli/internal/api"
	"github.com/WendellAdriel/dev-null-cli/internal/audio"
)

var voiceSpeeds = []float64{1, 1.5, 2}

const (
	voiceSeek       = 2 * time.Second
	voiceTickEvery  = 100 * time.Millisecond
	voiceVolumeStep = 0.1
	volumeSaveDelay = 500 * time.Millisecond
	voiceLevels     = "▁▂▃▄▅▆▇█"
)

// voicePlayback is the one note playing or paused. Its decoded audio is dropped as soon as it stops being the note.
type voicePlayback struct {
	messageID    int64
	conversation string
	state        string
	audio        audio.Audio
	stream       *audio.Stream
	at           time.Duration
	speed        float64
	generation   int
}

type voiceState struct {
	openSound   func() (audio.Device, error)
	record      func() (audio.Recorder, error)
	download    func(context.Context, string) ([]byte, error)
	device      audio.Device
	unavailable bool
	playback    voicePlayback
	generation  int
	// volumeGeneration debounces volume saves, and volumePending is a change quitting must still write.
	volumeGeneration int
	volumePending    bool
}

// voiceArea is a voice note's row in the thread: the play glyph and the waveform, for clicks.
type voiceArea struct {
	line, play, bars, width int
	id                      int64
}

type voiceLoaded struct {
	session, generation int
	audio               audio.Audio
	err                 error
}

type voiceTick struct{ session, generation int }

type volumeSaveTick struct{ session, generation int }

// selectedVoice is the voice note picked with j/k or a click. Unlike reply and react, it never falls back to the newest
// message, which is not highlighted while the thread follows new messages, but a note still playing keeps its keys.
func (m AppModel) selectedVoice() (*api.Message, bool) {
	if m.remotes.thread.selected == "" {
		return m.playingVoice()
	}
	item, ok := m.selectedItem()
	if !ok || item.message == nil || item.message.Voice == nil {
		return nil, false
	}
	return item.message, true
}

func (m AppModel) playingVoice() (*api.Message, bool) {
	p, t := m.voice.playback, m.remotes.thread
	if p.state != "playing" || p.conversation != t.conversation.ID {
		return nil, false
	}
	for _, item := range m.threadItems() {
		if item.message != nil && item.message.ID == p.messageID && item.message.Voice != nil {
			return item.message, true
		}
	}
	return nil, false
}

// voiceKey handles the player keys on a selected voice note. Seek and speed act only on the note already playing or
// paused, so Left still closes the thread from any other message.
func (m *AppModel) voiceKey(pressed string) (tea.Cmd, bool) {
	note, ok := m.selectedVoice()
	if !ok {
		return nil, false
	}
	cmd, handled := m.playerKey(note, pressed)
	// Selecting a note played while following new messages keeps its keys once it pauses.
	if handled && m.remotes.thread.selected == "" {
		m.remotes.thread.selected = threadItem{message: note}.key()
		m.renderRemoteThread()
	}
	return cmd, handled
}

func (m *AppModel) playerKey(note *api.Message, pressed string) (tea.Cmd, bool) {
	p := &m.voice.playback
	loaded := p.messageID == note.ID && (p.state == "playing" || p.state == "paused")
	switch pressed {
	case "space":
		return m.toggleVoice(note), true
	case "left", "right":
		if !loaded {
			return nil, false
		}
		delta := voiceSeek
		if pressed == "left" {
			delta = -voiceSeek
		}
		return m.seekVoice(m.voiceAt() + delta), true
	case "x":
		if !loaded {
			return nil, false
		}
		return m.cycleVoiceSpeed(), true
	case "-", "+", "=":
		step := voiceVolumeStep
		if pressed == "-" {
			step = -step
		}
		return m.changeVoiceVolume(step), true
	}
	return nil, false
}

// voiceKeysHint names the player keys for the selected voice note, or is empty for any other message.
func (m AppModel) voiceKeysHint() string {
	note, ok := m.selectedVoice()
	switch {
	case !ok:
		return ""
	case m.voice.unavailable:
		return "space copy the web link"
	}
	p := m.voice.playback
	if p.messageID != note.ID || (p.state != "playing" && p.state != "paused") {
		return "space play · -/+ volume"
	}
	action := "space play"
	if p.state == "playing" {
		action = "space pause"
	}
	return action + " · ←/→ seek · x speed · -/+ volume"
}

func (m *AppModel) toggleVoice(note *api.Message) tea.Cmd {
	p := &m.voice.playback
	if p.messageID == note.ID {
		switch p.state {
		case "playing":
			m.pauseVoice()
			return nil
		case "paused":
			// Like the web's audio element, playing a note left at its end starts it over.
			if p.at >= p.audio.Duration() {
				return m.playVoice(0)
			}
			return m.playVoice(p.at)
		case "loading":
			return nil
		}
	}
	m.stopVoice()
	if !m.ensureSoundDevice() {
		m.renderRemoteThread()
		return m.copyText(m.conversationWebURL(m.remotes.thread.conversation), "Conversation link")
	}
	m.voice.generation++
	*p = voicePlayback{messageID: note.ID, conversation: m.remotes.thread.conversation.ID, state: "loading", speed: 1, generation: m.voice.generation}
	m.renderRemoteThread()
	session, generation, download, url, life := m.session, p.generation, m.voice.download, note.Voice.URL, m.account.life
	return life.command(func() tea.Msg {
		ctx, cancel := context.WithTimeout(life.ctx, 30*time.Second)
		defer cancel()
		data, err := download(ctx, url)
		if err != nil {
			return voiceLoaded{session: session, generation: generation, err: err}
		}
		decoded, err := audio.Decode(data)
		return voiceLoaded{session: session, generation: generation, audio: decoded, err: err}
	})
}

func (m *AppModel) acceptVoiceLoaded(msg voiceLoaded) tea.Cmd {
	p := &m.voice.playback
	t := m.remotes.thread
	if msg.session != m.session || msg.generation != p.generation || t == nil || t.conversation.ID != p.conversation {
		return nil
	}
	if msg.err != nil {
		p.state = "failed"
		m.renderRemoteThread()
		return nil
	}
	p.audio = msg.audio
	return m.playVoice(0)
}

// ensureSoundDevice opens the sound device on first use, and remembers for the session when there is none.
func (m *AppModel) ensureSoundDevice() bool {
	if !m.voice.unavailable && m.voice.device == nil {
		device, err := m.voice.openSound()
		m.voice.device, m.voice.unavailable = device, err != nil
	}
	return !m.voice.unavailable
}

func (m *AppModel) playVoice(from time.Duration) tea.Cmd {
	p := &m.voice.playback
	if from >= p.audio.Duration() {
		m.finishVoice()
		return nil
	}
	stream := audio.NewStream(p.audio, audio.StreamOptions{From: from, Speed: p.speed, Volume: m.tuiSettings.VoiceVolume, Rate: m.voice.device.Rate()})
	if err := m.voice.device.Play(stream); err != nil {
		p.state, p.stream = "failed", nil
		m.renderRemoteThread()
		return nil
	}
	p.stream, p.state, p.at = stream, "playing", from
	m.voice.generation++
	p.generation = m.voice.generation
	m.renderRemoteThread()
	return m.voiceTick()
}

func (m AppModel) voiceTick() tea.Cmd {
	session, generation := m.session, m.voice.playback.generation
	return tea.Tick(voiceTickEvery, func(time.Time) tea.Msg { return voiceTick{session, generation} })
}

// acceptVoiceTick redraws the clock and waveform while a note plays, and rewinds it when it ends, as the web does.
func (m *AppModel) acceptVoiceTick(msg voiceTick) tea.Cmd {
	p := &m.voice.playback
	if msg.session != m.session || msg.generation != p.generation || p.state != "playing" {
		return nil
	}
	if p.stream.Finished(m.voice.device.Buffered()) {
		m.finishVoice()
		return nil
	}
	m.renderRemoteThread()
	return m.voiceTick()
}

// finishVoice rewinds a note that played to its end, or was moved there, without starting the device again.
func (m *AppModel) finishVoice() {
	p := &m.voice.playback
	if p.stream != nil {
		m.voice.device.Stop()
	}
	p.state, p.stream, p.at = "paused", nil, 0
	m.voice.generation++
	p.generation = m.voice.generation
	m.renderRemoteThread()
}

func (m *AppModel) pauseVoice() {
	p := &m.voice.playback
	p.at = m.voiceAt()
	m.voice.device.Stop()
	p.state, p.stream = "paused", nil
	m.voice.generation++
	p.generation = m.voice.generation
	m.renderRemoteThread()
}

// voiceAt is where the note is as heard, behind what the device has read by what it still holds.
func (m AppModel) voiceAt() time.Duration {
	p := m.voice.playback
	if p.stream != nil {
		return p.stream.Heard(m.voice.device.Buffered())
	}
	return p.at
}

func (m *AppModel) seekVoice(to time.Duration) tea.Cmd {
	p := &m.voice.playback
	to = max(0, to)
	if p.state == "playing" {
		return m.playVoice(to)
	}
	p.at = min(to, p.audio.Duration())
	m.renderRemoteThread()
	return nil
}

func (m *AppModel) cycleVoiceSpeed() tea.Cmd {
	p := &m.voice.playback
	next := 0
	for i, speed := range voiceSpeeds {
		if speed == p.speed {
			next = (i + 1) % len(voiceSpeeds)
		}
	}
	p.speed = voiceSpeeds[next]
	if p.state == "playing" {
		return m.playVoice(m.voiceAt())
	}
	m.renderRemoteThread()
	return nil
}

func (m *AppModel) changeVoiceVolume(step float64) tea.Cmd {
	volume := math.Round(min(1, max(0, m.tuiSettings.VoiceVolume+step))*100) / 100
	if volume == m.tuiSettings.VoiceVolume {
		return nil
	}
	m.tuiSettings.VoiceVolume = volume
	m.voice.volumePending = true
	if stream := m.voice.playback.stream; stream != nil {
		stream.SetVolume(volume)
	}
	m.renderRemoteThread()
	m.voice.volumeGeneration++
	session, generation := m.session, m.voice.volumeGeneration
	return tea.Tick(volumeSaveDelay, func(time.Time) tea.Msg { return volumeSaveTick{session, generation} })
}

// acceptVolumeSave writes tui.json once the volume keys settle, so quick presses never overlap their saves.
func (m *AppModel) acceptVolumeSave(msg volumeSaveTick) tea.Cmd {
	if msg.session != m.session || msg.generation != m.voice.volumeGeneration {
		return nil
	}
	m.voice.volumePending = false
	return m.saveTUISettings(false)
}

// stopVoice stops the note and forgets its audio, for leaving the thread or starting another note.
func (m *AppModel) stopVoice() {
	if m.voice.playback.messageID != 0 && m.voice.device != nil {
		m.voice.device.Stop()
	}
	m.voice.generation++
	m.voice.playback = voicePlayback{}
}

// closeVoice also lets go of the sound device, for signing out or quitting.
func (m *AppModel) closeVoice() {
	m.stopVoice()
	if m.voice.device != nil {
		m.voice.device.Close()
	}
	m.voice.device, m.voice.unavailable = nil, false
}

func (m AppModel) conversationWebURL(c api.Conversation) string {
	origin := strings.TrimRight(m.account.origin, "/")
	if c.Kind == "org" {
		return origin + "/remotes/orgs/" + c.ID
	}
	return origin + "/remotes/@" + c.Partner.Handle
}

func (m AppModel) voiceArea(id int64) (voiceArea, bool) {
	for _, area := range m.remotes.thread.voices {
		if area.id == id {
			return area, true
		}
	}
	return voiceArea{}, false
}

// voiceClick plays or pauses from the glyph, and seeks from the waveform once the note is loaded.
func (m *AppModel) voiceClick(area voiceArea, x int) tea.Cmd {
	t := m.remotes.thread
	t.selected = fmt.Sprint("m:", area.id)
	note, ok := m.selectedVoice()
	if !ok {
		return nil
	}
	p := m.voice.playback
	loaded := p.messageID == area.id && (p.state == "playing" || p.state == "paused")
	if loaded && x >= area.bars && x < area.bars+area.width {
		fraction := (float64(x-area.bars) + 0.5) / float64(area.width)
		return m.seekVoice(time.Duration(fraction * float64(p.audio.Duration())))
	}
	return m.toggleVoice(note)
}

func formatVoiceTime(d time.Duration) string {
	seconds := max(0, int(d/time.Second))
	return fmt.Sprintf("%d:%02d", seconds/60, seconds%60)
}

func formatSpeed(speed float64) string {
	return strings.TrimSuffix(fmt.Sprintf("%.1f", speed), ".0") + "×"
}

// voiceLine draws a note as its glyph, clock, waveform, and speed, filling the waveform up to the played part. It
// returns where the glyph and waveform start in the line, and the waveform's width.
func (m AppModel) voiceLine(message api.Message, width int, foreground, background string, p palette) (string, int, int, int) {
	voice := message.Voice
	playback := m.voice.playback
	current := playback.messageID == message.ID
	duration := time.Duration(voice.DurationMS) * time.Millisecond
	speed, at := 1.0, time.Duration(0)
	if current {
		speed, at = playback.speed, m.voiceAt()
	}
	clock := at
	if clock <= 0 {
		clock = duration
	}
	glyph := "▶"
	switch {
	case current && playback.state == "playing":
		// Terminals draw ⏸ as a two-cell emoji, which pushes the clock out of place.
		glyph = "‖"
	case current && playback.state == "loading":
		glyph = "◌"
	case current && playback.state == "failed":
		glyph = "↻"
	}
	head := " " + glyph + " " + formatVoiceTime(time.Duration(float64(clock)/speed)) + " "
	tail := " " + formatSpeed(speed) + " "
	switch {
	case current && playback.state == "failed":
		tail = " Retry "
	case m.voice.unavailable && message.ID != previewID:
		tail = " Open on the web "
	}
	bars := max(4, min(audio.Bars, width-lipgloss.Width(head)-lipgloss.Width(tail)))
	played := 0
	if duration > 0 {
		played = int(float64(bars) * float64(at) / float64(duration))
	}
	// Unplayed bars are the bubble's text faded toward its fill, so they read on the blue own bubble too.
	unplayed := mixColor(foreground, background, 0.65)
	var wave strings.Builder
	for column, level := range downsample(voice.Waveform, bars) {
		colour := unplayed
		if column < played {
			colour = foreground
		}
		index := min(len([]rune(voiceLevels))-1, level*len([]rune(voiceLevels))/101)
		wave.WriteString(text(colour).Background(lipgloss.Color(background)).Render(string([]rune(voiceLevels)[index])))
	}
	tailText := tail
	if m.voice.unavailable && message.ID != previewID && (!current || playback.state != "failed") {
		tailText = linked(m.conversationWebURL(m.remotes.thread.conversation), text(foreground).Underline(true).Render)(tail)
	}
	line := paintLine(head+wave.String()+tailText, foreground, background)
	return line, 1, lipgloss.Width(head), bars
}

// downsample keeps the loudest level of each slice, so a narrow pane still shows the note's peaks.
func downsample(levels []int, columns int) []int {
	out := make([]int, columns)
	if len(levels) == 0 {
		return out
	}
	for column := range out {
		from := column * len(levels) / columns
		to := max(from+1, (column+1)*len(levels)/columns)
		for _, level := range levels[from:min(to, len(levels))] {
			out[column] = max(out[column], min(100, max(0, level)))
		}
	}
	return out
}

// previewID marks the playback of the user's own recording, which has no message id yet.
const previewID = -1

// voiceRecording is a note being recorded, then reviewed once its recorder has stopped.
type voiceRecording struct {
	recorder   audio.Recorder
	samples    []float32
	waveform   []int
	levels     []float64
	generation int
}

type recordingTick struct{ session, generation int }

// recordingKey handles Ctrl+T and every key while a recording is asked for, running, or waiting for review.
func (m *AppModel) recordingKey(pressed string) (tea.Cmd, bool) {
	mc := &m.remotes.thread.composer
	rec := mc.recording
	switch {
	case mc.voiceConfirm:
		mc.voiceConfirm = false
		if pressed != "enter" {
			return nil, true
		}
		return m.startRecording(true), true
	case rec != nil && rec.recorder != nil:
		switch pressed {
		case "enter":
			m.stopRecording()
		case "esc":
			m.cancelRecording()
			m.resizeThread()
		}
		return nil, true
	case rec != nil:
		switch pressed {
		case "space":
			return m.toggleRecordingPreview(), true
		case "enter", "ctrl+s":
			return m.pushMessage(), true
		case "d":
			m.cancelRecording()
			m.resizeThread()
			return nil, true
		case "esc":
			return nil, false
		}
		return nil, true
	case pressed == "ctrl+t":
		if strings.TrimSpace(mc.input.Value()) != "" || len(mc.images) > 0 {
			mc.voiceConfirm = true
			return nil, true
		}
		return m.startRecording(false), true
	}
	return nil, false
}

// startRecording opens the microphone, and only then clears the draft it replaces, so a failure keeps the text.
func (m *AppModel) startRecording(replaceDraft bool) tea.Cmd {
	mc := &m.remotes.thread.composer
	recorder, err := m.voice.record()
	if err != nil {
		mc.status = recordingError(err)
		return nil
	}
	m.stopVoice()
	mc.status = ""
	mc.recording = &voiceRecording{recorder: recorder, generation: m.voice.generation}
	var save tea.Cmd
	if replaceDraft {
		mc.input.Reset()
		mc.images, mc.stripFocus = nil, false
		mc.suggest.close()
		save = m.scheduleDraftSave()
	}
	m.resizeThread()
	return tea.Batch(m.recordingTick(), save)
}

// recordingError words a recorder failure the way the web does.
func recordingError(err error) string {
	switch {
	case errors.Is(err, audio.ErrNoRecorder):
		return "Voice notes need ffmpeg or sox. Record on the web."
	case errors.Is(err, audio.ErrNoMicrophone):
		return "fatal: no microphone found"
	case errors.Is(err, audio.ErrMicrophoneDenied):
		return "fatal: microphone access denied"
	case errors.Is(err, audio.ErrNoSound):
		return "fatal: no sound from the microphone. Is it muted?"
	}
	return "fatal: could not open the microphone"
}

func (m AppModel) recordingTick() tea.Cmd {
	session, generation := m.session, m.remotes.thread.composer.recording.generation
	return tea.Tick(voiceTickEvery, func(time.Time) tea.Msg { return recordingTick{session, generation} })
}

func (m *AppModel) acceptRecordingTick(msg recordingTick) tea.Cmd {
	t := m.remotes.thread
	if msg.session != m.session || t == nil || t.composer.recording == nil {
		return nil
	}
	rec := t.composer.recording
	if rec.generation != msg.generation || rec.recorder == nil {
		return nil
	}
	if err := rec.recorder.Err(); err != nil {
		m.cancelRecording()
		t.composer.status = recordingError(err)
		m.resizeThread()
		return nil
	}
	if rec.recorder.Elapsed() >= audio.MaxRecording {
		m.stopRecording()
		return nil
	}
	if rec.recorder.Elapsed() > 0 {
		rec.levels = append(rec.levels, rec.recorder.Level())
	}
	return m.recordingTick()
}

// stopRecording keeps a note of at least half a second for review, as the web does, and drops a shorter one.
func (m *AppModel) stopRecording() {
	mc := &m.remotes.thread.composer
	samples := mc.recording.recorder.Stop()
	mc.recording.recorder = nil
	if len(samples) == 0 {
		mc.recording, mc.status = nil, recordingError(audio.ErrNoSound)
		m.resizeThread()
		return
	}
	if time.Duration(len(samples))*time.Second/audio.RecordRate < audio.MinDuration {
		mc.recording, mc.status = nil, "fatal: voice note shorter than half a second"
		m.resizeThread()
		return
	}
	mc.recording.samples, mc.recording.waveform = samples, audio.Waveform(samples)
	m.resizeThread()
}

// cancelRecording releases the microphone and forgets the recording and its playback.
func (m *AppModel) cancelRecording() {
	t := m.remotes.thread
	if t == nil {
		return
	}
	mc := &t.composer
	if mc.recording != nil && mc.recording.recorder != nil {
		mc.recording.recorder.Cancel()
	}
	if m.voice.playback.messageID == previewID {
		m.stopVoice()
	}
	mc.recording, mc.voiceConfirm = nil, false
}

func (m *AppModel) toggleRecordingPreview() tea.Cmd {
	p := &m.voice.playback
	if p.messageID == previewID {
		if p.state == "playing" {
			m.pauseVoice()
			return nil
		}
		return m.playVoice(p.at)
	}
	m.stopVoice()
	if !m.ensureSoundDevice() {
		m.remotes.thread.composer.status = "Sound output is unavailable here."
		return nil
	}
	samples := m.remotes.thread.composer.recording.samples
	*p = voicePlayback{messageID: previewID, conversation: m.remotes.thread.conversation.ID, state: "paused", audio: audio.Audio{Samples: samples, Rate: audio.RecordRate}, speed: 1}
	return m.playVoice(0)
}

// recordingLines draws the recording's meter and clock, a wait until the microphone sends sound, or the recorded note
// ready for review.
func (m AppModel) recordingLines(rec *voiceRecording, width int, p palette) []string {
	if rec.recorder == nil {
		duration := len(rec.samples) * 1000 / audio.RecordRate
		note := api.Message{ID: previewID, Voice: &api.Voice{DurationMS: duration, Waveform: rec.waveform}}
		line, _, _, _ := m.voiceLine(note, width, p.ink, p.wash, p)
		return []string{" " + line}
	}
	elapsed := rec.recorder.Elapsed()
	if elapsed == 0 {
		return []string{" " + text(p.danger).Render("●") + " " + text(p.muted).Render("waiting for the microphone…")}
	}
	clock := text(p.ink)
	if audio.MaxRecording-elapsed <= 10*time.Second {
		clock = text(p.warning).Bold(true)
	}
	head := text(p.danger).Render("●") + " " + clock.Render(formatVoiceTime(elapsed)+"/"+formatVoiceTime(audio.MaxRecording)) + " "
	levels := rec.levels[max(0, len(rec.levels)-(width-lipgloss.Width(head)-1)):]
	var meter strings.Builder
	runes := []rune(voiceLevels)
	for _, level := range levels {
		meter.WriteRune(runes[min(len(runes)-1, int(level*float64(len(runes))))])
	}
	return []string{" " + head + text(p.blue).Render(meter.String())}
}
