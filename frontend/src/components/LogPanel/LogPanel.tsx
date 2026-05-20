import { useState, useCallback } from 'react';
import Box from '@mui/material/Box';
import Button from '@mui/material/Button';
import CircularProgress from '@mui/material/CircularProgress';
import Typography from '@mui/material/Typography';
import LogToolbar from './LogToolbar.js';
import LogList from './LogList.js';
import { useLogHistory } from '../../hooks/useLogHistory.js';
import { useLogStream } from '../../hooks/useLogStream.js';
import { useLogStore, useFilteredLines } from '../../store/logStore.js';

export default function LogPanel() {
  const {
    selectedNamespace: namespace,
    selectedPod: pod,
    mode,
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

  const { loading } = useLogHistory(
    liveEnabled ? null : namespace,
    liveEnabled ? null : pod,
    filters,
  );

  useLogStream(namespace, pod, liveEnabled);

  const filteredLines = useFilteredLines();

  const handleLiveToggle = useCallback((on: boolean) => {
    setLiveEnabled(on);
    if (!on) setPageToken('');
  }, []);

  if (!namespace || !pod) {
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
        <Typography>Select a pod from the sidebar to view logs.</Typography>
      </Box>
    );
  }

  return (
    <Box sx={{ flex: 1, display: 'flex', flexDirection: 'column', overflow: 'hidden' }}>
      <LogToolbar
        namespace={namespace}
        pod={pod}
        liveEnabled={liveEnabled}
        onLiveToggle={handleLiveToggle}
      />

      {mode === 'loading' && loading ? (
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
            onClick={() => setPageToken(prevPageToken)}
          >
            ← Load earlier
          </Button>
          <Button
            size="small"
            disabled={!nextPageToken}
            onClick={() => setPageToken(nextPageToken)}
          >
            Load later →
          </Button>
        </Box>
      )}
    </Box>
  );
}
