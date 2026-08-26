package indexes

import (
	"bufio"
	"bytes"
	"container/heap"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
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
	indexDirName       = ".indexes"
	manifestName       = "indexes.json"
	indexFormatVersion = 2
	shardCount         = 256
	shardMagic         = "SLI2"
)

var validIndexKey = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_.-]{0,127}$`)

type manifest struct {
	Keys          []string `json:"keys"`
	FormatVersion int      `json:"formatVersion,omitempty"`
}

type Entry struct {
	Timestamp string `json:"ts"`
	Namespace string `json:"namespace"`
	Pod       string `json:"pod"`
	Value     string `json:"value"`
	Line      string `json:"line"`
	Offset    int64  `json:"-"`
	Length    uint32 `json:"-"`
}

type reference struct {
	Timestamp time.Time
	ValidTime bool
	Namespace string
	Pod       string
	Value     string
	Offset    int64
	Length    uint32
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
	mu             sync.Mutex
	logsRoot       string
	root           string
	keys           map[string]struct{}
	ready          chan struct{}
	needsMigration bool
}

func NewManager(logsRoot string) *Manager {
	m := &Manager{
		logsRoot: logsRoot,
		root:     filepath.Join(logsRoot, indexDirName),
		keys:     make(map[string]struct{}),
		ready:    make(chan struct{}),
	}
	if err := m.load(); err != nil || !m.needsMigration {
		close(m.ready)
		return m
	}
	go m.migrate()
	return m
}

func (m *Manager) migrate() {
	m.mu.Lock()
	defer m.mu.Unlock()
	defer close(m.ready)
	for _, key := range m.sortedKeysLocked() {
		if err := m.rebuildLocked(key); err != nil {
			return
		}
	}
	_ = m.saveLocked()
}

func ValidateKey(key string) error {
	if !validIndexKey.MatchString(key) {
		return fmt.Errorf("invalid index key %q", key)
	}
	return nil
}

func (m *Manager) List() []string {
	<-m.ready
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

	<-m.ready
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

	<-m.ready
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

// Compact removes references to log files that retention has deleted or
// truncated. It rewrites shards one record at a time, so compaction itself has
// constant memory usage.
func (m *Manager) Compact() error {
	<-m.ready
	m.mu.Lock()
	defer m.mu.Unlock()

	fileSizes := make(map[string]int64)
	for key := range m.keys {
		for shard := 0; shard < shardCount; shard++ {
			path := m.shardPathByNumber(key, byte(shard))
			if _, err := os.Stat(path); err != nil {
				if os.IsNotExist(err) {
					continue
				}
				return err
			}
			tempPath := path + ".compact"
			temp, err := os.Create(tempPath)
			if err != nil {
				return err
			}
			retained := 0
			err = m.scanShardLocked(key, byte(shard), func(ref reference) error {
				if !m.referenceAvailable(ref, fileSizes) {
					return nil
				}
				record, encodeErr := encodeReference(ref, retained == 0)
				if encodeErr != nil {
					return encodeErr
				}
				if _, writeErr := temp.Write(record); writeErr != nil {
					return writeErr
				}
				retained++
				return nil
			})
			closeErr := temp.Close()
			if err != nil || closeErr != nil {
				_ = os.Remove(tempPath)
				if err != nil {
					return err
				}
				return closeErr
			}
			if retained == 0 {
				_ = os.Remove(tempPath)
				if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
					return err
				}
				continue
			}
			if err := os.Rename(tempPath, path); err != nil {
				_ = os.Remove(tempPath)
				return err
			}
		}
	}
	return nil
}

// ObserveLineAt indexes a line already written to its canonical pod log.
func (m *Manager) ObserveLineAt(namespace, pod string, offset int64, length uint32, line string) {
	<-m.ready
	m.mu.Lock()
	defer m.mu.Unlock()

	if len(m.keys) == 0 {
		return
	}
	for key := range m.keys {
		if value, ok := indexedValue(line, key); ok {
			timestamp, timestampErr := time.Parse(time.RFC3339, lineTimestamp(line))
			_ = m.appendReferenceLocked(key, reference{
				Timestamp: timestamp,
				ValidTime: timestampErr == nil,
				Namespace: namespace,
				Pod:       pod,
				Value:     value,
				Offset:    offset,
				Length:    length,
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

	<-m.ready
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

	entries, total, err := m.readReferencePageLocked(key, value, start, pageSize, loadLastPage)
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
		line, readErr := m.readReferencedLine(entry)
		if readErr == nil {
			lines = append(lines, line)
		}
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

	<-m.ready
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
	fileSizes := make(map[string]int64)
	for shard := 0; shard < shardCount; shard++ {
		byValue := make(map[string]ValueInfo)
		err := m.scanShardLocked(key, byte(shard), func(ref reference) error {
			if !m.referenceAvailable(ref, fileSizes) {
				return nil
			}
			info := byValue[ref.Value]
			info.Value = ref.Value
			info.Count++
			if ref.ValidTime && ref.Timestamp.After(info.LastUpdated) {
				info.LastUpdated = ref.Timestamp
			}
			byValue[ref.Value] = info
			return nil
		})
		if err != nil {
			return nil, "", "", err
		}
		for _, info := range byValue {
			valueCount++
			heap.Push(h, info)
			if h.Len() > limit {
				heap.Pop(h)
			}
		}
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
	m.needsMigration = mf.FormatVersion != indexFormatVersion
	return nil
}

func (m *Manager) saveLocked() error {
	keys := m.sortedKeysLocked()

	if err := os.MkdirAll(m.root, 0755); err != nil {
		return err
	}
	f, err := os.Create(filepath.Join(m.root, manifestName))
	if err != nil {
		return err
	}
	defer f.Close()
	return json.NewEncoder(f).Encode(manifest{Keys: keys, FormatVersion: indexFormatVersion})
}

func (m *Manager) sortedKeysLocked() []string {
	keys := make([]string, 0, len(m.keys))
	for key := range m.keys {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
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
	var offset int64
	for {
		rawLine, readErr := reader.ReadString('\n')
		if len(rawLine) == 0 && errors.Is(readErr, io.EOF) {
			break
		}
		if readErr != nil && !errors.Is(readErr, io.EOF) {
			return readErr
		}
		line := strings.TrimRight(rawLine, "\r\n")
		value, ok := indexedValue(line, key)
		if ok {
			timestamp, timestampErr := time.Parse(time.RFC3339, lineTimestamp(line))
			if err := m.appendReferenceLocked(key, reference{
				Timestamp: timestamp,
				ValidTime: timestampErr == nil,
				Namespace: namespace,
				Pod:       pod,
				Value:     value,
				Offset:    offset,
				Length:    uint32(len(line)),
			}); err != nil {
				return err
			}
		}
		offset += int64(len(rawLine))
		if errors.Is(readErr, io.EOF) {
			break
		}
	}
	return nil
}

func (m *Manager) appendReferenceLocked(key string, ref reference) error {
	path := m.shardPath(key, ref.Value)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return err
	}
	record, err := encodeReference(ref, info.Size() == 0)
	if err != nil {
		return err
	}
	_, err = f.Write(record)
	return err
}

func encodeReference(ref reference, includeMagic bool) ([]byte, error) {
	var record bytes.Buffer
	if includeMagic {
		record.WriteString(shardMagic)
	}
	namespace := []byte(ref.Namespace)
	pod := []byte(ref.Pod)
	value := []byte(ref.Value)
	bodyLength := 33 + len(namespace) + len(pod) + len(value)
	if bodyLength > int(^uint32(0)) {
		return nil, errors.New("index reference is too large")
	}
	_ = binary.Write(&record, binary.BigEndian, uint32(bodyLength))
	if ref.ValidTime {
		record.WriteByte(1)
	} else {
		record.WriteByte(0)
	}
	_ = binary.Write(&record, binary.BigEndian, ref.Timestamp.UnixNano())
	_ = binary.Write(&record, binary.BigEndian, ref.Offset)
	_ = binary.Write(&record, binary.BigEndian, ref.Length)
	_ = binary.Write(&record, binary.BigEndian, uint32(len(namespace)))
	_ = binary.Write(&record, binary.BigEndian, uint32(len(pod)))
	_ = binary.Write(&record, binary.BigEndian, uint32(len(value)))
	record.Write(namespace)
	record.Write(pod)
	record.Write(value)
	return record.Bytes(), nil
}

func (m *Manager) scanShardLocked(key string, shard byte, visit func(reference) error) error {
	f, err := os.Open(m.shardPathByNumber(key, shard))
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	defer f.Close()

	reader := bufio.NewReader(f)
	magic := make([]byte, len(shardMagic))
	if _, err := io.ReadFull(reader, magic); err != nil {
		return err
	}
	if string(magic) != shardMagic {
		return errors.New("unsupported index shard format")
	}
	for {
		var bodyLength uint32
		if err := binary.Read(reader, binary.BigEndian, &bodyLength); err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
				return nil
			}
			return err
		}
		if bodyLength < 33 || bodyLength > 64*1024*1024 {
			return fmt.Errorf("invalid index record length %d", bodyLength)
		}
		body := make([]byte, bodyLength)
		if _, err := io.ReadFull(reader, body); err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
				return nil
			}
			return err
		}
		validTime := body[0] == 1
		timestampNanos := int64(binary.BigEndian.Uint64(body[1:9]))
		offset := int64(binary.BigEndian.Uint64(body[9:17]))
		lineLength := binary.BigEndian.Uint32(body[17:21])
		namespaceLength := binary.BigEndian.Uint32(body[21:25])
		podLength := binary.BigEndian.Uint32(body[25:29])
		valueLength := binary.BigEndian.Uint32(body[29:33])
		payloadLength := uint64(namespaceLength) + uint64(podLength) + uint64(valueLength)
		if payloadLength != uint64(bodyLength-33) {
			return errors.New("invalid index record payload")
		}
		pos := uint32(33)
		namespace := string(body[pos : pos+namespaceLength])
		pos += namespaceLength
		pod := string(body[pos : pos+podLength])
		pos += podLength
		value := string(body[pos : pos+valueLength])
		if err := visit(reference{
			Timestamp: time.Unix(0, timestampNanos), ValidTime: validTime,
			Namespace: namespace, Pod: pod, Value: value,
			Offset: offset, Length: lineLength,
		}); err != nil {
			return err
		}
	}
}

func (m *Manager) readReferencePageLocked(key, value string, start, pageSize int, loadLastPage bool) ([]Entry, int, error) {
	limit := pageSize
	if !loadLastPage {
		limit += start
	}
	h := &entryPageHeap{keepNewest: loadLastPage}
	heap.Init(h)
	total := 0
	fileSizes := make(map[string]int64)
	err := m.scanShardLocked(key, shardNumber(value), func(ref reference) error {
		if ref.Value != value {
			return nil
		}
		path := filepath.Join(m.logsRoot, ref.Namespace, ref.Pod+".log")
		size, checked := fileSizes[path]
		if !checked {
			info, statErr := os.Stat(path)
			if statErr != nil {
				fileSizes[path] = -1
				return nil
			}
			size = info.Size()
			fileSizes[path] = size
		}
		if size < 0 || ref.Offset < 0 || ref.Offset+int64(ref.Length) > size {
			return nil
		}
		entry := Entry{
			Timestamp: ref.Timestamp.Format(time.RFC3339Nano), Namespace: ref.Namespace,
			Pod: ref.Pod, Value: ref.Value, Offset: ref.Offset, Length: ref.Length,
		}
		heap.Push(h, sortableEntry{
			entry: entry, position: total, time: ref.Timestamp, valid: ref.ValidTime,
		})
		total++
		if h.Len() > limit {
			heap.Pop(h)
		}
		return nil
	})
	if err != nil {
		return nil, 0, err
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

func (m *Manager) readReferencedLine(entry Entry) (string, error) {
	f, err := os.Open(filepath.Join(m.logsRoot, entry.Namespace, entry.Pod+".log"))
	if err != nil {
		return "", err
	}
	defer f.Close()
	line := make([]byte, entry.Length)
	if _, err := f.ReadAt(line, entry.Offset); err != nil {
		return "", err
	}
	return string(line), nil
}

func (m *Manager) referenceAvailable(ref reference, fileSizes map[string]int64) bool {
	path := filepath.Join(m.logsRoot, ref.Namespace, ref.Pod+".log")
	size, checked := fileSizes[path]
	if !checked {
		info, err := os.Stat(path)
		if err != nil {
			fileSizes[path] = -1
			return false
		}
		size = info.Size()
		fileSizes[path] = size
	}
	return size >= 0 && ref.Offset >= 0 && ref.Offset+int64(ref.Length) <= size
}

func (m *Manager) shardPath(key, value string) string {
	return m.shardPathByNumber(key, shardNumber(value))
}

func (m *Manager) shardPathByNumber(key string, shard byte) string {
	return filepath.Join(m.keyRoot(key), "shards", fmt.Sprintf("%02x.idx", shard))
}

func shardNumber(value string) byte {
	return sha256.Sum256([]byte(value))[0]
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
