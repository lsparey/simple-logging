import { useState, useEffect } from 'react';
import { logClient } from '../grpc/client.js';
import type { DeploymentInfo } from '../gen/simplelog/v1/log_service_pb.js';

export function useDeploymentList(namespace: string | null) {
  const [deployments, setDeployments] = useState<DeploymentInfo[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (!namespace) {
      setDeployments([]);
      return;
    }
    let cancelled = false;

    async function load() {
      setLoading(true);
      try {
        const resp = await logClient.listDeployments({ namespace: namespace! });
        if (!cancelled) {
          setDeployments(resp.deployments);
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

  return { deployments, loading, error };
}
