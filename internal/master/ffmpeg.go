package master

import (
	"fmt"
	"strings"
)

// Loudnorm holds one-pass loudnorm targets (EBU R128).
type Loudnorm struct {
	I   float64 // integrated loudness target (LUFS)
	TP  float64 // true peak (dBTP)
	LRA float64 // loudness range (LU)
}

func (l Loudnorm) filter() string {
	return fmt.Sprintf("loudnorm=I=%g:TP=%g:LRA=%g", l.I, l.TP, l.LRA)
}

// silenceArgs builds the ffmpeg invocation that renders one silence WAV
// matching the line WAVs' format (rate, mono).
func silenceArgs(rate, durationMS int, outPath string) []string {
	return []string{
		"-y",
		"-f", "lavfi",
		"-i", fmt.Sprintf("anullsrc=r=%d:cl=mono", rate),
		"-t", fmt.Sprintf("%.3f", float64(durationMS)/1000.0),
		"-c:a", "pcm_s16le",
		outPath,
	}
}

// concatArgs builds the final concat + loudnorm + encode invocation.
//
// metadataPath is the ffmetadata chapter file; empty disables chapters.
// format is "mp3" or "m4b" (m4b uses the ipod muxer — the standard container
// for AAC audiobooks).
func concatArgs(listPath, metadataPath, format, outPath string, ln Loudnorm, bitrate string) []string {
	args := []string{
		"-y",
		"-f", "concat",
		"-safe", "0",
		"-i", listPath,
	}
	if metadataPath != "" {
		args = append(args, "-i", metadataPath, "-map_metadata", "1")
	}
	args = append(args, "-af", ln.filter())
	switch format {
	case "mp3":
		args = append(args, "-c:a", "libmp3lame", "-b:a", bitrate)
	case "m4b":
		args = append(args, "-c:a", "aac", "-b:a", bitrate, "-movflags", "+faststart", "-f", "ipod")
	}
	args = append(args, outPath)
	return args
}

// concatList renders the concat demuxer input file. Single quotes inside
// paths are escaped per the ffmpeg concat demuxer quoting rules.
func concatList(paths []string) string {
	var b strings.Builder
	for _, p := range paths {
		b.WriteString("file '")
		b.WriteString(strings.ReplaceAll(p, "'", `'\''`))
		b.WriteString("'\n")
	}
	return b.String()
}

// Chapter is one m4b chapter (times in milliseconds).
type Chapter struct {
	Title   string
	StartMS int
	EndMS   int
}

// ffmetadata renders the FFMETADATA1 chapter file.
func ffmetadata(chapters []Chapter) string {
	var b strings.Builder
	b.WriteString(";FFMETADATA1\n")
	for _, c := range chapters {
		b.WriteString("[CHAPTER]\n")
		b.WriteString("TIMEBASE=1/1000\n")
		fmt.Fprintf(&b, "START=%d\n", c.StartMS)
		fmt.Fprintf(&b, "END=%d\n", c.EndMS)
		fmt.Fprintf(&b, "title=%s\n", escapeMetadataValue(c.Title))
	}
	return b.String()
}

// escapeMetadataValue escapes the characters the ffmetadata format treats
// specially (=, ;, #, \ and newline).
func escapeMetadataValue(s string) string {
	r := strings.NewReplacer(
		`\`, `\\`,
		"=", `\=`,
		";", `\;`,
		"#", `\#`,
		"\n", `\`+"\n",
	)
	return r.Replace(s)
}
