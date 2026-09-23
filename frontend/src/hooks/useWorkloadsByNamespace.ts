import { useState, useEffect } from 'react';
import { logClient } from '../grpc/client.js';
import type { WorkloadInfo } from '../gen/simplelog/v1/log_service_pb.js';

/**
 * Fetches every namespace's workloads in parallel, optionally filtered down
 * to one kind, so callers can tell upfront which namespaces have anything to
 * show (rather than discovering it's empty only after the user expands it).
 * Omit kind to get every workload of every kind for each namespace.
 */
export function useWorkloadsByNamespace(namespaces: string[], kind?: string) {
  const [byNamespace, setByNamespace] = useState<Record<string, WorkloadInfo[]>>({});
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    let cancelled = false;

    async function load() {
      if (namespaces.length === 0) {
        // Bail out via the same reference when already empty: some callers
        // pass a fresh `[]` literal each render (e.g. a ternary fallback),
        // and setting a brand-new object here every time would otherwise
        // re-render this hook indefinitely, since object state never
        // satisfies React's reference-equality bailout the way primitives do.
        setByNamespace((prev) => (Object.keys(prev).length === 0 ? prev : {}));
        setLoading(false);
        return;
      }

      setLoading(true);
      try {
        const results = await Promise.all(
          namespaces.map(async (ns) => {
            const resp = await logClient.listWorkloads({ namespace: ns });
            const workloads = kind ? resp.workloads.filter((w) => w.kind === kind) : resp.workloads;
            return [ns, workloads] as const;
          }),
        );
        if (!cancelled) {
          setByNamespace(Object.fromEntries(results));
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
    const interval = namespaces.length > 0 ? setInterval(load, 30_000) : undefined;
    return () => {
      cancelled = true;
      if (interval) clearInterval(interval);
    };
  }, [namespaces, kind]);

  return { workloadsByNamespace: byNamespace, loading, error };
}
