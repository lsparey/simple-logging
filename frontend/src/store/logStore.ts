import { useMemo } from 'react';
import { create } from 'zustand';

export type DisplayMode = 'idle' | 'loading' | 'history' | 'live';

export interface JsonFormat {
  timestampKey?: string;
  levelKey?: string;
  messageKey?: string;
}

interface LogStore {
  // Pod selection
  selectedNamespace: string | null;
  selectedPod: string | null;
  setSelectedPod: (namespace: string, pod: string, jsonLogging?: boolean) => void;

  // Deployment selection (mutually exclusive with pod selection)
  selectedDeployment: string | null;
  setSelectedDeployment: (namespace: string, deployment: string, jsonLogging?: boolean) => void;

  // Whether the currently selected pod/deployment uses JSON log formatting
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

  // Dark mode
  darkMode: boolean;
  toggleDarkMode: () => void;

  // JSON log format configuration (per pod/deployment, keyed by makeFormatKey())
  jsonFormats: Record<string, JsonFormat>;
  setJsonFormat: (key: string, format: JsonFormat | null) => void;
}

const stored = localStorage.getItem('simple-logging.theme');
const storedJsonFormats = localStorage.getItem('simple-logging.jsonFormats');
let initialJsonFormats: Record<string, JsonFormat> = {};
try {
  if (storedJsonFormats) initialJsonFormats = JSON.parse(storedJsonFormats) as Record<string, JsonFormat>;
} catch { /* ignore */ }

export const useLogStore = create<LogStore>((set) => ({
  selectedNamespace: null,
  selectedPod: null,
  selectionKey: 0,
  jsonLogging: false,

  setSelectedPod: (namespace, pod, jsonLogging = false) =>
    set((s) => ({
      selectedNamespace: namespace,
      selectedPod: pod,
      selectedDeployment: null,
      jsonLogging,
      mode: 'loading',
      lines: [],
      prevPageToken: '',
      nextPageToken: '',
      searchText: '',
      startTime: 0,
      endTime: 0,
      selectionKey: s.selectionKey + 1,
    })),

  selectedDeployment: null,
  setSelectedDeployment: (namespace, deployment, jsonLogging = false) =>
    set((s) => ({
      selectedNamespace: namespace,
      selectedPod: null,
      selectedDeployment: deployment,
      jsonLogging,
      mode: 'loading',
      lines: [],
      prevPageToken: '',
      nextPageToken: '',
      searchText: '',
      startTime: 0,
      endTime: 0,
      selectionKey: s.selectionKey + 1,
    })),

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
export function makeFormatKey(
  namespace: string,
  pod?: string | null,
  deployment?: string | null,
): string {
  if (pod) return `pod:${namespace}/${pod}`;
  if (deployment) return `deployment:${namespace}/${deployment}`;
  return '';
}

/** Derived: lines filtered by current searchText */
export function useFilteredLines(): string[] {
  const lines = useLogStore((s) => s.lines);
  const searchText = useLogStore((s) => s.searchText);
  return useMemo(() => {
    if (!searchText) return lines;
    const lower = searchText.toLowerCase();
    return lines.filter((l) => l.toLowerCase().includes(lower));
  }, [lines, searchText]);
}
