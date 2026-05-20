import { useEffect, useCallback } from "react";
import { List, useListRef, type RowComponentProps } from "react-window";
import Box from "@mui/material/Box";
import LogLine from "./LogLine.js";
import { useLogStore } from "../../store/logStore.js";

const ROW_HEIGHT = 22;

interface Props {
  lines: string[];
  darkMode: boolean;
  autoScroll: boolean;
  onScrollUp: () => void;
  onScrollBottom: () => void;
}

export default function LogList({
  lines,
  darkMode,
  autoScroll,
  onScrollUp,
  onScrollBottom,
}: Props) {
  const listRef = useListRef(null);
  const mode = useLogStore((s) => s.mode);

  useEffect(() => {
    if (autoScroll && mode === "live" && lines.length > 0) {
      try {
        listRef.current?.scrollToRow({ index: lines.length - 1, align: "end" });
      } catch {
        // ignore RangeError during rapid re-renders
      }
    }
  }, [lines.length, autoScroll, mode, listRef]);

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
    },
    [lines.length, onScrollUp, onScrollBottom],
  );

  return (
    <Box sx={{ flex: 1, overflow: "hidden", fontFamily: "monospace", height: "100%" }}>
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
        <List
          listRef={listRef}
          rowCount={lines.length}
          rowHeight={ROW_HEIGHT}
          rowComponent={RowComponent}
          rowProps={{}}
          onRowsRendered={handleRowsRendered}
          style={{ height: "100%" }}
        />
      )}
    </Box>
  );
}
