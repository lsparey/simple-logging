import { useState, useCallback } from 'react';
import Box from '@mui/material/Box';
import CircularProgress from '@mui/material/CircularProgress';
import Typography from '@mui/material/Typography';
import LogToolbar from './LogToolbar.js';
import LogList from './LogList.js';
import { useLogHistory } from '../../hooks/useLogHistory.js';
import { useLogStream } from '../../hooks/useLogStream.js';
import { useDeploymentLogHistory } from '../../hooks/useDeploymentLogHistory.js';
import { useDeploymentLogStream } from '../../hooks/useDeploymentLogStream.js';
import { useLogStore, useFilteredLines, makeFormatKey } from '../../store/logStore.js';
import { logClient } from '../../grpc/client.js';

export default function LogPanel() {
  const {
    selectedNamespace: namespace,
    selectedPod: pod,
    selectedDeployment: deployment,
    selectionKey,
    mode,
    prevPageToken,
    startTime,
    endTime,
    darkMode,
    isFetchingMore,
    jsonFormats,
  } = useLogStore();

  const [liveEnabled, setLiveEnabled] = useState(false);
  const [autoScroll, setAutoScroll] = useState(true);
  // prependKey increments with every loadOlder call; prependCount carries the
  // number of lines added so LogList can adjust scrollTop even when two
  // consecutive fetches return the same count.
  const [prependKey, setPrependKey] = useState(0);
  const [prependCount, setPrependCount] = useState(0);

  // Always start from the beginning for the initial/filter-driven load.
  const filters = { startTime, endTime, pageToken: '' };

  // Pod-mode hooks (disabled when a deployment is selected)
  useLogHistory(
    !deployment && !liveEnabled ? namespace : null,
    !deployment && !liveEnabled ? pod : null,
    filters,
  );
  useLogStream(namespace, !deployment ? pod : null, liveEnabled);

  // Deployment-mode hooks (disabled when a pod is selected)
  useDeploymentLogHistory(
    deployment && !liveEnabled ? namespace : null,
    deployment && !liveEnabled ? deployment : null,
    filters,
  );
  useDeploymentLogStream(namespace, deployment && liveEnabled ? deployment : null, liveEnabled);

  const filteredLines = useFilteredLines();
  const jsonFormat = namespace ? (jsonFormats[makeFormatKey(namespace, pod, deployment)] ?? null) : null;

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
    if (!namespace || (!pod && !deployment)) return;
    setIsFetchingMore(true);
    try {
      if (deployment) {
        const resp = await logClient.getDeploymentLogs({
          namespace,
          deployment,
          startTime: BigInt(st),
          endTime: BigInt(et),
          pageSize: 200,
          pageToken: token,
        });
        setPrependKey((k) => k + 1);
        setPrependCount(resp.lines.length);
        useLogStore.getState().prependLines(resp.lines);
        useLogStore.getState().setPaginationTokens(resp.prevPageToken, useLogStore.getState().nextPageToken);
      } else if (pod) {
        const resp = await logClient.getLogs({
          namespace,
          pod,
          startTime: BigInt(st),
          endTime: BigInt(et),
          pageSize: 200,
          pageToken: token,
        });
        setPrependKey((k) => k + 1);
        setPrependCount(resp.lines.length);
        useLogStore.getState().prependLines(resp.lines);
        useLogStore.getState().setPaginationTokens(resp.prevPageToken, useLogStore.getState().nextPageToken);
      }
    } catch {
      // ignore fetch errors for load-older
    } finally {
      useLogStore.getState().setIsFetchingMore(false);
    }
  }, [namespace, pod, deployment]);

  // Stable scroll callbacks so LogList's handleRowsRendered doesn't recreate
  // (and re-trigger react-window's onRowsRendered effect) on every render.
  const handleScrollUp = useCallback(() => setAutoScroll(false), []);
  const handleScrollBottom = useCallback(() => setAutoScroll(true), []);

  if (!namespace || (!pod && !deployment)) {
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
        <Typography>Select a pod or deployment from the sidebar to view logs.</Typography>
      </Box>
    );
  }

  return (
    <Box sx={{ flex: 1, display: 'flex', flexDirection: 'column', overflow: 'hidden' }}>
      <LogToolbar
        namespace={namespace}
        pod={pod ?? undefined}
        deployment={deployment ?? undefined}
        liveEnabled={liveEnabled}
        onLiveToggle={handleLiveToggle}
      />

      {mode === 'loading' ? (
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
