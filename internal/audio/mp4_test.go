package audio

import (
	"bytes"
	"encoding/binary"
	"math"
)

// u32 is a size or count in a test note, which never comes near 4 GiB.
func u32(n int) uint32 {
	if n < 0 || n > math.MaxUint32 {
		panic("value out of range for a test note")
	}
	return uint32(n)
}

func writeBox(kind string, parts ...[]byte) []byte {
	data := bytes.Join(parts, nil)
	return append(binary.BigEndian.AppendUint32(nil, u32(8+len(data))), append([]byte(kind), data...)...)
}

func be(values ...uint32) []byte {
	var out []byte
	for _, v := range values {
		out = binary.BigEndian.AppendUint32(out, v)
	}
	return out
}

func be16(values ...uint16) []byte {
	var out []byte
	for _, v := range values {
		out = binary.BigEndian.AppendUint16(out, v)
	}
	return out
}

// serverMP4 follows VoiceNotes::mp4 in the backend: one chunk, an edit list for the pre-skip, and the boxes it writes.
func serverMP4(channels uint8, preSkip uint16, packets [][]byte) []byte {
	matrix := be(0x10000, 0, 0, 0, 0x10000, 0, 0, 0, 0x40000000)
	var sizes []byte
	for _, packet := range packets {
		sizes = binary.BigEndian.AppendUint32(sizes, u32(len(packet)))
	}
	moov := func(offset uint32) []byte {
		return writeBox("moov",
			writeBox("mvhd", be(0, 0, 0, 1000, 2000, 0x10000), be16(0x100), make([]byte, 10), matrix, make([]byte, 24), be(2)),
			writeBox("trak",
				writeBox("tkhd", be(3, 0, 0, 1, 0, 2000), make([]byte, 8), be16(0, 0, 0x100, 0), matrix, be(0, 0)),
				writeBox("edts", writeBox("elst", be(0, 1, 2000, uint32(preSkip), 0x10000))),
				writeBox("mdia",
					writeBox("mdhd", be(0, 0, 0, 48000, u32(len(packets)*960)), be16(0x55C4, 0)),
					writeBox("hdlr", be(0, 0), []byte("soun"), make([]byte, 12), []byte("SoundHandler\x00")),
					writeBox("minf",
						writeBox("smhd", be(0, 0)),
						writeBox("dinf", writeBox("dref", be(0, 1), writeBox("url ", be(1)))),
						writeBox("stbl",
							writeBox("stsd", be(0, 1), writeBox("Opus",
								make([]byte, 6), be16(1), make([]byte, 8), be16(uint16(channels), 16, 0, 0), be(48000<<16),
								writeBox("dOps", []byte{0, channels}, be16(preSkip), be(48000), be16(0), []byte{0}),
							)),
							writeBox("stts", be(0, 1, u32(len(packets)), 960)),
							writeBox("stsc", be(0, 1, 1, u32(len(packets)), 1)),
							writeBox("stsz", be(0, 0, u32(len(packets))), sizes),
							writeBox("stco", be(0, 1, offset)),
						),
					),
				),
			),
		)
	}
	ftyp := writeBox("ftyp", []byte("isom"), be(0x200), []byte("isomiso2mp41"))
	offset := u32(len(ftyp) + len(moov(0)) + 8)
	return bytes.Join([][]byte{ftyp, moov(offset), writeBox("mdat", bytes.Join(packets, nil))}, nil)
}
