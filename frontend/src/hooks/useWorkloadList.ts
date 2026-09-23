import { useState, useEffect } from 'react';
import { logClient } from '../grpc/client.js';
import type { WorkloadInfo } from '../gen/simplelog/v1/log_service_pb.js';

export function useWorkloadList(namespace: string | null) {
  const [workloads, setWorkloads] = useState<WorkloadInfo[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (!namespace) {
      return;
    }
    let cancelled = false;

    async function load() {
      setLoading(true);
      try {
        const resp = await logClient.listWorkloads({ namespace: namespace! });
        if (!cancelled) {
          setWorkloads(resp.workloads);
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

  return { workloads: namespace ? workloads : [], loading: namespace ? loading : false, error };
}
