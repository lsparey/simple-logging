package storage

import (
	"bufio"
	"io"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestCapLine(t *testing.T) {
	short := "fits"
	if got := CapLine(short); got != short {
		t.Errorf("short line changed: %q", got)
	}

	long := strings.Repeat("é", MaxLineBytes) // 2 bytes per rune
	got := CapLine(long)
	if len(got) > MaxLineBytes {
		t.Errorf("capped line is %d bytes, over %d", len(got), MaxLineBytes)
	}
	if !strings.HasSuffix(got, truncatedMarker) {
		t.Error("capped line is missing the truncation marker")
	}
	if !utf8.ValidString(got) {
		t.Error("cap split a UTF-8 character")
	}
}

func TestReadCappedLine(t *testing.T) {
	input := "short\n" + strings.Repeat("z", 100) + "\nlast without newline"
	r := bufio.NewReaderSize(strings.NewReader(input), 16) // smaller than the long line
	var got []string
	for {
		line, err := ReadCappedLine(r, 40)
		if err == io.EOF {
			break
		}
		if err != nil && line == "" {
			t.Fatalf("ReadCappedLine: %v", err)
		}
		got = append(got, line)
	}
	want := []string{"short", strings.Repeat("z", 40), "last without newline"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("got %q, want %q", got, want)
	}
}
