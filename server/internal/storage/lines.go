package storage

import (
	"bufio"
	"io"
	"unicode/utf8"
)

// MaxLineBytes caps the size of one stored log line, prefix included.
// SegmentWriter truncates anything longer (see CapLine), so every reader of
// segments can use NewLineScanner and never meet a line it can't hold.
const MaxLineBytes = 1 << 20

// truncatedMarker ends a line that CapLine shortened.
const truncatedMarker = " …[truncated]"

// CapLine returns line unchanged if it fits in MaxLineBytes, and otherwise
// cuts it (on a UTF-8 boundary) and appends truncatedMarker so the total is
// within MaxLineBytes.
func CapLine(line string) string {
	if len(line) <= MaxLineBytes {
		return line
	}
	cut := MaxLineBytes - len(truncatedMarker)
	for cut > 0 && !utf8.RuneStart(line[cut]) {
		cut--
	}
	return line[:cut] + truncatedMarker
}

// NewLineScanner returns a line scanner over a segment (or any stored log
// file) whose buffer fits the longest line SegmentWriter will write.
func NewLineScanner(r io.Reader) *bufio.Scanner {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 64*1024), MaxLineBytes+1)
	return scanner
}

// ReadCappedLine reads one '\n'-terminated line from r, of any length,
// returning at most maxBytes of it (without the newline) and discarding the
// rest, so an arbitrarily long source line costs bounded memory. It returns
// io.EOF only when no bytes at all were read.
func ReadCappedLine(r *bufio.Reader, maxBytes int) (string, error) {
	var buf []byte
	for {
		chunk, err := r.ReadSlice('\n')
		if room := maxBytes - len(buf); room > 0 {
			if len(chunk) > room {
				buf = append(buf, chunk[:room]...)
			} else {
				buf = append(buf, chunk...)
			}
		}
		switch err {
		case nil:
			return trimNewline(buf), nil
		case bufio.ErrBufferFull:
			continue
		case io.EOF:
			if len(buf) == 0 {
				return "", io.EOF
			}
			return trimNewline(buf), nil
		default:
			return trimNewline(buf), err
		}
	}
}

func trimNewline(b []byte) string {
	if n := len(b); n > 0 && b[n-1] == '\n' {
		b = b[:n-1]
	}
	if n := len(b); n > 0 && b[n-1] == '\r' {
		b = b[:n-1]
	}
	return string(b)
}
