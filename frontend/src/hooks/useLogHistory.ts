import { useEffect, useCallback, useRef } from 'react';
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
  const abortRef = useRef<AbortController | null>(null);

  const load = useCallback(async () => {
    if (!namespace || !pod) return;
    abortRef.current?.abort();
    const controller = new AbortController();
    abortRef.current = controller;
    try {
      const resp = await logClient.getLogs(
        {
          namespace,
          pod,
          startTime: BigInt(filters.startTime),
          endTime: BigInt(filters.endTime),
          pageSize: 200,
          pageToken: filters.pageToken,
          loadLastPage: true,
        },
        { signal: controller.signal },
      );
      if (controller.signal.aborted) return;
      setLines(resp.lines);
      // prev token = the token we used to arrive at this page
      setPaginationTokens(resp.prevPageToken, resp.nextPageToken);
      setMode('history');
    } catch {
      if (controller.signal.aborted) return;
      setMode('history');
    }
  }, [namespace, pod, filters.startTime, filters.endTime, filters.pageToken, setLines, setPaginationTokens, setMode]);

  useEffect(() => {
    load();
    return () => abortRef.current?.abort();
  }, [load]);

  return { reload: load };
}
