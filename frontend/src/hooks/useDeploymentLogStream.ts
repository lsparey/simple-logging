import { useEffect, useRef } from 'react';
import { logClient } from '../grpc/client.js';
import { useLogStore } from '../store/logStore.js';

export function useDeploymentLogStream(
  namespace: string | null,
  deployment: string | null,
  enabled: boolean,
) {
  const appendLines = useLogStore((s) => s.appendLines);
  const setMode = useLogStore((s) => s.setMode);
  const abortRef = useRef<AbortController | null>(null);

  useEffect(() => {
    if (!enabled || !namespace || !deployment) {
      abortRef.current?.abort();
      abortRef.current = null;
      return;
    }

    const controller = new AbortController();
    abortRef.current = controller;
    setMode('live');

    (async () => {
      try {
        const stream = logClient.streamDeploymentLogs(
          { namespace, deployment },
          { signal: controller.signal },
        );
        for await (const msg of stream) {
          appendLines([msg.line]);
        }
      } catch {
        // AbortError is expected on cleanup; ignore silently.
      }
    })();

    return () => {
      controller.abort();
      abortRef.current = null;
    };
  }, [enabled, namespace, deployment, appendLines, setMode]);
}
