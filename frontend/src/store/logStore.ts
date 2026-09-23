import { useMemo } from 'react';
import { create } from 'zustand';

export type DisplayMode = 'idle' | 'loading' | 'history' | 'live';

export interface JsonFormat {
  timestampKey?: string;
  levelKey?: string;
  messageKey?: string;
}

interface LogStore {
  // Workload selection: a workload is (namespace, kind, name), e.g.
  // ("default", "Deployment", "web-app") or ("default", "Pod", "standalone").
  // Every pod belongs to exactly one workload (a bare pod is its own
  // singleton "Pod"-kind workload), so this is the one selection concept the
  // log view needs.
  selectedNamespace: string | null;
  selectedWorkloadKind: string | null;
  selectedWorkloadName: string | null;
  setSelectedWorkload: (namespace: string, kind: string, name: string, jsonLogging?: boolean) => void;

  // Client-side container filter over the merged workload log view, applied
  // to already-fetched lines by parsing each line's "[ns/pod/container]"
  // tag. null means "no filter" (show every container). There is no
  // equivalent pod filter: browsing a single pod's own logs is done by
  // selecting that pod directly in the sidebar (kind "Pod"), not by
  // filtering a multi-pod workload's merged view down to one pod.
  selectedContainerFilter: string | null;
  setSelectedContainerFilter: (container: string | null) => void;

  // Index selection (mutually exclusive with workload selection)
  selectedIndexKey: string | null;
  selectedIndexValue: string;
  enterIndexMode: () => void;
  leaveIndexMode: () => void;
  setSelectedIndex: (key: string) => void;
  setSelectedIndexValue: (value: string) => void;
  indexListVersion: number;
  refreshIndexList: () => void;

  // Whether the currently selected workload/index uses JSON log formatting
  jsonLogging: boolean;
  // Update jsonLogging without changing the current selection (used by live polling)
  setJsonLogging: (v: boolean) => void;

  // Increments on every selection (even re-selecting the same resource)
  selectionKey: number;

  // Display mode
  mode: DisplayMode;
  setMode: (mode: DisplayMode) => void;

  // Log lines
  lines: string[];
  appendLines: (lines: string[]) => void;
  prependLines: (lines: string[]) => void;
  setLines: (lines: string[]) => void;
  clearLines: () => void;

  // Pagination cursors
  prevPageToken: string;
  nextPageToken: string;
  setPaginationTokens: (prev: string, next: string) => void;
  setNextPageToken: (token: string) => void;

  // Infinite scroll fetch state
  isFetchingMore: boolean;
  setIsFetchingMore: (fetching: boolean) => void;

  // Filters
  searchText: string;
  setSearchText: (text: string) => void;
  startTime: number; // Unix seconds, 0 = unset
  endTime: number;
  setTimeRange: (start: number, end: number) => void;

  // Timestamp (ms) of the first visible log line in the current scroll view; 0 = unknown
  visibleTimestamp: number;
  setVisibleTimestamp: (t: number) => void;

  // Dark mode
  darkMode: boolean;
  toggleDarkMode: () => void;

  // JSON log format configuration (per workload or index)
  jsonFormats: Record<string, JsonFormat>;
  setJsonFormat: (key: string, format: JsonFormat | null) => void;
}

const stored = localStorage.getItem('simple-logging.theme');
const storedJsonFormats = localStorage.getItem('simple-logging.jsonFormats');
let initialJsonFormats: Record<string, JsonFormat> = {};
try {
  if (storedJsonFormats) initialJsonFormats = JSON.parse(storedJsonFormats) as Record<string, JsonFormat>;
} catch { /* ignore */ }

// Fields reset by every selection change (workload, index, or leaving both).
const RESET_ON_SELECT = {
  lines: [] as string[],
  prevPageToken: '',
  nextPageToken: '',
  searchText: '',
  startTime: 0,
  endTime: 0,
  visibleTimestamp: 0,
};

