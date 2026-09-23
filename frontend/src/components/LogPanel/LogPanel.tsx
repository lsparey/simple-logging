import { useState, useCallback } from 'react';
import { useNavigate } from 'react-router-dom';
import Box from '@mui/material/Box';
import CircularProgress from '@mui/material/CircularProgress';
import Typography from '@mui/material/Typography';
import LogToolbar from './LogToolbar.js';
import LogList from './LogList.js';
import SearchPanel, { type JumpTarget } from './SearchPanel.js';
import { useWorkloadLogHistory } from '../../hooks/useWorkloadLogHistory.js';
import { useWorkloadLogStream } from '../../hooks/useWorkloadLogStream.js';
import { useLogStore, useFilteredLines, makeFormatKey } from '../../store/logStore.js';
import { logClient } from '../../grpc/client.js';

export type SearchMode = 'page' | 'server';

// How far around a search hit's timestamp to load when jumping to context,
// so the surrounding lines are visible without pulling in unrelated history.
const JUMP_CONTEXT_WINDOW_MS = 5 * 60 * 1000;

export default function LogPanel() {
  const {
    selectedNamespace: namespace,
    selectedWorkloadKind: kind,
    selectedWorkloadName: name,
    selectionKey,
    mode,
    prevPageToken,
    startTime,
    endTime,
    darkMode,
    isFetchingMore,
    jsonFormats,
  } = useLogStore();

  const navigate = useNavigate();
  const [liveEnabled, setLiveEnabled] = useState(false);
  const [autoScroll, setAutoScroll] = useState(true);
  const [searchMode, setSearchMode] = useState<SearchMode>('page');
  // prependKey increments with every loadOlder call; prependCount carries the
  // number of lines added so LogList can adjust scrollTop even when two
  // consecutive fetches return the same count.
  const [prependKey, setPrependKey] = useState(0);
  const [prependCount, setPrependCount] = useState(0);

  // Always start from the beginning for the initial/filter-driven load.
  const filters = { startTime, endTime, pageToken: '' };

  useWorkloadLogHistory(
    !liveEnabled ? namespace : null,
    !liveEnabled ? kind : null,
    !liveEnabled ? name : null,
    filters,
  );
  useWorkloadLogStream(namespace, kind, name, liveEnabled);

  const filteredLines = useFilteredLines();
  const jsonFormat = namespace && kind && name ? (jsonFormats[makeFormatKey(namespace, kind, name)] ?? null) : null;

  const handleLiveToggle = useCallback((on: boolean) => {
    setLiveEnabled(on);
    // When entering live mode, force auto-scroll so the list jumps to the
    // bottom immediately (the scroll effect in LogList fires when mode -> 'live').
    if (on) setAutoScroll(true);
  }, []);

  // Loads the previous (older) page and prepends it to the existing lines.
  // Reads prevPageToken / isFetchingMore from store at call-time so this
  // callback stays stable, preventing spurious re-triggers.
  const loadOlder = useCallback(async () => {
    const {
      prevPageToken: token,
      isFetchingMore: fetching,
      startTime: st,
      endTime: et,
      setIsFetchingMore,
    } = useLogStore.getState();
    if (!token || fetching) return;
    if (!namespace || !kind || !name) return;
    setIsFetchingMore(true);
    try {
      const resp = await logClient.getWorkloadLogs({
        namespace,
        kind,
        name,
        startTime: BigInt(st),
        endTime: BigInt(et),
        pageSize: 200,
        pageToken: token,
      });
      setPrependKey((k) => k + 1);
      setPrependCount(resp.lines.length);
      useLogStore.getState().prependLines(resp.lines);
      useLogStore.getState().setPaginationTokens(resp.prevPageToken, useLogStore.getState().nextPageToken);
    } catch {
      // ignore fetch errors for load-older
    } finally {
      useLogStore.getState().setIsFetchingMore(false);
    }
  }, [namespace, kind, name]);

  // Stable scroll callbacks so LogList's handleRowsRendered doesn't recreate
  // (and re-trigger react-window's onRowsRendered effect) on every render.
  const handleScrollUp = useCallback(() => setAutoScroll(false), []);
  const handleScrollBottom = useCallback(() => setAutoScroll(true), []);

  // Selects the search hit's own pod directly (kind "Pod" resolves any pod
  // by name regardless of its real owner), filtered to the hit's container
  // and, if the hit has a timestamp, with the time range narrowed to a
  // window around it so the surrounding context loads instead of just the
  // latest page.
  const handleJumpToContext = useCallback(async (target: JumpTarget) => {
    let jsonLogging: boolean | undefined;
    try {
      const resp = await logClient.listPods({ namespace: target.namespace });
      jsonLogging = resp.pods.find((p) => p.name === target.pod)?.jsonLogging;
    } catch {
      // best-effort: default jsonLogging to false on failure
    }

    const store = useLogStore.getState();
    store.setSelectedWorkload(target.namespace, 'Pod', target.pod, jsonLogging);
    store.setSelectedContainerFilter(target.container);
    if (target.tsMs !== null) {
      store.setTimeRange(
        Math.floor((target.tsMs - JUMP_CONTEXT_WINDOW_MS) / 1000),
        Math.floor((target.tsMs + JUMP_CONTEXT_WINDOW_MS) / 1000),
      );
    }
    setSearchMode('page');
    navigate(`/ns/${encodeURIComponent(target.namespace)}/Pod/${encodeURIComponent(target.pod)}`);
  }, [navigate]);

  if (!namespace || !kind || !name) {
    return (
      <Box
        sx={{
          flex: 1,
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'center',
          color: 'text.disabled',
        }}
      >
        <Typography>Select a workload from the sidebar to view logs.</Typography>
      </Box>
    );
  }

  return (
    <Box sx={{ flex: 1, display: 'flex', flexDirection: 'column', overflow: 'hidden' }}>
      <LogToolbar
        namespace={namespace}
        kind={kind}
        name={name}
        liveEnabled={liveEnabled}
        onLiveToggle={handleLiveToggle}
        searchMode={searchMode}
        onSearchModeChange={setSearchMode}
      />

      {searchMode === 'server' ? (
        <SearchPanel namespace={namespace} kind={kind} name={name} onJumpToContext={handleJumpToContext} />
      ) : mode === 'loading' ? (
        <Box sx={{ display: 'flex', justifyContent: 'center', mt: 4 }}>
          <CircularProgress size={32} />
        </Box>
      ) : (
        <LogList
          lines={filteredLines}
          darkMode={darkMode}
          jsonFormat={jsonFormat}
          autoScroll={autoScroll}
          liveEnabled={liveEnabled}
          isFetchingMore={isFetchingMore}
          hasOlderLogs={!!prevPageToken}
          lineCount={filteredLines.length}
          selectionKey={selectionKey}
          prependKey={prependKey}
          prependCount={prependCount}
          onScrollUp={handleScrollUp}
          onScrollBottom={handleScrollBottom}
          onNearTop={!liveEnabled ? loadOlder : undefined}
        />
      )}
    </Box>
  );
}
