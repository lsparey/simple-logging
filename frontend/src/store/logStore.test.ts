import { renderHook, act } from '@testing-library/react';
import { afterEach, beforeEach, describe, it, expect } from 'vitest';
import { useLogStore, useFilteredLines } from './logStore.js';

// Reset the store to a clean state between tests so they don't bleed into each other.
beforeEach(() => {
  useLogStore.setState({
    lines: [],
    searchText: '',
    prevPageToken: '',
    nextPageToken: '',
    isFetchingMore: false,
    mode: 'idle',
    selectedNamespace: null,
    selectedPod: null,
    selectedDeployment: null,
    startTime: 0,
    endTime: 0,
  });
});

afterEach(() => {
  useLogStore.setState({
    lines: [],
    searchText: '',
  });
});

// ---------------------------------------------------------------------------
// Line mutations
// ---------------------------------------------------------------------------

describe('logStore — line mutations', () => {
  it('setLines replaces all lines', () => {
    act(() => useLogStore.getState().setLines(['a', 'b', 'c']));
    expect(useLogStore.getState().lines).toEqual(['a', 'b', 'c']);
  });

  it('appendLines adds lines to the end', () => {
    act(() => {
      useLogStore.getState().setLines(['a', 'b']);
      useLogStore.getState().appendLines(['c', 'd']);
    });
    expect(useLogStore.getState().lines).toEqual(['a', 'b', 'c', 'd']);
  });

  it('prependLines adds lines to the front', () => {
    act(() => {
      useLogStore.getState().setLines(['c', 'd']);
      useLogStore.getState().prependLines(['a', 'b']);
    });
    expect(useLogStore.getState().lines).toEqual(['a', 'b', 'c', 'd']);
  });

  it('clearLines empties the array', () => {
    act(() => {
      useLogStore.getState().setLines(['a', 'b']);
      useLogStore.getState().clearLines();
    });
    expect(useLogStore.getState().lines).toHaveLength(0);
  });
});

// ---------------------------------------------------------------------------
// Selection actions clear state
// ---------------------------------------------------------------------------

describe('logStore — selection side effects', () => {
  it('setSelectedPod resets lines, tokens, searchText, and increments selectionKey', () => {
    act(() => {
      useLogStore.getState().setLines(['old']);
      useLogStore.getState().setSearchText('filter');
      useLogStore.setState({ prevPageToken: 'tok', nextPageToken: 'tok2' });
    });
    const prevKey = useLogStore.getState().selectionKey;

    act(() => useLogStore.getState().setSelectedPod('default', 'my-pod'));

    const s = useLogStore.getState();
    expect(s.lines).toHaveLength(0);
    expect(s.searchText).toBe('');
    expect(s.prevPageToken).toBe('');
    expect(s.nextPageToken).toBe('');
    expect(s.selectionKey).toBe(prevKey + 1);
    expect(s.selectedPod).toBe('my-pod');
    expect(s.selectedDeployment).toBeNull();
  });

  it('setSelectedDeployment resets state and clears pod selection', () => {
    act(() => {
      useLogStore.getState().setSelectedPod('default', 'old-pod');
    });

    act(() => useLogStore.getState().setSelectedDeployment('default', 'my-deploy'));

    const s = useLogStore.getState();
    expect(s.selectedDeployment).toBe('my-deploy');
    expect(s.selectedPod).toBeNull();
  });
});

// ---------------------------------------------------------------------------
// Pagination tokens
// ---------------------------------------------------------------------------

describe('logStore — pagination', () => {
  it('setPaginationTokens updates both tokens', () => {
    act(() => useLogStore.getState().setPaginationTokens('prev-tok', 'next-tok'));
    expect(useLogStore.getState().prevPageToken).toBe('prev-tok');
    expect(useLogStore.getState().nextPageToken).toBe('next-tok');
  });

  it('setTimeRange clears lines and tokens', () => {
    act(() => {
      useLogStore.getState().setLines(['a', 'b']);
      useLogStore.setState({ prevPageToken: 'p', nextPageToken: 'n' });
      useLogStore.getState().setTimeRange(1000, 2000);
    });
    const s = useLogStore.getState();
    expect(s.lines).toHaveLength(0);
    expect(s.prevPageToken).toBe('');
    expect(s.nextPageToken).toBe('');
    expect(s.startTime).toBe(1000);
    expect(s.endTime).toBe(2000);
  });
});

// ---------------------------------------------------------------------------
// useFilteredLines
// ---------------------------------------------------------------------------

describe('useFilteredLines', () => {
  it('returns all lines when searchText is empty', () => {
    act(() => useLogStore.getState().setLines(['INFO foo', 'WARN bar', 'ERROR baz']));

    const { result } = renderHook(() => useFilteredLines());
    expect(result.current).toEqual(['INFO foo', 'WARN bar', 'ERROR baz']);
  });

  it('returns only matching lines (case-insensitive)', () => {
    act(() => useLogStore.getState().setLines(['INFO foo', 'WARN bar', 'ERROR baz']));
    act(() => useLogStore.getState().setSearchText('warn'));

    const { result } = renderHook(() => useFilteredLines());
    expect(result.current).toEqual(['WARN bar']);
  });

  it('returns empty array when nothing matches', () => {
    act(() => useLogStore.getState().setLines(['INFO foo', 'WARN bar']));
    act(() => useLogStore.getState().setSearchText('xyz'));

    const { result } = renderHook(() => useFilteredLines());
    expect(result.current).toHaveLength(0);
  });

  it('updates reactively when lines change', () => {
    act(() => useLogStore.getState().setSearchText('foo'));
    const { result } = renderHook(() => useFilteredLines());
    expect(result.current).toHaveLength(0);

    act(() => useLogStore.getState().setLines(['INFO foo bar', 'INFO baz']));
    expect(result.current).toEqual(['INFO foo bar']);
  });

  it('updates reactively when searchText changes', () => {
    act(() => useLogStore.getState().setLines(['INFO foo', 'WARN bar']));
    const { result } = renderHook(() => useFilteredLines());
    expect(result.current).toHaveLength(2);

    act(() => useLogStore.getState().setSearchText('foo'));
    expect(result.current).toEqual(['INFO foo']);
  });
});
