import { useState, useEffect } from 'react';
import { logClient } from '../grpc/client.js';
import type { PodInfo } from '../gen/simplelog/v1/log_service_pb.js';

/**
 * Fetches every namespace's individual pods in parallel via ListPods.
 *
 * This is deliberately separate from useWorkloadsByNamespace: ListWorkloads
 * groups owned pods under their controller (kind "Deployment", "DaemonSet",
 * etc.) and only reports kind "Pod" for genuinely unowned bare pods, which
 * are rare in a real cluster. The sidebar's Pods section instead means
 * "browse every pod individually" regardless of ownership, which needs the
 * flat per-pod listing ListPods provides.
 */
export function usePodsByNamespace(namespaces: string[]) {
  const [byNamespace, setByNamespace] = useState<Record<string, PodInfo[]>>({});
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
            const resp = await logClient.listPods({ namespace: ns });
            return [ns, resp.pods] as const;
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
  }, [namespaces]);

  return { podsByNamespace: byNamespace, loading, error };
}
