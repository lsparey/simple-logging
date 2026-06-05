import { useCallback, useEffect } from 'react';
import { logClient } from '../grpc/client.js';
import { useLogStore } from '../store/logStore.js';

export function useIndexLogHistory(
  key: string | null,
  value: string | null,
) {
  const setLines = useLogStore((s) => s.setLines);
  const setPaginationTokens = useLogStore((s) => s.setPaginationTokens);
  const setMode = useLogStore((s) => s.setMode);

  const load = useCallback(async () => {
    if (!key || !value) {
      setLines([]);
      setPaginationTokens('', '');
      setMode('idle');
      return;
    }
    setMode('loading');
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
