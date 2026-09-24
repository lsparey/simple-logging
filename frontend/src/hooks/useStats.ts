import { useCallback, useEffect, useState } from 'react';
import { logClient } from '../grpc/client.js';
import type { GetStatsResponse } from '../gen/simplelog/v1/log_service_pb.js';

interface StatsState {
  /** Null until loaded, or if the server couldn't report stats. */
  stats: GetStatsResponse | null;
  refresh: () => void;
}

/** The server's self-metrics (GetStats), fetched once and on refresh. */
export function useStats(): StatsState {
  const [stats, setStats] = useState<GetStatsResponse | null>(null);
  const [refreshKey, setRefreshKey] = useState(0);

  useEffect(() => {
    let cancelled = false;
    logClient.getStats({})
      .then((response) => {
        if (!cancelled) setStats(response);
      })
      .catch(() => {
        // Stats are supplementary to the storage figures, so a failure just
        // leaves the collection section hidden rather than adding an error.
        if (!cancelled) setStats(null);
      });
    return () => {
      cancelled = true;
    };
  }, [refreshKey]);

  const refresh = useCallback(() => setRefreshKey((key) => key + 1), []);

  return { stats, refresh };
}
