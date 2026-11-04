package audio

import (
	"encoding/binary"
	"errors"

	"github.com/pion/opus"
)

const (
	opusRate = 48000
	// maxPackets allows MaxDuration in the shortest Opus frames, 2.5 ms.
	maxPackets = int(MaxDuration/1e6) * 2 / 5
	// maxSamples allows MaxDuration plus a second of padding at 48 kHz.
	maxSamples = (int(MaxDuration/1e9) + 1) * opusRate
	// maxFrameSamples is the most one Opus packet decodes to: 120 ms at 48 kHz.
	maxFrameSamples = 5760
)

type mp4Box struct {
	kind       string
	start, end int
}

type opusTrack struct {
	channels uint8
	preSkip  uint16
	packets  [][]byte
}

var errBadMP4 = errors.New("not an Opus MP4 voice note")

// boxes lists the boxes between start and end. A size of 0 runs to the end, and 64-bit sizes aren't used by notes.
func boxes(data []byte, start, end int) ([]mp4Box, error) {
	var found []mp4Box
	for start < end {
		if end-start < 8 {
			return nil, errBadMP4
		}
		size := int(binary.BigEndian.Uint32(data[start:]))
		if size == 0 {
			size = end - start
		}
		if size < 8 || size > end-start {
			return nil, errBadMP4
		}
		found = append(found, mp4Box{string(data[start+4 : start+8]), start + 8, start + size})
		start += size
	}
	return found, nil
}

func child(data []byte, parent mp4Box, path ...string) (mp4Box, error) {
	current := parent
	for _, kind := range path {
		children, err := boxes(data, current.start, current.end)
		if err != nil {
			return mp4Box{}, err
		}
		next := -1
		for i, box := range children {
			if box.kind == kind {
				next = i
				break
			}
		}
		if next < 0 {
			return mp4Box{}, errBadMP4
		}
		current = children[next]
	}
	return current, nil
}

// fullBox is a box's data after its version and flags, if it holds at least size bytes.
func fullBox(data []byte, box mp4Box, size int) ([]byte, error) {
	if box.end-box.start < 4+size {
		return nil, errBadMP4
	}
	return data[box.start+4 : box.end], nil
}

// readMP4 reads the one Opus track: its channels and pre-skip from dOps, and its packets through stsc, stsz, and stco.
func readMP4(data []byte) (opusTrack, error) {
	stbl, err := child(data, mp4Box{start: 0, end: len(data)}, "moov", "trak", "mdia", "minf", "stbl")
	if err != nil {
		return opusTrack{}, err
	}
	track, err := readOpusEntry(data, stbl)
	if err != nil {
		return opusTrack{}, err
	}
	sizes, err := sampleSizes(data, stbl)
	if err != nil {
		return opusTrack{}, err
	}
	offsets, err := chunkOffsets(data, stbl)
	if err != nil {
		return opusTrack{}, err
	}
	perChunk, err := samplesPerChunk(data, stbl, len(offsets))
	if err != nil {
		return opusTrack{}, err
	}
	for chunk, at := range offsets {
		for range perChunk[chunk] {
			if len(track.packets) == len(sizes) {
				break
			}
			size := sizes[len(track.packets)]
			if at < 0 || size > len(data)-at {
				return opusTrack{}, errBadMP4
			}
			track.packets = append(track.packets, data[at:at+size])
			at += size
		}
	}
	if len(track.packets) != len(sizes) || len(sizes) == 0 {
		return opusTrack{}, errBadMP4
	}
	return track, nil
}

