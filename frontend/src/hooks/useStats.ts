import { useCallback, useEffect, useState } from 'react';
import { logClient } from '../grpc/client.js';
import type { GetStatsResponse } from '../gen/simplelog/v1/log_service_pb.js';

interface StatsState {
  /** Null until loaded, or if the server couldn't report stats. */
  stats: GetStatsResponse | null;
  refresh: () => void;
}

/** How often the dashboard re-reads the collection counters on its own. */
export const STATS_POLL_INTERVAL_MS = 30_000;

/**
 * The server's self-metrics (GetStats): fetched on mount, every
 * STATS_POLL_INTERVAL_MS while mounted, and on refresh, so a dropped-lines
 * warning appears without reloading the page.
 */
export function useStats(): StatsState {
  const [stats, setStats] = useState<GetStatsResponse | null>(null);
  const [refreshKey, setRefreshKey] = useState(0);

  useEffect(() => {
    const interval = setInterval(() => setRefreshKey((key) => key + 1), STATS_POLL_INTERVAL_MS);
    return () => clearInterval(interval);
  }, []);

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
