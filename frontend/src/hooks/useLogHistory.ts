import { useEffect, useCallback } from 'react';
import { logClient } from '../grpc/client.js';
import { useLogStore } from '../store/logStore.js';

interface Filters {
  startTime: number;
  endTime: number;
  pageToken: string;
}

export function useLogHistory(
  namespace: string | null,
  pod: string | null,
  filters: Filters,
) {
  const { setLines, setPaginationTokens, setMode } = useLogStore.getState();

  const load = useCallback(async () => {
    if (!namespace || !pod) return;
    try {
      const resp = await logClient.getLogs({
        namespace,
        pod,
        startTime: BigInt(filters.startTime),
        endTime: BigInt(filters.endTime),
        pageSize: 200,
        pageToken: filters.pageToken,
      });
      setLines(resp.lines);
      // prev token = the token we used to arrive at this page
      setPaginationTokens(filters.pageToken, resp.nextPageToken);
      setMode('history');
    } catch {
      setMode('history');
    }
  }, [namespace, pod, filters.startTime, filters.endTime, filters.pageToken, setLines, setPaginationTokens, setMode]);

  useEffect(() => {
    load();
  }, [load]);

  return { reload: load };
}
