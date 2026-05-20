import { useState, useEffect } from 'react';
import { logClient } from '../grpc/client.js';

export function useNamespaces() {
  const [namespaces, setNamespaces] = useState<string[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    let cancelled = false;

    async function load() {
      try {
        const resp = await logClient.listNamespaces({});
        if (!cancelled) {
          setNamespaces(resp.namespaces);
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
  }, []);

  return { namespaces, loading, error };
}
