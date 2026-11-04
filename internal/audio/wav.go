package audio

import (
	"bytes"
	"encoding/binary"
	"time"
)

// wavFormat is VoiceNotes::wavFormat: PCM, mono, 16 kHz, 32000 bytes a second, 2-byte frames, 16 bits.
var wavFormat = []byte{1, 0, 1, 0, 0x80, 0x3e, 0, 0, 0x00, 0x7d, 0, 0, 2, 0, 16, 0}

// EncodeWAV writes 16 kHz mono samples as the only WAV the server accepts, clamping like the web's encodeWav. It keeps
// at most MaxDuration, which is all the server takes.
func EncodeWAV(samples []float32) []byte {
	count := len(samples)
	if limit := int(MaxDuration/time.Second) * RecordRate; count > limit {
		count = limit
	}
	pcm := make([]int16, count)
	for i, sample := range samples[:count] {
		clamped := max(-1, min(1, sample))
		pcm[i] = int16(clamped * 0x7fff)
		if clamped < 0 {
			pcm[i] = int16(clamped * 0x8000)
		}
	}
	size := uint32(2 * count)
	out := make([]byte, 0, 44+size)
	out = append(out, "RIFF"...)
	out = binary.LittleEndian.AppendUint32(out, 36+size)
	out = append(out, "WAVEfmt "...)
	out = binary.LittleEndian.AppendUint32(out, 16)
	out = append(out, wavFormat...)
	out = append(out, "data"...)
	out = binary.LittleEndian.AppendUint32(out, size)
	out, _ = binary.Append(out, binary.LittleEndian, pcm)
	return out
}

// readWAV walks the chunks the way VoiceNotes::samples does and accepts only its one format.
func readWAV(data []byte) (Audio, error) {
	if len(data) < 12 || string(data[8:12]) != "WAVE" {
		return Audio{}, errNotVoice
	}
	var format []byte
	for at := 12; at+8 <= len(data); {
		id := string(data[at : at+4])
		size := int(binary.LittleEndian.Uint32(data[at+4:]))
		body := at + 8
		if size > len(data)-body {
			return Audio{}, errNotVoice
		}
		if id == "fmt " && size >= 16 {
			format = data[body : body+16]
		}
		if id == "data" {
			if !bytes.Equal(format, wavFormat) || size%2 != 0 {
				return Audio{}, errNotVoice
			}
			pcm := make([]int16, size/2)
			if _, err := binary.Decode(data[body:body+size], binary.LittleEndian, pcm); err != nil {
				return Audio{}, errNotVoice
			}
			samples := make([]float32, len(pcm))
			for i, value := range pcm {
				samples[i] = float32(value) / 32768
			}
			return Audio{Samples: samples, Rate: RecordRate}, nil
		}
		at = body + size + size%2
	}
	return Audio{}, errNotVoice
}
