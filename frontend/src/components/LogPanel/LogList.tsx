import { useEffect, useLayoutEffect, useCallback, useRef, useState } from "react";
import { List, useListRef, type RowComponentProps } from "react-window";
import Box from "@mui/material/Box";
import Chip from "@mui/material/Chip";
import CircularProgress from "@mui/material/CircularProgress";
import LogLine from "./LogLine.js";

const ROW_HEIGHT = 22;
const NEAR_TOP_THRESHOLD = 15;
const BOTTOM_SPACER_ROWS = 2;

/** Extract a short display timestamp from the first space-delimited token of a log line. */
function parseTimestamp(line: string | undefined): string | null {
  if (!line) return null;
  const ts = line.split(' ')[0];
  // Expect at least "YYYY-MM-DDTHH:MM" (16 chars) in RFC3339 format
  if (ts.length < 16 || !ts.includes('T')) return null;
  return ts.slice(0, 16).replace('T', ' ');
}

interface Props {
  lines: string[];
  darkMode: boolean;
  autoScroll: boolean;
  liveEnabled?: boolean;
  isFetchingMore?: boolean;
  hasOlderLogs?: boolean;
  lineCount?: number;
  selectionKey?: number;
  prependKey?: number;
  prependCount?: number;
  onScrollUp: () => void;
  onScrollBottom: () => void;
  onNearTop?: () => void;
}

