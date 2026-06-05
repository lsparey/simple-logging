import { useCallback, useEffect, useState } from 'react';
import type { LogIndexValueInfo } from '../gen/simplelog/v1/log_service_pb.js';
import { logClient } from '../grpc/client.js';

export function useIndexValues(key: string | null) {
  const [values, setValues] = useState<LogIndexValueInfo[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const load = useCallback(async () => {
    if (!key) {
      setValues([]);
      setError(null);
      setLoading(false);
      return;
    }
    setLoading(true);
    try {
      const resp = await logClient.listIndexValues({ key });
      setValues(resp.values);
      setError(null);
    } catch (err) {
      setValues([]);
      setError(err instanceof Error ? err.message : 'Failed to load index values');
    } finally {
      setLoading(false);
    }
  }, [key]);

  useEffect(() => {
    void Promise.resolve().then(load);
  }, [load]);

  return { values, loading, error, reload: load };
}
