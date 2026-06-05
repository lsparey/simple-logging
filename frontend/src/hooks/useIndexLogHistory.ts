import { useCallback, useEffect } from 'react';
import { logClient } from '../grpc/client.js';
import { useLogStore } from '../store/logStore.js';

export function useIndexLogHistory(
  key: string | null,
  value: string | null,
) {
  const { setLines, setPaginationTokens, setMode } = useLogStore.getState();

  const load = useCallback(async () => {
    if (!key || !value) return;
    try {
      const resp = await logClient.getIndexLogs({
        key,
        value,
        pageSize: 200,
        pageToken: '',
        loadLastPage: true,
      });
      setLines(resp.lines);
      setPaginationTokens(resp.prevPageToken, resp.nextPageToken);
      setMode('history');
    } catch {
      setLines([]);
      setPaginationTokens('', '');
      setMode('history');
    }
  }, [key, value, setLines, setPaginationTokens, setMode]);

  useEffect(() => {
    load();
  }, [load]);

  return { reload: load };
}
