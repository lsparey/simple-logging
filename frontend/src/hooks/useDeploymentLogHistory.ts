import { useCallback, useEffect, useRef } from 'react';
import { logClient } from '../grpc/client.js';
import { useLogStore } from '../store/logStore.js';

interface Filters {
  startTime: number;
  endTime: number;
  pageToken: string;
}

export function useDeploymentLogHistory(
  namespace: string | null,
  deployment: string | null,
  filters: Filters,
) {
  const { setLines, setPaginationTokens, setMode } = useLogStore.getState();
  const abortRef = useRef<AbortController | null>(null);

  const load = useCallback(async () => {
    if (!namespace || !deployment) return;
    abortRef.current?.abort();
    const controller = new AbortController();
    abortRef.current = controller;
    try {
      const resp = await logClient.getDeploymentLogs(
        {
          namespace,
          deployment,
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
      setPaginationTokens(resp.prevPageToken, resp.nextPageToken);
      setMode('history');
    } catch {
      if (controller.signal.aborted) return;
      setMode('history');
    }
  }, [namespace, deployment, filters.startTime, filters.endTime, filters.pageToken, setLines, setPaginationTokens, setMode]);

  useEffect(() => {
    load();
    return () => abortRef.current?.abort();
  }, [load]);

  return { reload: load };
}
