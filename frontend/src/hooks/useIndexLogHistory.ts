import { useCallback, useEffect, useRef } from 'react';
import { logClient } from '../grpc/client.js';
import { useLogStore } from '../store/logStore.js';

export function useIndexLogHistory(
  key: string | null,
  value: string | null,
) {
  const setLines = useLogStore((s) => s.setLines);
  const setPaginationTokens = useLogStore((s) => s.setPaginationTokens);
  const setMode = useLogStore((s) => s.setMode);
  const abortRef = useRef<AbortController | null>(null);

  const load = useCallback(async () => {
    abortRef.current?.abort();
    if (!key || !value) {
      setLines([]);
      setPaginationTokens('', '');
      setMode('idle');
      return;
    }
    const controller = new AbortController();
    abortRef.current = controller;
    setMode('loading');
    try {
      const resp = await logClient.getIndexLogs(
        {
          key,
          value,
          pageSize: 200,
          pageToken: '',
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
      setLines([]);
      setPaginationTokens('', '');
      setMode('history');
    }
  }, [key, value, setLines, setPaginationTokens, setMode]);

  useEffect(() => {
    load();
    return () => abortRef.current?.abort();
  }, [load]);

  return { reload: load };
}
