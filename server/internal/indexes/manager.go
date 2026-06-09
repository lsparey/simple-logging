package indexes

import (
	"bufio"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	indexDirName = ".indexes"
	manifestName = "indexes.json"
)

var validIndexKey = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_.-]{0,127}$`)

type manifest struct {
	Keys []string `json:"keys"`
}

type Entry struct {
	Timestamp string `json:"ts"`
	Namespace string `json:"namespace"`
	Pod       string `json:"pod"`
	Value     string `json:"value"`
	Line      string `json:"line"`
}

type ValueInfo struct {
	Value string
	Count int64
}

type Manager struct {
	mu       sync.Mutex
	logsRoot string
	root     string
	keys     map[string]struct{}
}

func NewManager(logsRoot string) *Manager {
	m := &Manager{
		logsRoot: logsRoot,
		root:     filepath.Join(logsRoot, indexDirName),
		keys:     make(map[string]struct{}),
	}
	_ = m.load()
	return m
}

func ValidateKey(key string) error {
	if !validIndexKey.MatchString(key) {
		return fmt.Errorf("invalid index key %q", key)
	}
	return nil
}

func (m *Manager) List() []string {
	m.mu.Lock()
	defer m.mu.Unlock()

	keys := make([]string, 0, len(m.keys))
	for key := range m.keys {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func (m *Manager) Create(key string) error {
	if err := ValidateKey(key); err != nil {
		return err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if err := os.MkdirAll(m.root, 0755); err != nil {
		return fmt.Errorf("create index root: %w", err)
	}
	if _, exists := m.keys[key]; exists {
		return nil
	}
	m.keys[key] = struct{}{}
	if err := m.saveLocked(); err != nil {
		delete(m.keys, key)
		return err
	}
	if err := m.rebuildLocked(key); err != nil {
		delete(m.keys, key)
		_ = m.saveLocked()
		return err
	}
	return nil
}

func (m *Manager) Delete(key string) error {
	if err := ValidateKey(key); err != nil {
		return err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.keys[key]; !exists {
		return os.ErrNotExist
	}
	delete(m.keys, key)
	if err := m.saveLocked(); err != nil {
		m.keys[key] = struct{}{}
		return err
	}
	return os.RemoveAll(m.keyRoot(key))
}

func (m *Manager) ObserveLine(namespace, pod, line string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if len(m.keys) == 0 {
		return
	}
	for key := range m.keys {
		if value, ok := indexedValue(line, key); ok {
			_ = m.appendLocked(key, Entry{
				Timestamp: lineTimestamp(line),
				Namespace: namespace,
				Pod:       pod,
				Value:     value,
				Line:      line,
			})
		}
	}
}

func (m *Manager) GetLogs(key, value string, pageSize int, pageToken string, loadLastPage bool) ([]string, string, string, error) {
	if err := ValidateKey(key); err != nil {
		return nil, "", "", err
	}
	if pageSize <= 0 {
		pageSize = 200
	}
	if pageSize > 1000 {
		pageSize = 1000
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.keys[key]; !exists {
		return nil, "", "", os.ErrNotExist
	}

	entries, err := m.readValueEntriesLocked(key, value)
	if err != nil {
		return nil, "", "", err
	}
	sortEntriesByTimestamp(entries)

	start := 0
	if loadLastPage {
		start = len(entries) - pageSize
		if start < 0 {
			start = 0
		}
	} else if pageToken != "" {
		start, err = strconv.Atoi(pageToken)
		if err != nil || start < 0 || start > len(entries) {
			return nil, "", "", errors.New("invalid page_token")
		}
	}

	end := start + pageSize
	if end > len(entries) {
		end = len(entries)
	}

	lines := make([]string, 0, end-start)
	for _, entry := range entries[start:end] {
		lines = append(lines, entry.Line)
	}

	next := ""
	if end < len(entries) {
		next = strconv.Itoa(end)
	}
	prev := ""
	if start > 0 {
		prevStart := start - pageSize
		if prevStart < 0 {
			prevStart = 0
		}
		prev = strconv.Itoa(prevStart)
	}

	return lines, next, prev, nil
}

func sortEntriesByTimestamp(entries []Entry) {
	type timedEntry struct {
		entry Entry
		time  time.Time
		valid bool
	}

	timed := make([]timedEntry, len(entries))
	for i, entry := range entries {
		parsed, err := time.Parse(time.RFC3339, entry.Timestamp)
		timed[i] = timedEntry{entry: entry, time: parsed, valid: err == nil}
	}
	sort.SliceStable(timed, func(i, j int) bool {
		if timed[i].valid != timed[j].valid {
			return timed[i].valid
		}
		return timed[i].valid && timed[i].time.Before(timed[j].time)
	})
	for i := range timed {
		entries[i] = timed[i].entry
	}
}

func (m *Manager) ListValues(key string) ([]ValueInfo, error) {
	if err := ValidateKey(key); err != nil {
		return nil, err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.keys[key]; !exists {
		return nil, os.ErrNotExist
	}

	counts := make(map[string]int64)
	valuesRoot := filepath.Join(m.keyRoot(key), "values")
	if err := filepath.WalkDir(valuesRoot, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || filepath.Ext(path) != ".jsonl" {
			return nil
		}
		entries, err := readEntriesFile(path)
		if err != nil {
			return err
		}
		for _, entry := range entries {
			counts[entry.Value]++
		}
		return nil
	}); err != nil {
		if os.IsNotExist(err) {
			return []ValueInfo{}, nil
		}
		return nil, err
	}

	values := make([]ValueInfo, 0, len(counts))
	for value, count := range counts {
		values = append(values, ValueInfo{Value: value, Count: count})
	}
	sort.Slice(values, func(i, j int) bool {
		if values[i].Count == values[j].Count {
			return values[i].Value < values[j].Value
		}
		return values[i].Count > values[j].Count
	})
	return values, nil
}

func (m *Manager) load() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	f, err := os.Open(filepath.Join(m.root, manifestName))
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	defer f.Close()

	var mf manifest
	if err := json.NewDecoder(f).Decode(&mf); err != nil {
		return err
	}
	for _, key := range mf.Keys {
		if ValidateKey(key) == nil {
			m.keys[key] = struct{}{}
		}
	}
	return nil
}

func (m *Manager) saveLocked() error {
	keys := make([]string, 0, len(m.keys))
	for key := range m.keys {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	if err := os.MkdirAll(m.root, 0755); err != nil {
		return err
	}
	f, err := os.Create(filepath.Join(m.root, manifestName))
	if err != nil {
		return err
	}
	defer f.Close()
	return json.NewEncoder(f).Encode(manifest{Keys: keys})
}

func (m *Manager) rebuildLocked(key string) error {
	if err := os.RemoveAll(m.keyRoot(key)); err != nil {
		return err
	}

	namespaces, err := os.ReadDir(m.logsRoot)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	for _, ns := range namespaces {
		if !ns.IsDir() || strings.HasPrefix(ns.Name(), ".") {
			continue
		}
		nsDir := filepath.Join(m.logsRoot, ns.Name())
		pods, err := os.ReadDir(nsDir)
		if err != nil {
			return err
		}
		for _, podFile := range pods {
			if podFile.IsDir() || filepath.Ext(podFile.Name()) != ".log" {
				continue
			}
			pod := strings.TrimSuffix(podFile.Name(), ".log")
			if err := m.backfillFileLocked(key, ns.Name(), pod, filepath.Join(nsDir, podFile.Name())); err != nil {
				return err
			}
		}
	}
	return nil
}

func (m *Manager) backfillFileLocked(key, namespace, pod, path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	reader := bufio.NewReader(f)
	for {
		line, err := readLine(reader)
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return err
		}
		value, ok := indexedValue(line, key)
		if !ok {
			continue
		}
		if err := m.appendLocked(key, Entry{
			Timestamp: lineTimestamp(line),
			Namespace: namespace,
			Pod:       pod,
			Value:     value,
			Line:      line,
		}); err != nil {
			return err
		}
	}
	return nil
}

func (m *Manager) appendLocked(key string, entry Entry) error {
	path := m.valuePath(key, entry.Value)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	return json.NewEncoder(f).Encode(entry)
}

func (m *Manager) readValueEntriesLocked(key, value string) ([]Entry, error) {
	path := m.valuePath(key, value)
	return readEntriesFile(path)
}

func readEntriesFile(path string) ([]Entry, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()

	var entries []Entry
	reader := bufio.NewReader(f)
	for {
		line, err := readLine(reader)
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, err
		}
		var entry Entry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue
		}
		entries = append(entries, entry)
	}
	return entries, nil
}

func readLine(r *bufio.Reader) (string, error) {
	line, err := r.ReadString('\n')
	if len(line) > 0 {
		return strings.TrimRight(line, "\r\n"), nil
	}
	return "", err
}

func (m *Manager) keyRoot(key string) string {
	return filepath.Join(m.root, "keys", encodePathPart(key))
}

func (m *Manager) valuePath(key, value string) string {
	digest := valueDigest(value)
	return filepath.Join(m.keyRoot(key), "values", digest[:2], digest+".jsonl")
}

func encodePathPart(s string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(s))
}

func valueDigest(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func indexedValue(line, key string) (string, bool) {
	payload := jsonPayload(line)
	if payload == "" {
		return "", false
	}
	var obj map[string]any
	if err := json.Unmarshal([]byte(payload), &obj); err != nil {
		return "", false
	}
	value, ok := obj[key]
	if !ok || value == nil {
		return "", false
	}
	switch v := value.(type) {
	case string:
		return v, true
	case bool:
		return strconv.FormatBool(v), true
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64), true
	default:
		return "", false
	}
}

func jsonPayload(line string) string {
	idx := strings.Index(line, "] ")
	if idx >= 0 {
		line = line[idx+2:]
	}
	trimmed := strings.TrimSpace(line)
	if !strings.HasPrefix(trimmed, "{") {
		return ""
	}
	return trimmed
}

func lineTimestamp(line string) string {
	idx := strings.IndexByte(line, ' ')
	if idx < 0 {
		return ""
	}
	return line[:idx]
}
