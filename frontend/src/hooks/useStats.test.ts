import { renderHook, act } from '@testing-library/react';
import { describe, expect, it, vi, afterEach } from 'vitest';
import { useStats, STATS_POLL_INTERVAL_MS } from './useStats.js';
import { logClient } from '../grpc/client.js';

vi.mock('../grpc/client.js', () => ({
  logClient: { getStats: vi.fn() },
}));
const getStats = vi.mocked(logClient.getStats);

afterEach(() => {
  vi.useRealTimers();
});

describe('useStats', () => {
  it('fetches on mount and again every poll interval', async () => {
    vi.useFakeTimers();
    getStats.mockResolvedValue({ linesDroppedTotal: 0n } as Awaited<ReturnType<typeof logClient.getStats>>);
    const { unmount } = renderHook(() => useStats());
    await act(async () => {});
    expect(getStats).toHaveBeenCalledTimes(1);

    await act(async () => {
      vi.advanceTimersByTime(STATS_POLL_INTERVAL_MS);
    });
    expect(getStats).toHaveBeenCalledTimes(2);

    unmount();
    await act(async () => {
      vi.advanceTimersByTime(STATS_POLL_INTERVAL_MS * 2);
    });
    expect(getStats).toHaveBeenCalledTimes(2);
  });
});
