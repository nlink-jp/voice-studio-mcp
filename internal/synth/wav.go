package synth

import (
	"encoding/binary"
	"fmt"
)

// WAVInfo is the subset of WAV header data the pipeline needs.
type WAVInfo struct {
	SampleRate      int
	Channels        int
	BitsPerSample   int
	DataBytes       int
	DurationSeconds float64
}

// ParseWAV reads the RIFF header of b and returns format and duration
// information. It walks chunks, so extra chunks (LIST etc.) are tolerated.
func ParseWAV(b []byte) (WAVInfo, error) {
	var info WAVInfo
	if len(b) < 12 || string(b[0:4]) != "RIFF" || string(b[8:12]) != "WAVE" {
		return info, fmt.Errorf("not a RIFF/WAVE stream (%d bytes)", len(b))
	}
	off := 12
	haveFmt := false
	for off+8 <= len(b) {
		id := string(b[off : off+4])
		size := int(binary.LittleEndian.Uint32(b[off+4 : off+8]))
		body := off + 8
		if size < 0 || body > len(b) {
			return info, fmt.Errorf("corrupt chunk %q at offset %d", id, off)
		}
		switch id {
		case "fmt ":
			if size < 16 || body+16 > len(b) {
				return info, fmt.Errorf("fmt chunk too short (%d bytes)", size)
			}
			info.Channels = int(binary.LittleEndian.Uint16(b[body+2 : body+4]))
			info.SampleRate = int(binary.LittleEndian.Uint32(b[body+4 : body+8]))
			info.BitsPerSample = int(binary.LittleEndian.Uint16(b[body+14 : body+16]))
			haveFmt = true
		case "data":
			// The data chunk may be the last chunk with its size covering the
			// remainder; clamp to the actual buffer.
			if body+size > len(b) {
				size = len(b) - body
			}
			info.DataBytes = size
		}
		// Chunks are word-aligned.
		off = body + size + (size & 1)
	}
	if !haveFmt {
		return info, fmt.Errorf("missing fmt chunk")
	}
	if info.DataBytes == 0 {
		return info, fmt.Errorf("missing or empty data chunk")
	}
	bytesPerSecond := info.SampleRate * info.Channels * info.BitsPerSample / 8
	if bytesPerSecond <= 0 {
		return info, fmt.Errorf("invalid format: rate=%d ch=%d bits=%d",
			info.SampleRate, info.Channels, info.BitsPerSample)
	}
	info.DurationSeconds = float64(info.DataBytes) / float64(bytesPerSecond)
	return info, nil
}