func readOpusEntry(data []byte, stbl mp4Box) (opusTrack, error) {
	stsd, err := child(data, stbl, "stsd")
	if err != nil {
		return opusTrack{}, err
	}
	entries, err := boxes(data, stsd.start+8, stsd.end)
	if err != nil || stsd.end-stsd.start < 8 || len(entries) != 1 || entries[0].kind != "Opus" {
		return opusTrack{}, errBadMP4
	}
	// An audio sample entry has 28 bytes of fields before its own boxes.
	entry := entries[0]
	if entry.end-entry.start < 28 {
		return opusTrack{}, errBadMP4
	}
	dOps, err := child(data, mp4Box{start: entry.start + 28, end: entry.end}, "dOps")
	if err != nil || dOps.end-dOps.start < 11 {
		return opusTrack{}, errBadMP4
	}
	channels := data[dOps.start+1]
	mapping := data[dOps.start+10]
	if channels < 1 || channels > 2 || mapping != 0 {
		return opusTrack{}, errBadMP4
	}
	return opusTrack{channels: channels, preSkip: binary.BigEndian.Uint16(data[dOps.start+2:])}, nil
}

func sampleSizes(data []byte, stbl mp4Box) ([]int, error) {
	box, err := child(data, stbl, "stsz")
	if err != nil {
		return nil, err
	}
	fields, err := fullBox(data, box, 8)
	if err != nil {
		return nil, err
	}
	fixed, count := int(binary.BigEndian.Uint32(fields)), int(binary.BigEndian.Uint32(fields[4:]))
	if count > maxPackets || (fixed == 0 && len(fields) < 8+4*count) {
		return nil, errBadMP4
	}
	sizes := make([]int, count)
	for i := range sizes {
		sizes[i] = fixed
		if fixed == 0 {
			sizes[i] = int(binary.BigEndian.Uint32(fields[8+4*i:]))
		}
	}
	return sizes, nil
}

func chunkOffsets(data []byte, stbl mp4Box) ([]int, error) {
	box, err := child(data, stbl, "stco")
	if err != nil {
		return nil, err
	}
	fields, err := fullBox(data, box, 4)
	if err != nil {
		return nil, err
	}
	count := int(binary.BigEndian.Uint32(fields))
	if count > maxPackets || len(fields) < 4+4*count {
		return nil, errBadMP4
	}
	offsets := make([]int, count)
	for i := range offsets {
		offsets[i] = int(binary.BigEndian.Uint32(fields[4+4*i:]))
	}
	return offsets, nil
}

// samplesPerChunk expands stsc's runs, where each entry applies from its first chunk until the next entry's.
func samplesPerChunk(data []byte, stbl mp4Box, chunks int) ([]int, error) {
	box, err := child(data, stbl, "stsc")
	if err != nil {
		return nil, err
	}
	fields, err := fullBox(data, box, 4)
	if err != nil {
		return nil, err
	}
	count := int(binary.BigEndian.Uint32(fields))
	if count > maxPackets || len(fields) < 4+12*count {
		return nil, errBadMP4
	}
	perChunk := make([]int, chunks)
	for i := range count {
		first := int(binary.BigEndian.Uint32(fields[4+12*i:]))
		samples := int(binary.BigEndian.Uint32(fields[8+12*i:]))
		if first < 1 || samples > maxPackets {
			return nil, errBadMP4
		}
		for chunk := first - 1; chunk < chunks; chunk++ {
			perChunk[chunk] = samples
		}
	}
	return perChunk, nil
}

// decode turns the packets into mono 48 kHz samples, mixing stereo down and dropping the pre-skip. A panic from a
// hostile packet becomes an error instead of a crash.
func (t opusTrack) decode() (decoded Audio, err error) {
	defer func() {
		if recover() != nil {
			decoded, err = Audio{}, errBadMP4
		}
	}()
	channels := int(t.channels)
	decoder, err := opus.NewDecoderWithOutput(opusRate, channels)
	if err != nil {
		return Audio{}, err
	}
	frame := make([]float32, maxFrameSamples*channels)
	var samples []float32
	for _, packet := range t.packets {
		count, err := decoder.DecodeToFloat32(packet, frame)
		if err != nil || len(samples)+count > maxSamples {
			return Audio{}, errBadMP4
		}
		for i := range count {
			if channels == 2 {
				samples = append(samples, (frame[2*i]+frame[2*i+1])/2)
				continue
			}
			samples = append(samples, frame[i])
		}
	}
	return Audio{Samples: samples[min(int(t.preSkip), len(samples)):], Rate: opusRate}, nil
}
