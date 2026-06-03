import { useRef, useEffect, useMemo, useState } from 'react';
import Box from '@mui/material/Box';
import { useTheme } from '@mui/material/styles';
import { useLogStore } from '../../store/logStore.js';

// Matches the RFC3339 timestamp prefix written by the collector:
// "2024-01-15T10:30:00Z [namespace/pod/container] message"
const TIMESTAMP_RE = /^(\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d+)?(?:Z|[+-]\d{2}:\d{2})?)/;

function parseTimestamp(line: string): number | null {
  const m = TIMESTAMP_RE.exec(line);
  if (!m) return null;
  const t = Date.parse(m[1]);
  return isNaN(t) ? null : t;
}

export default function LogHistogram() {
  const canvasRef = useRef<HTMLCanvasElement>(null);
  const containerRef = useRef<HTMLDivElement>(null);
  const lines = useLogStore((s) => s.lines);
  const theme = useTheme();
  const [dimensions, setDimensions] = useState({ width: 0, height: 0 });

  // Track container size so we can size the canvas drawing buffer to match.
  useEffect(() => {
    const container = containerRef.current;
    if (!container) return;
    const observer = new ResizeObserver((entries) => {
      const rect = entries[0].contentRect;
      setDimensions({
        width: Math.floor(rect.width),
        height: Math.floor(rect.height),
      });
    });
    observer.observe(container);
    return () => observer.disconnect();
  }, []);

  // Extract millisecond timestamps from every log line.
  const timestamps = useMemo(() => {
    const result: number[] = [];
    for (const line of lines) {
      const t = parseTimestamp(line);
      if (t !== null) result.push(t);
    }
    return result;
  }, [lines]);

  // Re-draw whenever data, size, or theme changes.
  useEffect(() => {
    const canvas = canvasRef.current;
    if (!canvas) return;
    const { width, height } = dimensions;
    if (width === 0 || height === 0) return;

    // Setting canvas.width also clears the canvas and resets context state.
    canvas.width = width;
    canvas.height = height;

    const ctx = canvas.getContext('2d');
    if (!ctx) return;

    if (timestamps.length < 2) return;

    let minT = Infinity;
    let maxT = -Infinity;
    for (const t of timestamps) {
      if (t < minT) minT = t;
      if (t > maxT) maxT = t;
    }
    // Use current time as the upper bound so that even when all messages share
    // the same timestamp (range === 0) we still render visible bars.
    const effectiveMax = Math.max(maxT, Date.now());
    const range = effectiveMax - minT;
    if (range === 0) return;

    // Constants for minimum bar/padding sizes (in canvas pixels).
    const BAR_MIN_W = 2;
    const BAR_MIN_H = 2;
    const PADDING = 2;

    // Drawable region after horizontal padding.
    const drawWidth = width - PADDING * 2;
    const drawHeight = height - PADDING * 2; // top + bottom padding
    if (drawWidth <= 0 || drawHeight <= 0) return;

    // Maximum buckets so each bar is at least BAR_MIN_W pixels wide.
    const maxBuckets = Math.floor(drawWidth / BAR_MIN_W);
    const bucketCount = Math.max(1, Math.min(maxBuckets, drawWidth));
    const buckets = new Array<number>(bucketCount).fill(0);
    for (const t of timestamps) {
      const idx = Math.min(
        bucketCount - 1,
        Math.floor(((t - minT) / range) * bucketCount),
      );
      buckets[idx]++;
    }

    let maxCount = 0;
    for (const c of buckets) {
      if (c > maxCount) maxCount = c;
    }
    if (maxCount === 0) return;

    // Bar width: divide drawable area evenly, at least BAR_MIN_W.
    const barW = Math.max(BAR_MIN_W, drawWidth / bucketCount);

    ctx.fillStyle = theme.palette.primary.main;
    for (let i = 0; i < bucketCount; i++) {
      if (buckets[i] === 0) continue;
      // At least BAR_MIN_H px for any occupied bucket; busiest bucket fills drawHeight.
      const barH = Math.max(BAR_MIN_H, Math.round((buckets[i] / maxCount) * drawHeight));
      const x = PADDING + i * barW;
      const y = height - PADDING - barH;
      ctx.fillRect(x, y, barW, barH);
    }
  }, [timestamps, dimensions, theme]);

  return (
    <Box
      ref={containerRef}
      sx={{
        flex: 1,
        minWidth: 160,
        height: 40,
        alignSelf: 'center',
        borderRadius: 1,
        overflow: 'hidden',
        bgcolor: 'action.hover',
      }}
    >
      <canvas
        ref={canvasRef}
        style={{ width: '100%', height: '100%', display: 'block' }}
      />
    </Box>
  );
}
