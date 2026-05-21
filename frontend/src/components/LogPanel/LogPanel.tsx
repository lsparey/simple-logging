import { useState, useCallback } from 'react';
import Box from '@mui/material/Box';
import Button from '@mui/material/Button';
import CircularProgress from '@mui/material/CircularProgress';
import Typography from '@mui/material/Typography';
import LogToolbar from './LogToolbar.js';
import LogList from './LogList.js';
import { useLogHistory } from '../../hooks/useLogHistory.js';
import { useLogStream } from '../../hooks/useLogStream.js';
import { useDeploymentLogHistory } from '../../hooks/useDeploymentLogHistory.js';
import { useDeploymentLogStream } from '../../hooks/useDeploymentLogStream.js';
import { useLogStore, useFilteredLines } from '../../store/logStore.js';

export default function LogPanel() {
  const {
    selectedNamespace: namespace,
    selectedPod: pod,
    selectedDeployment: deployment,
    mode,
    setMode,
    nextPageToken,
    prevPageToken,
    startTime,
    endTime,
    darkMode,
  } = useLogStore();

  const [liveEnabled, setLiveEnabled] = useState(false);
  const [pageToken, setPageToken] = useState('');
  const [autoScroll, setAutoScroll] = useState(true);

  const filters = { startTime, endTime, pageToken };

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
    if (!on) setPageToken('');
  }, []);

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
          onScrollUp={() => setAutoScroll(false)}
          onScrollBottom={() => setAutoScroll(true)}
        />
      )}

      {!liveEnabled && (
        <Box
          sx={{
            display: 'flex',
            justifyContent: 'space-between',
            px: 2,
            py: 0.75,
            borderTop: 1,
            borderColor: 'divider',
          }}
        >
          <Button
            size="small"
            disabled={!prevPageToken}
            onClick={() => { setMode('loading'); setPageToken(prevPageToken); }}
          >
            ← Load earlier
          </Button>
          <Button
            size="small"
            disabled={!nextPageToken}
            onClick={() => { setMode('loading'); setPageToken(nextPageToken); }}
          >
            Load later →
          </Button>
        </Box>
      )}
    </Box>
  );
}
