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
import { useLogStore, useFilteredLines } from '../../store/logStore.js';
import { logClient } from '../../grpc/client.js';

export default function LogPanel() {
  const {
    selectedNamespace: namespace,
    selectedPod: pod,
    selectedDeployment: deployment,
    mode,
    nextPageToken,
    startTime,
    endTime,
    darkMode,
    isFetchingMore,
  } = useLogStore();

  const [liveEnabled, setLiveEnabled] = useState(false);
  const [autoScroll, setAutoScroll] = useState(true);

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

  const handleLiveToggle = useCallback((on: boolean) => {
    setLiveEnabled(on);
  }, []);

  // Loads the next page and appends it to the existing lines.
  // Reads nextPageToken / isFetchingMore from store at call-time so this callback
  // stays stable (only recreates when the selected resource changes), preventing
  // react-window's onRowsRendered effect from firing on every store update and
  // triggering duplicate fetches.
  const loadMore = useCallback(async () => {
    const {
      nextPageToken: token,
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
        useLogStore.getState().appendLines(resp.lines);
        useLogStore.getState().setNextPageToken(resp.nextPageToken);
      } else if (pod) {
        const resp = await logClient.getLogs({
          namespace,
          pod,
          startTime: BigInt(st),
          endTime: BigInt(et),
          pageSize: 200,
          pageToken: token,
        });
        useLogStore.getState().appendLines(resp.lines);
        useLogStore.getState().setNextPageToken(resp.nextPageToken);
      }
    } catch {
      // ignore fetch errors for load-more
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
          autoScroll={autoScroll}
          onScrollUp={handleScrollUp}
          onScrollBottom={handleScrollBottom}
          onNearBottom={!liveEnabled ? loadMore : undefined}
        />
      )}

      {!liveEnabled && mode !== 'loading' && (
        <Box
          sx={{
            display: 'flex',
            justifyContent: 'space-between',
            alignItems: 'center',
            px: 2,
            py: 0.5,
            borderTop: 1,
            borderColor: 'divider',
          }}
        >
          <Typography variant="caption" sx={{ color: 'text.secondary' }}>
            {filteredLines.length} line{filteredLines.length !== 1 ? 's' : ''} loaded
          </Typography>
          {isFetchingMore ? (
            <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.5 }}>
              <CircularProgress size={12} />
              <Typography variant="caption" sx={{ color: 'text.secondary' }}>Loading more…</Typography>
            </Box>
          ) : nextPageToken ? (
            <Typography variant="caption" sx={{ color: 'text.disabled' }}>↓ Scroll for more</Typography>
          ) : (
            <Typography variant="caption" sx={{ color: 'text.disabled' }}>End of log</Typography>
          )}
        </Box>
      )}
    </Box>
  );
}
