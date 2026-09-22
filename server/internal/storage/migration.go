package storage

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"go.uber.org/zap"
)

// MigrateLegacyLayout converts any v0.11 single-file-per-pod logs
// (<logsRoot>/<namespace>/<pod>.log) into the segmented per-container,
// per-day layout, deleting each legacy file once it has been fully migrated.
// It is idempotent and safe to run on every startup: a namespace with no
// legacy files does nothing.
//
// The collector has only ever streamed one container per pod (multi-container
// collection is a later phase), so every line in a given legacy file belongs
// to the same container — its name is read from the first "[ns/pod/container]"
// tag found in the file.
func MigrateLegacyLayout(logsRoot string, log *zap.Logger) error {
	namespaceEntries, err := os.ReadDir(logsRoot)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read logs root: %w", err)
	}

	for _, nsEntry := range namespaceEntries {
		if !nsEntry.IsDir() || strings.HasPrefix(nsEntry.Name(), ".") {
			continue
		}
		namespace := nsEntry.Name()
		nsDir := filepath.Join(logsRoot, namespace)

		podFiles, err := os.ReadDir(nsDir)
		if err != nil {
			return fmt.Errorf("read namespace dir %q: %w", nsDir, err)
		}
		for _, podFile := range podFiles {
			if podFile.IsDir() || filepath.Ext(podFile.Name()) != ".log" {
				continue
			}
			pod := strings.TrimSuffix(podFile.Name(), ".log")
			legacyPath := filepath.Join(nsDir, podFile.Name())
			if err := migrateLegacyPodFile(logsRoot, namespace, pod, legacyPath, log); err != nil {
				return fmt.Errorf("migrate %s/%s: %w", namespace, pod, err)
			}
		}
	}
	return nil
}

func migrateLegacyPodFile(logsRoot, namespace, pod, legacyPath string, log *zap.Logger) error {
	container, err := detectLegacyContainer(legacyPath)
	if err != nil {
		return fmt.Errorf("detect container: %w", err)
	}
	if container == "" {
		container = "unknown"
	}

	f, err := os.Open(legacyPath)
	if err != nil {
		return err
	}
	defer f.Close()

	w, err := NewSegmentWriter(logsRoot, namespace, pod, container)
	if err != nil {
		return err
	}

	reader := bufio.NewReader(f)
	lineCount := 0
	for {
		rawLine, readErr := reader.ReadString('\n')
		line := strings.TrimRight(rawLine, "\r\n")
		if line != "" {
			if werr := w.Write(parseLegacyTimestamp(line), line); werr != nil {
				_ = w.Close()
				return fmt.Errorf("write migrated line: %w", werr)
			}
			lineCount++
		}
		if readErr != nil {
			if errors.Is(readErr, io.EOF) {
				break
			}
			_ = w.Close()
			return fmt.Errorf("read legacy file: %w", readErr)
		}
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("close segment writer: %w", err)
	}

	if err := os.Remove(legacyPath); err != nil {
		return fmt.Errorf("remove legacy file: %w", err)
	}
	log.Info("migrated legacy log file to segmented layout",
		zap.String("namespace", namespace),
		zap.String("pod", pod),
		zap.String("container", container),
		zap.Int("lines", lineCount),
	)
	return nil
}

// detectLegacyContainer returns the container name from the first
// "[ns/pod/container]" tag found in the file, or "" if none is found (e.g. a
// file containing only restart-separator lines).
func detectLegacyContainer(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		start := strings.IndexByte(line, '[')
		end := strings.IndexByte(line, ']')
		if start < 0 || end < 0 || end < start {
			continue
		}
		parts := strings.Split(line[start+1:end], "/")
		if len(parts) == 3 && parts[2] != "" {
			return parts[2], nil
		}
	}
	return "", scanner.Err()
}

// parseLegacyTimestamp extracts the leading RFC3339(Nano) timestamp from a
// legacy log line, falling back to now for lines that don't have one (e.g.
// bare "--- pod restarted at ... ---" separators, which predate this format).
func parseLegacyTimestamp(line string) time.Time {
	idx := strings.IndexByte(line, ' ')
	if idx < 0 {
		return time.Now().UTC()
	}
	ts, err := time.Parse(time.RFC3339Nano, line[:idx])
	if err != nil {
		return time.Now().UTC()
	}
	return ts
}