export const useLogStore = create<LogStore>((set) => ({
  selectedNamespace: null,
  selectedWorkloadKind: null,
  selectedWorkloadName: null,
  selectionKey: 0,
  jsonLogging: false,

  setSelectedWorkload: (namespace, kind, name, jsonLogging = false) =>
    set((s) => ({
      ...RESET_ON_SELECT,
      selectedNamespace: namespace,
      selectedWorkloadKind: kind,
      selectedWorkloadName: name,
      selectedContainerFilter: null,
      selectedIndexKey: null,
      selectedIndexValue: '',
      jsonLogging,
      mode: 'loading',
      selectionKey: s.selectionKey + 1,
    })),

  selectedContainerFilter: null,
  setSelectedContainerFilter: (container) => set({ selectedContainerFilter: container }),

  selectedIndexKey: null,
  selectedIndexValue: '',
  indexListVersion: 0,
  enterIndexMode: () =>
    set((s) => ({
      ...RESET_ON_SELECT,
      selectedNamespace: null,
      selectedWorkloadKind: null,
      selectedWorkloadName: null,
      selectedContainerFilter: null,
      selectedIndexKey: '',
      selectedIndexValue: '',
      jsonLogging: false,
      mode: 'idle',
      selectionKey: s.selectionKey + 1,
    })),
  leaveIndexMode: () =>
    set({
      selectedIndexKey: null,
      selectedIndexValue: '',
      lines: [],
      prevPageToken: '',
      nextPageToken: '',
      searchText: '',
      visibleTimestamp: 0,
    }),
  setSelectedIndex: (key) =>
    set((s) => ({
      ...RESET_ON_SELECT,
      selectedNamespace: null,
      selectedWorkloadKind: null,
      selectedWorkloadName: null,
      selectedContainerFilter: null,
      selectedIndexKey: key,
      selectedIndexValue: '',
      jsonLogging: false,
      mode: 'idle',
      selectionKey: s.selectionKey + 1,
    })),
  setSelectedIndexValue: (value) =>
    set({
      selectedIndexValue: value,
      mode: value ? 'loading' : 'idle',
      lines: [],
      prevPageToken: '',
      nextPageToken: '',
      searchText: '',
      visibleTimestamp: 0,
    }),
  refreshIndexList: () => set((s) => ({ indexListVersion: s.indexListVersion + 1 })),

  mode: 'idle',
  setMode: (mode) => set({ mode }),

  lines: [],
  appendLines: (newLines) =>
    set((s) => ({ lines: [...s.lines, ...newLines] })),
  prependLines: (newLines) =>
    set((s) => ({ lines: [...newLines, ...s.lines] })),
  setLines: (lines) => set({ lines }),
  clearLines: () => set({ lines: [] }),

  prevPageToken: '',
  nextPageToken: '',
  setPaginationTokens: (prev, next) =>
    set({ prevPageToken: prev, nextPageToken: next }),
  setNextPageToken: (token) => set({ nextPageToken: token }),

  isFetchingMore: false,
  setIsFetchingMore: (isFetchingMore) => set({ isFetchingMore }),

  searchText: '',
  setSearchText: (searchText) => set({ searchText }),

  startTime: 0,
  endTime: 0,
  setTimeRange: (startTime, endTime) =>
    set({ startTime, endTime, lines: [], prevPageToken: '', nextPageToken: '' }),

  visibleTimestamp: 0,
  setVisibleTimestamp: (visibleTimestamp) => set({ visibleTimestamp }),

  darkMode: stored === 'dark',
  toggleDarkMode: () =>
    set((s) => {
      const next = !s.darkMode;
      localStorage.setItem('simple-logging.theme', next ? 'dark' : 'light');
      return { darkMode: next };
    }),

  jsonFormats: initialJsonFormats,
  setJsonFormat: (key, format) =>
    set((s) => {
      const next = { ...s.jsonFormats };
      if (format) {
        next[key] = format;
      } else {
        delete next[key];
      }
      localStorage.setItem('simple-logging.jsonFormats', JSON.stringify(next));
      return { jsonFormats: next };
    }),

  setJsonLogging: (v) => set({ jsonLogging: v }),
}));

/** Build the per-resource key used to store jsonFormats entries. */
export function makeFormatKey(namespace: string, kind: string, name: string): string {
  return `workload:${namespace}/${kind}/${name}`;
}

export function makeIndexFormatKey(indexKey: string): string {
  return `index:${indexKey}`;
}

/** Extracts the container name from a line's "[ns/pod/container]" tag, or null. */
export function lineContainer(line: string): string | null {
  const start = line.indexOf('[');
  const end = line.indexOf(']', start);
  if (start < 0 || end < 0) return null;
  const parts = line.slice(start + 1, end).split('/');
  return parts.length === 3 ? parts[2] : null;
}

/** Derived: lines filtered by current searchText and selectedContainerFilter */
export function useFilteredLines(): string[] {
  const lines = useLogStore((s) => s.lines);
  const searchText = useLogStore((s) => s.searchText);
  const containerFilter = useLogStore((s) => s.selectedContainerFilter);
  return useMemo(() => {
    let result = lines;
    if (containerFilter) {
      result = result.filter((l) => lineContainer(l) === containerFilter);
    }
    if (searchText) {
      const lower = searchText.toLowerCase();
      result = result.filter((l) => l.toLowerCase().includes(lower));
    }
    return result;
  }, [lines, searchText, containerFilter]);
}
