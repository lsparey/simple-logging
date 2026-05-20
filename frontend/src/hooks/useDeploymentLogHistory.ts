import { useState, useCallback, useEffect } from 'react';
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
  const [loading, setLoading] = useState(false);
  const { setLines, setPaginationTokens, setMode } = useLogStore.getState();

  const load = useCallback(async () => {
    if (!namespace || !deployment) return;
    setLoading(true);
    setMode('loading');
    try {
      const resp = await logClient.getDeploymentLogs({
        namespace,
        deployment,
        startTime: BigInt(filters.startTime),
        endTime: BigInt(filters.endTime),
        pageSize: 200,
        pageToken: filters.pageToken,
      });
      setLines(resp.lines);
      setPaginationTokens(filters.pageToken, resp.nextPageToken);
      setMode('history');
    } catch {
      setMode('history');
    } finally {
      setLoading(false);
    }
  }, [namespace, deployment, filters.startTime, filters.endTime, filters.pageToken, setLines, setPaginationTokens, setMode]);

  useEffect(() => {
    load();
  }, [load]);

  return { loading, reload: load };
}
