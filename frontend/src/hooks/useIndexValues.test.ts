import { act, renderHook, waitFor } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { logClient } from '../grpc/client.js';
import { useIndexValues } from './useIndexValues.js';

vi.mock('../grpc/client.js', () => ({
  logClient: {
    listIndexValues: vi.fn(),
  },
}));

const listIndexValues = vi.mocked(logClient.listIndexValues);

describe('useIndexValues', () => {
  beforeEach(() => {
    listIndexValues.mockReset();
    listIndexValues
      .mockResolvedValueOnce({
        values: [{ value: 'newest', count: 1n, lastUpdatedUnixMs: 2n }],
        nextPageToken: '50',
        prevPageToken: '',
      })
      .mockResolvedValueOnce({
        values: [{ value: 'older', count: 1n, lastUpdatedUnixMs: 1n }],
        nextPageToken: '',
        prevPageToken: '0',
      })
      .mockResolvedValueOnce({
        values: [{ value: 'newest', count: 1n, lastUpdatedUnixMs: 2n }],
        nextPageToken: '50',
        prevPageToken: '',
      });
  });

  it('loads and navigates paginated index values', async () => {
    const { result } = renderHook(() => useIndexValues('companyUuid'));

    await waitFor(() => expect(result.current.values[0]?.value).toBe('newest'));
    expect(listIndexValues).toHaveBeenNthCalledWith(1, {
      key: 'companyUuid',
      pageSize: 50,
      pageToken: '',
    });
    expect(result.current.hasPrevPage).toBe(false);
    expect(result.current.hasNextPage).toBe(true);

    await act(async () => {
      await result.current.nextPage();
    });

    expect(result.current.values[0]?.value).toBe('older');
    expect(listIndexValues).toHaveBeenNthCalledWith(2, {
      key: 'companyUuid',
      pageSize: 50,
      pageToken: '50',
    });
    expect(result.current.hasPrevPage).toBe(true);
    expect(result.current.hasNextPage).toBe(false);

    await act(async () => {
      await result.current.prevPage();
    });

    expect(listIndexValues).toHaveBeenNthCalledWith(3, {
      key: 'companyUuid',
      pageSize: 50,
      pageToken: '0',
    });
    expect(result.current.values[0]?.value).toBe('newest');
    expect(result.current.hasPrevPage).toBe(false);
  });
});
