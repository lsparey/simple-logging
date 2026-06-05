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

    // Buffer incoming lines and flush once per animation frame so that a burst
    // of many log messages from the server results in a single React render
    // instead of one render per message (which would exceed React's nested
    // update limit with large bursts).
    const buffer: string[] = [];
    let rafId: number | null = null;

    (async () => {
      try {
        const stream = logClient.streamDeploymentLogs(
          { namespace, deployment },
          { signal: controller.signal },
        );
        for await (const msg of stream) {
          buffer.push(msg.line);
          if (rafId === null) {
            rafId = requestAnimationFrame(() => {
              appendLines(buffer.splice(0));
              rafId = null;
            });
          }
        }
      } catch {
        // AbortError is expected on cleanup; ignore silently.
      }
      // Flush any lines buffered when the stream ends cleanly.
      if (rafId !== null) {
        cancelAnimationFrame(rafId);
        rafId = null;
      }
      if (buffer.length > 0) appendLines(buffer.splice(0));
    })();

    return () => {
      controller.abort();
      abortRef.current = null;
      if (rafId !== null) {
        cancelAnimationFrame(rafId);
        rafId = null;
      }
    };
  }, [enabled, namespace, deployment, appendLines, setMode]);
}
