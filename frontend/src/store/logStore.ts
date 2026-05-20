import { create } from 'zustand';

export type DisplayMode = 'idle' | 'loading' | 'history' | 'live';

interface LogStore {
  // Pod selection
  selectedNamespace: string | null;
  selectedPod: string | null;
  setSelectedPod: (namespace: string, pod: string) => void;

  // Display mode
  mode: DisplayMode;
  setMode: (mode: DisplayMode) => void;

  // Log lines
  lines: string[];
  appendLines: (lines: string[]) => void;
  setLines: (lines: string[]) => void;
  clearLines: () => void;

  // Pagination cursors
  prevPageToken: string;
  nextPageToken: string;
  setPaginationTokens: (prev: string, next: string) => void;

  // Filters
  searchText: string;
  setSearchText: (text: string) => void;
  startTime: number; // Unix seconds, 0 = unset
  endTime: number;
  setTimeRange: (start: number, end: number) => void;

  // Dark mode
  darkMode: boolean;
  toggleDarkMode: () => void;
}

const stored = localStorage.getItem('simple-logging.theme');

export const useLogStore = create<LogStore>((set) => ({
  selectedNamespace: null,
  selectedPod: null,
  setSelectedPod: (namespace, pod) =>
    set({
      selectedNamespace: namespace,
      selectedPod: pod,
      mode: 'loading',
      lines: [],
      prevPageToken: '',
      nextPageToken: '',
      searchText: '',
      startTime: 0,
      endTime: 0,
    }),

  mode: 'idle',
  setMode: (mode) => set({ mode }),

  lines: [],
  appendLines: (newLines) =>
    set((s) => ({ lines: [...s.lines, ...newLines] })),
  setLines: (lines) => set({ lines }),
  clearLines: () => set({ lines: [] }),

  prevPageToken: '',
  nextPageToken: '',
  setPaginationTokens: (prev, next) =>
    set({ prevPageToken: prev, nextPageToken: next }),

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
}));

/** Derived: lines filtered by current searchText */
export function useFilteredLines(): string[] {
  return useLogStore((s) => {
    if (!s.searchText) return s.lines;
    const lower = s.searchText.toLowerCase();
    return s.lines.filter((l) => l.toLowerCase().includes(lower));
  });
}