export default function LogList({
  lines,
  darkMode,
  autoScroll,
  liveEnabled = false,
  isFetchingMore = false,
  hasOlderLogs = false,
  selectionKey = 0,
  prependKey = 0,
  prependCount = 0,
  onScrollUp,
  onScrollBottom,
  onNearTop,
}: Props) {
  const listRef = useListRef(null);
  // Ref attached to the last rendered row, used to scroll it into view.
  const lastRowRef = useRef<HTMLDivElement | null>(null);
  // Ref on the outer Box so we can adjust scrollTop after prepending lines.
  const containerRef = useRef<HTMLDivElement | null>(null);

  // Keep a synchronously-updated ref to the latest lines array so that
  // RowComponent and handleRowsRendered can read current values without
  // including `lines` in their useCallback deps.  Including `lines` in those
  // deps causes both callbacks to be recreated on every append, which triggers
  // react-window's internal onRowsRendered useEffect (it has the callback as a
  // dep) — ultimately causing "Maximum update depth exceeded" during rapid live
  // streaming.
  const linesRef = useRef(lines);
  // eslint-disable-next-line react-hooks/refs
  linesRef.current = lines; // intentionally written during render so RowComponent and handleRowsRendered always see the latest array

  // Tracks the currently visible row range for the scroll indicator.
  const [scrollInfo, setScrollInfo] = useState<{ ts: string | null; start: number; stop: number }>(
    { ts: null, start: 0, stop: 0 },
  );

  // On initial selection load (selectionKey changes), scroll to the bottom
  // once the first batch of lines arrives. While settling, suppress the
  // near-top trigger so we don't immediately fire loadOlder.
  const needsInitialScrollRef = useRef(false);
  const settlingRef = useRef(false);
  useEffect(() => {
    needsInitialScrollRef.current = true;
    settlingRef.current = true;
  }, [selectionKey]);

  // Called by the List's onResize once react-window has a real measured height.
  // That's the earliest point scrollToRow works correctly — same API the live
  // mode uses successfully.  Uses linesRef.current so this callback stays
  // stable across line appends and doesn't trigger react-window's onResize
  // layout effect on every update.
  const handleResize = useCallback(() => {
    if (!needsInitialScrollRef.current || linesRef.current.length === 0 || liveEnabled) return;
    needsInitialScrollRef.current = false;
    try {
      listRef.current?.scrollToRow({ index: linesRef.current.length - 1 + BOTTOM_SPACER_ROWS, align: "end" });
    } catch {
      // ignore RangeError during rapid re-renders
    }
    setTimeout(() => { settlingRef.current = false; }, 150);
  }, [liveEnabled, listRef]);

  // Live mode: auto-scroll to bottom when new lines arrive.
  useEffect(() => {
    if (liveEnabled && autoScroll && lines.length > 0) {
      try {
        listRef.current?.scrollToRow({ index: lines.length - 1 + BOTTOM_SPACER_ROWS, align: "end" });
      } catch {
        // ignore RangeError during rapid re-renders
      }
    }
  }, [liveEnabled, lines.length, autoScroll, listRef]);

  // After prepending N lines to the top, adjust the scroll offset so the
  // previously-visible rows remain in view (no visual jump). prependKey
  // always increments so this fires even when two fetches return equal counts.
  useLayoutEffect(() => {
    if (prependCount > 0) {
      const el = containerRef.current?.querySelector<HTMLElement>('[style*="overflow"]');
      if (el) el.scrollTop += prependCount * ROW_HEIGHT;
    }
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [prependKey]);

  const RowComponent = useCallback(
    ({ index, style }: RowComponentProps) => {
      const currentLines = linesRef.current;
      return index < currentLines.length
        ? <div style={style} ref={index === currentLines.length - 1 ? lastRowRef : undefined}>
            <LogLine line={currentLines[index]} darkMode={darkMode} />
          </div>
        : <div style={style} />;
    },
    [darkMode],
  );

  const handleRowsRendered = useCallback(
    (visibleRows: { startIndex: number; stopIndex: number }) => {
      const currentLines = linesRef.current;
      if (visibleRows.stopIndex >= currentLines.length - 1) onScrollBottom();
      else onScrollUp();

      if (
        onNearTop &&
        !settlingRef.current &&
        currentLines.length > 0 &&
        visibleRows.startIndex <= NEAR_TOP_THRESHOLD
      ) {
        onNearTop();
      }

      // Update scroll indicator (setScrollInfo is a stable useState setter — no dep needed).
      setScrollInfo({
        ts: parseTimestamp(currentLines[visibleRows.startIndex]),
        start: visibleRows.startIndex,
        stop: visibleRows.stopIndex,
      });
    },
    // linesRef.current gives access to the latest lines without making this
    // callback unstable — it's updated synchronously during every render so
    // reads here always see the current array.
    [onScrollUp, onScrollBottom, onNearTop],
  );

  const pageNum = Math.floor(scrollInfo.start / 200) + 1;
  const totalPages = Math.max(1, Math.ceil(lines.length / 200));
  const chipLabel = [
    scrollInfo.ts,
    `Page ${pageNum} / ${totalPages}`,
  ].filter(Boolean).join('  ·  ');

  const liveChipLabel = (
    <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.75 }}>
      <Box
        sx={{
          width: 7,
          height: 7,
          borderRadius: '50%',
          bgcolor: 'success.main',
          flexShrink: 0,
          '@keyframes livePulse': {
            '0%': { boxShadow: '0 0 0 0 rgba(102, 187, 106, 0.8)' },
            '70%': { boxShadow: '0 0 0 6px rgba(102, 187, 106, 0)' },
            '100%': { boxShadow: '0 0 0 0 rgba(102, 187, 106, 0)' },
          },
          animation: 'livePulse 1.6s ease-out infinite',
        }}
      />
      <Box component="span" sx={{ color: 'success.main', fontWeight: 700, letterSpacing: 0.6 }}>LIVE</Box>
    </Box>
  );

  const leftChipLabel = isFetchingMore ? (
    <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.75 }}>
      <CircularProgress size={10} sx={{ color: 'inherit' }} />
      <Box component="span">Loading older…</Box>
    </Box>
  ) : hasOlderLogs ? '↑ Scroll for older' : null;

  return (
    <Box ref={containerRef} sx={{ flex: 1, overflow: "hidden", fontFamily: "monospace", height: "100%", bgcolor: "background.default", position: "relative" }}>
      {lines.length === 0 ? (
        <Box
          sx={{
            height: "100%",
            display: "flex",
            alignItems: "center",
            justifyContent: "center",
            color: "text.disabled",
            fontSize: "0.875rem",
          }}
        >
          No log lines to display.
        </Box>
      ) : (
        <>
          <List
            listRef={listRef}
            rowCount={lines.length + BOTTOM_SPACER_ROWS}
            rowHeight={ROW_HEIGHT}
            rowComponent={RowComponent}
            rowProps={{}}
            onRowsRendered={handleRowsRendered}
            onResize={handleResize}
            style={{ height: "100%" }}
          />
          <Chip
            size="small"
            label={liveEnabled ? liveChipLabel : chipLabel}
            sx={{
              position: "absolute",
              bottom: 8,
              right: 16,
              pointerEvents: "none",
              opacity: 0.92,
              fontSize: "0.68rem",
              height: 22,
              zIndex: 1,
              bgcolor: "background.paper",
              border: 1,
              borderColor: liveEnabled ? 'success.main' : 'divider',
            }}
          />

          {/* Left status chip */}
          {!liveEnabled && leftChipLabel !== null && (
            <Chip
              size="small"
              label={leftChipLabel}
              sx={{
                position: 'absolute',
                bottom: 8,
                left: 8,
                pointerEvents: 'none',
                opacity: 0.92,
                fontSize: '0.68rem',
                height: 22,
                zIndex: 1,
                bgcolor: 'background.paper',
                border: 1,
                borderColor: 'divider',
              }}
            />
          )}

          {liveEnabled && !autoScroll && (
            <Chip
              size="small"
              label="↑ Scroll to bottom to resume"
              sx={{
                position: 'absolute',
                bottom: 8,
                left: 8,
                pointerEvents: 'none',
                opacity: 0.92,
                fontSize: '0.68rem',
                height: 22,
                zIndex: 1,
                bgcolor: 'background.paper',
                border: 1,
                borderColor: 'divider',
              }}
            />
          )}
        </>
      )}
    </Box>
  );
}
