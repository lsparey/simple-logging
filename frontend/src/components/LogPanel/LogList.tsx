import { useEffect, useCallback, useState } from "react";
import { List, useListRef, type RowComponentProps } from "react-window";
import Box from "@mui/material/Box";
import Chip from "@mui/material/Chip";
import CircularProgress from "@mui/material/CircularProgress";
import LogLine from "./LogLine.js";

const ROW_HEIGHT = 22;
const NEAR_BOTTOM_THRESHOLD = 15;

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
  hasMore?: boolean;
  lineCount?: number;
  onScrollUp: () => void;
  onScrollBottom: () => void;
  onNearBottom?: () => void;
}

export default function LogList({
  lines,
  darkMode,
  autoScroll,
  liveEnabled = false,
  isFetchingMore = false,
  hasMore = false,
  lineCount = 0,
  onScrollUp,
  onScrollBottom,
  onNearBottom,
}: Props) {
  const listRef = useListRef(null);

  // Tracks the currently visible row range for the scroll indicator.
  const [scrollInfo, setScrollInfo] = useState<{ ts: string | null; start: number; stop: number }>(
    { ts: null, start: 0, stop: 0 },
  );

  useEffect(() => {
    if (liveEnabled && autoScroll && lines.length > 0) {
      try {
        listRef.current?.scrollToRow({ index: lines.length - 1, align: "end" });
      } catch {
        // ignore RangeError during rapid re-renders
      }
    }
  }, [liveEnabled, lines.length, autoScroll, listRef]);

  const RowComponent = useCallback(
    ({ index, style }: RowComponentProps) => (
      <div style={style}>
        <LogLine line={lines[index]} darkMode={darkMode} />
      </div>
    ),
    [lines, darkMode],
  );

  const handleRowsRendered = useCallback(
    (visibleRows: { startIndex: number; stopIndex: number }) => {
      if (visibleRows.stopIndex >= lines.length - 1) onScrollBottom();
      else onScrollUp();

      if (
        onNearBottom &&
        lines.length > 0 &&
        visibleRows.stopIndex >= lines.length - NEAR_BOTTOM_THRESHOLD
      ) {
        onNearBottom();
      }

      // Update scroll indicator (setScrollInfo is a stable useState setter — no dep needed).
      setScrollInfo({
        ts: parseTimestamp(lines[visibleRows.startIndex]),
        start: visibleRows.startIndex,
        stop: visibleRows.stopIndex,
      });
    },
    // Use the full `lines` array (not just lines.length) so the latest entries are
    // captured for timestamp parsing, while keeping the same recreation frequency.
    [lines, onScrollUp, onScrollBottom, onNearBottom],
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
      <Box component="span">{lineCount} line{lineCount !== 1 ? 's' : ''}</Box>
      <CircularProgress size={10} sx={{ color: 'inherit' }} />
      <Box component="span">Loading more…</Box>
    </Box>
  ) : `${lineCount} line${lineCount !== 1 ? 's' : ''}  ·  ${hasMore ? '↓ Scroll for more' : 'End of log'}`;

  return (
    <Box sx={{ flex: 1, overflow: "hidden", fontFamily: "monospace", height: "100%", bgcolor: "background.default", position: "relative" }}>
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
            rowCount={lines.length}
            rowHeight={ROW_HEIGHT}
            rowComponent={RowComponent}
            rowProps={{}}
            onRowsRendered={handleRowsRendered}
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
          {!liveEnabled && (
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
