package indexes

import (
	"bufio"
	"container/heap"
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
	Value       string
	Count       int64
	LastUpdated time.Time
}

type valueInfoHeap struct{ values []ValueInfo }

func (h valueInfoHeap) Len() int           { return len(h.values) }
func (h valueInfoHeap) Less(i, j int) bool { return valueInfoLess(h.values[j], h.values[i]) }
func (h valueInfoHeap) Swap(i, j int)      { h.values[i], h.values[j] = h.values[j], h.values[i] }
func (h *valueInfoHeap) Push(value any)    { h.values = append(h.values, value.(ValueInfo)) }
func (h *valueInfoHeap) Pop() any {
	old := h.values
	value := old[len(old)-1]
	h.values = old[:len(old)-1]
	return value
}

func valueInfoLess(a, b ValueInfo) bool {
	if a.LastUpdated.Equal(b.LastUpdated) {
		return a.Value < b.Value
	}
	return a.LastUpdated.After(b.LastUpdated)
}

type sortableEntry struct {
	entry    Entry
	position int
	time     time.Time
	valid    bool
}

type entryPageHeap struct {
	entries    []sortableEntry
	keepNewest bool
}

func (h entryPageHeap) Len() int { return len(h.entries) }
func (h entryPageHeap) Less(i, j int) bool {
	less := sortableEntryLess(h.entries[i], h.entries[j])
	if h.keepNewest {
		return less
	}
	return !less
}
func (h entryPageHeap) Swap(i, j int)   { h.entries[i], h.entries[j] = h.entries[j], h.entries[i] }
func (h *entryPageHeap) Push(value any) { h.entries = append(h.entries, value.(sortableEntry)) }
func (h *entryPageHeap) Pop() any {
	old := h.entries
	value := old[len(old)-1]
	h.entries = old[:len(old)-1]
	return value
}

func sortableEntryLess(a, b sortableEntry) bool {
	if a.valid != b.valid {
		return a.valid
	}
	if a.valid && !a.time.Equal(b.time) {
		return a.time.Before(b.time)
	}
	return a.position < b.position
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

	start := 0
	if !loadLastPage && pageToken != "" {
		var err error
		start, err = strconv.Atoi(pageToken)
		if err != nil || start < 0 {
			return nil, "", "", errors.New("invalid page_token")
		}
	}

	entries, total, err := readEntriesPage(m.valuePath(key, value), start, pageSize, loadLastPage)
	if err != nil {
		return nil, "", "", err
	}
	if start > total {
		return nil, "", "", errors.New("invalid page_token")
	}
	if loadLastPage {
		start = total - len(entries)
	}

	lines := make([]string, 0, len(entries))
	for _, entry := range entries {
		lines = append(lines, entry.Line)
	}

	next := ""
	end := start + len(entries)
	if end < total {
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

func (m *Manager) ListValues(key string, pageSize int, pageToken string) ([]ValueInfo, string, string, error) {
	if err := ValidateKey(key); err != nil {
		return nil, "", "", err
	}
	if pageSize <= 0 {
		pageSize = 50
	}
	if pageSize > 200 {
		pageSize = 200
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.keys[key]; !exists {
		return nil, "", "", os.ErrNotExist
	}

	start := 0
	if pageToken != "" {
		var err error
		start, err = strconv.Atoi(pageToken)
		if err != nil || start < 0 {
			return nil, "", "", errors.New("invalid page_token")
		}
	}
	limit := start + pageSize
	h := &valueInfoHeap{}
	heap.Init(h)
	valueCount := 0
	valuesRoot := filepath.Join(m.keyRoot(key), "values")
	if err := filepath.WalkDir(valuesRoot, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || filepath.Ext(path) != ".jsonl" {
			return nil
		}
		info, ok, err := summarizeEntriesFile(path)
		if err != nil {
			return err
		}
		if !ok {
			return nil
		}
		valueCount++
		heap.Push(h, info)
		if h.Len() > limit {
			heap.Pop(h)
		}
		return nil
	}); err != nil {
		if os.IsNotExist(err) {
			return []ValueInfo{}, "", "", nil
		}
		return nil, "", "", err
	}

	if start > valueCount {
		return nil, "", "", errors.New("invalid page_token")
	}
	values := h.values
	sort.Slice(values, func(i, j int) bool { return valueInfoLess(values[i], values[j]) })
	if start > len(values) {
		start = len(values)
	}
	page := values[start:]

	next := ""
	end := start + len(page)
	if end < valueCount {
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
	return page, next, prev, nil
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

func readEntriesPage(path string, start, pageSize int, loadLastPage bool) ([]Entry, int, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, 0, nil
		}
		return nil, 0, err
	}
	defer f.Close()

	limit := pageSize
	if !loadLastPage {
		limit += start
	}
	h := &entryPageHeap{keepNewest: loadLastPage}
	heap.Init(h)
	reader := bufio.NewReader(f)
	total := 0
	for {
		line, readErr := readLine(reader)
		if readErr != nil {
			if errors.Is(readErr, io.EOF) {
				break
			}
			return nil, 0, readErr
		}
		var entry Entry
		if json.Unmarshal([]byte(line), &entry) != nil {
			continue
		}
		parsed, parseErr := time.Parse(time.RFC3339, entry.Timestamp)
		heap.Push(h, sortableEntry{
			entry: entry, position: total, time: parsed, valid: parseErr == nil,
		})
		total++
		if h.Len() > limit {
			heap.Pop(h)
		}
	}

	selected := h.entries
	sort.Slice(selected, func(i, j int) bool { return sortableEntryLess(selected[i], selected[j]) })
	if !loadLastPage {
		if start > len(selected) {
			start = len(selected)
		}
		selected = selected[start:]
	}
	entries := make([]Entry, len(selected))
	for i := range selected {
		entries[i] = selected[i].entry
	}
	return entries, total, nil
}

func summarizeEntriesFile(path string) (ValueInfo, bool, error) {
	f, err := os.Open(path)
	if err != nil {
		return ValueInfo{}, false, err
	}
	defer f.Close()

	var info ValueInfo
	reader := bufio.NewReader(f)
	for {
		line, readErr := readLine(reader)
		if readErr != nil {
			if errors.Is(readErr, io.EOF) {
				break
			}
			return ValueInfo{}, false, readErr
		}
		var entry Entry
		if json.Unmarshal([]byte(line), &entry) != nil {
			continue
		}
		if info.Count == 0 {
			info.Value = entry.Value
		}
		info.Count++
		if timestamp, parseErr := time.Parse(time.RFC3339, entry.Timestamp); parseErr == nil && timestamp.After(info.LastUpdated) {
			info.LastUpdated = timestamp
		}
	}
	return info, info.Count > 0, nil
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
