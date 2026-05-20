import { useState, useEffect } from 'react';
import { logClient } from '../grpc/client.js';
import type { PodInfo } from '../gen/simplelog/v1/log_service_pb.js';

export function usePodList(namespace: string | null) {
  const [pods, setPods] = useState<PodInfo[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (!namespace) {
      setPods([]);
      return;
    }
    let cancelled = false;

    async function load() {
      setLoading(true);
      try {
        const resp = await logClient.listPods({ namespace: namespace! });
        if (!cancelled) {
          setPods(resp.pods);
          setLoading(false);
        }
      } catch (e) {
        if (!cancelled) {
          setError(String(e));
          setLoading(false);
        }
      }
    }

    load();
    const interval = setInterval(load, 30_000);
    return () => {
      cancelled = true;
      clearInterval(interval);
    };
  }, [namespace]);

  return { pods, loading, error };
}
