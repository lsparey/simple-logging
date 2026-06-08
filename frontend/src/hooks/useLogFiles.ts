import { useCallback, useEffect, useState } from 'react';
import { logClient } from '../grpc/client.js';

export interface LogFileSummary {
  namespace: string;
  name: string;
  sizeBytes: bigint;
  kind: string;
  modifiedAtUnixMs: bigint;
  subject: string;
}

interface LogFilesState {
  files: LogFileSummary[];
  totalSizeBytes: bigint;
  loading: boolean;
  error: string | null;
  refresh: () => void;
}

export function useLogFiles(): LogFilesState {
  const [files, setFiles] = useState<LogFileSummary[]>([]);
  const [totalSizeBytes, setTotalSizeBytes] = useState(0n);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [refreshKey, setRefreshKey] = useState(0);

  useEffect(() => {
    let cancelled = false;

    logClient.listLogFiles({})
      .then((response) => {
        if (cancelled) return;
        setFiles(response.files);
        setTotalSizeBytes(response.totalSizeBytes);
      })
      .catch((err: unknown) => {
        if (cancelled) return;
        setError(err instanceof Error ? err.message : 'Unable to load log files');
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });

    return () => {
      cancelled = true;
    };
  }, [refreshKey]);

  const refresh = useCallback(() => {
    setLoading(true);
    setError(null);
    setRefreshKey((key) => key + 1);
  }, []);

  return { files, totalSizeBytes, loading, error, refresh };
}
