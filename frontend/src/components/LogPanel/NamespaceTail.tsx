import { useCallback, useEffect, useMemo, useState } from 'react';
import Box from '@mui/material/Box';
import Button from '@mui/material/Button';
import Chip from '@mui/material/Chip';
import TextField from '@mui/material/TextField';
import Typography from '@mui/material/Typography';
import StopIcon from '@mui/icons-material/Stop';
import LogList from './LogList.js';
import { logClient } from '../../grpc/client.js';
import { useLogStore } from '../../store/logStore.js';

/** Lines kept on screen; older ones scroll off, since a namespace can be busy. */
export const NAMESPACE_TAIL_MAX_LINES = 5000;

interface Props {
  namespace: string;
  onStop: () => void;
}

/**
 * A live tail of every pod in a namespace (StreamWorkloadLogs with no kind or
 * name). It shows only lines written from when it starts, with no history
 * or paging, and keeps its own state apart from the workload log view.
 */
export default function NamespaceTail({ namespace, onStop }: Props) {
  const darkMode = useLogStore((s) => s.darkMode);
  const [lines, setLines] = useState<string[]>([]);
  const [filter, setFilter] = useState('');
  const [autoScroll, setAutoScroll] = useState(true);

  useEffect(() => {
    const controller = new AbortController();
    // Flush once per animation frame, so a burst of lines is one render.
    const buffer: string[] = [];
    let rafId: number | null = null;
    const flush = () => {
      rafId = null;
      const batch = buffer.splice(0);
      setLines((prev) => {
        const next = prev.concat(batch);
        return next.length > NAMESPACE_TAIL_MAX_LINES ? next.slice(-NAMESPACE_TAIL_MAX_LINES) : next;
      });
    };

    (async () => {
      try {
        for await (const msg of logClient.streamWorkloadLogs({ namespace }, { signal: controller.signal })) {
          buffer.push(msg.line);
          if (rafId === null) rafId = requestAnimationFrame(flush);
        }
      } catch {
        // AbortError on stop or unmount is expected.
      }
    })();

    return () => {
      controller.abort();
      if (rafId !== null) cancelAnimationFrame(rafId);
    };
  }, [namespace]);

  const shown = useMemo(() => {
    if (!filter) return lines;
    const lower = filter.toLowerCase();
    return lines.filter((l) => l.toLowerCase().includes(lower));
  }, [lines, filter]);

  const handleScrollUp = useCallback(() => setAutoScroll(false), []);
  const handleScrollBottom = useCallback(() => setAutoScroll(true), []);

  return (
    <Box sx={{ flex: 1, display: 'flex', flexDirection: 'column', overflow: 'hidden' }}>
      <Box sx={{ display: 'flex', flexWrap: 'wrap', alignItems: 'center', gap: 1.5, px: 2, py: 1, borderBottom: 1, borderColor: 'divider' }}>
        <Chip label={`${namespace} · all pods`} size="small" variant="outlined" sx={{ fontFamily: 'monospace' }} />
        <Chip label="Live" size="small" color="success" />
        <TextField
          size="small"
          placeholder="Filter…"
          value={filter}
          onChange={(e) => setFilter(e.target.value)}
          sx={{ flex: 1, minWidth: 160 }}
        />
        <Button size="small" startIcon={<StopIcon />} onClick={onStop}>
          Stop
        </Button>
      </Box>
      {lines.length === 0 ? (
        <Box sx={{ flex: 1, display: 'flex', alignItems: 'center', justifyContent: 'center', color: 'text.disabled' }}>
          <Typography>Waiting for new log lines from pods in {namespace}…</Typography>
        </Box>
      ) : (
        <LogList
          lines={shown}
          darkMode={darkMode}
          autoScroll={autoScroll}
          liveEnabled
          lineCount={shown.length}
          onScrollUp={handleScrollUp}
          onScrollBottom={handleScrollBottom}
        />
      )}
    </Box>
  );
}
