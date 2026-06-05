import { useCallback, useEffect, useState } from 'react';
import { logClient } from '../grpc/client.js';
import type { LogIndexInfo } from '../gen/simplelog/v1/log_service_pb.js';

export function useIndexList() {
  const [indexes, setIndexes] = useState<LogIndexInfo[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const reload = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const resp = await logClient.listIndexes({});
      setIndexes(resp.indexes);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load indexes');
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void Promise.resolve().then(reload);
  }, [reload]);

  return { indexes, loading, error, reload };
}
