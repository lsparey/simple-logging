import { useCallback, useEffect, useState } from 'react';
import type { LogIndexValueInfo } from '../gen/simplelog/v1/log_service_pb.js';
import { logClient } from '../grpc/client.js';

export function useIndexValues(key: string | null) {
  const [values, setValues] = useState<LogIndexValueInfo[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [pageToken, setPageToken] = useState('');
  const [nextPageToken, setNextPageToken] = useState('');
  const [prevPageToken, setPrevPageToken] = useState('');

  const load = useCallback(async (token = '') => {
    if (!key) {
      setValues([]);
      setError(null);
      setLoading(false);
      setPageToken('');
      setNextPageToken('');
      setPrevPageToken('');
      return;
    }
    setLoading(true);
    try {
      const resp = await logClient.listIndexValues({ key, pageSize: 50, pageToken: token });
      setValues(resp.values);
      setPageToken(token);
      setNextPageToken(resp.nextPageToken);
      setPrevPageToken(resp.prevPageToken);
      setError(null);
    } catch (err) {
      setValues([]);
      setNextPageToken('');
      setPrevPageToken('');
      setError(err instanceof Error ? err.message : 'Failed to load index values');
    } finally {
      setLoading(false);
    }
  }, [key]);

  useEffect(() => {
    void Promise.resolve().then(() => load(''));
  }, [load]);

  return {
    values,
    loading,
    error,
    hasNextPage: nextPageToken !== '',
    hasPrevPage: prevPageToken !== '',
    nextPage: () => load(nextPageToken),
    prevPage: () => load(prevPageToken),
    reload: () => load(pageToken),
  };
}
