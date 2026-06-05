import { useCallback, useState } from 'react';
import type { FormEvent } from 'react';
import AddIcon from '@mui/icons-material/Add';
import Box from '@mui/material/Box';
import Button from '@mui/material/Button';
import Chip from '@mui/material/Chip';
import CircularProgress from '@mui/material/CircularProgress';
import TextField from '@mui/material/TextField';
import Typography from '@mui/material/Typography';
import { useNavigate } from 'react-router-dom';
import { logClient } from '../../grpc/client.js';
import { useIndexLogHistory } from '../../hooks/useIndexLogHistory.js';
import { useFilteredLines, useLogStore } from '../../store/logStore.js';
import CreateIndexDialog from './CreateIndexDialog.js';
import LogList from './LogList.js';

export default function IndexPanel() {
  const {
    selectedIndexKey,
    selectedIndexValue,
    setSelectedIndex,
    setSelectedIndexValue,
    refreshIndexList,
    selectionKey,
    mode,
    prevPageToken,
    darkMode,
    isFetchingMore,
    searchText,
    setSearchText,
  } = useLogStore();
  const navigate = useNavigate();
  const [dialogOpen, setDialogOpen] = useState(false);
  const [valueDraft, setValueDraft] = useState(selectedIndexValue);
  const [autoScroll, setAutoScroll] = useState(true);
  const [prependKey, setPrependKey] = useState(0);
  const [prependCount, setPrependCount] = useState(0);

  useIndexLogHistory(selectedIndexKey, selectedIndexValue || null);
  const filteredLines = useFilteredLines();

  function handleValueSubmit(e: FormEvent) {
    e.preventDefault();
    setSelectedIndexValue(valueDraft.trim());
    setAutoScroll(true);
  }

  const loadOlder = useCallback(async () => {
    const {
      selectedIndexKey: key,
      selectedIndexValue: value,
      prevPageToken: token,
      isFetchingMore: fetching,
      setIsFetchingMore,
    } = useLogStore.getState();
    if (!key || !value || !token || fetching) return;
    setIsFetchingMore(true);
    try {
      const resp = await logClient.getIndexLogs({
        key,
        value,
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
  }, []);

  return (
    <Box sx={{ flex: 1, display: 'flex', flexDirection: 'column', overflow: 'hidden' }}>
      <Box
        sx={{
          display: 'flex',
          flexWrap: 'wrap',
          alignItems: 'center',
          gap: 1.5,
          px: 2,
          py: 1,
          borderBottom: 1,
          borderColor: 'divider',
        }}
      >
        {selectedIndexKey && (
          <Chip
            label={selectedIndexKey}
            size="small"
            variant="outlined"
            sx={{ fontFamily: 'monospace' }}
          />
        )}
        <Button
          size="small"
          startIcon={<AddIcon />}
          variant={selectedIndexKey ? 'outlined' : 'contained'}
          onClick={() => setDialogOpen(true)}
        >
          Create Index
        </Button>
        {selectedIndexKey && (
          <Box component="form" onSubmit={handleValueSubmit} sx={{ display: 'flex', gap: 1, flex: 1, minWidth: 280 }}>
            <TextField
              size="small"
              label="Value"
              value={valueDraft}
              onChange={(e) => setValueDraft(e.target.value)}
              sx={{ minWidth: 180, flex: '0 1 320px' }}
            />
            <Button type="submit" size="small" variant="contained" disabled={!valueDraft.trim()}>
              Search
            </Button>
            <TextField
              size="small"
              placeholder="Filter results..."
              value={searchText}
              onChange={(e) => setSearchText(e.target.value)}
              sx={{ flex: 1, minWidth: 160 }}
            />
          </Box>
        )}
      </Box>

      {!selectedIndexKey ? (
        <Box sx={{ flex: 1, display: 'flex', alignItems: 'center', justifyContent: 'center', color: 'text.disabled' }}>
          <Typography>Create or select an index to query JSON logs.</Typography>
        </Box>
      ) : !selectedIndexValue ? (
        <Box sx={{ flex: 1, display: 'flex', alignItems: 'center', justifyContent: 'center', color: 'text.disabled' }}>
          <Typography>Enter a value for {selectedIndexKey}.</Typography>
        </Box>
      ) : mode === 'loading' ? (
        <Box sx={{ display: 'flex', justifyContent: 'center', mt: 4 }}>
          <CircularProgress size={32} />
        </Box>
      ) : (
        <LogList
          lines={filteredLines}
          darkMode={darkMode}
          jsonFormat={null}
          autoScroll={autoScroll}
          liveEnabled={false}
          isFetchingMore={isFetchingMore}
          hasOlderLogs={!!prevPageToken}
          lineCount={filteredLines.length}
          selectionKey={selectionKey}
          prependKey={prependKey}
          prependCount={prependCount}
          onScrollUp={() => setAutoScroll(false)}
          onScrollBottom={() => setAutoScroll(true)}
          onNearTop={loadOlder}
        />
      )}

      <CreateIndexDialog
        open={dialogOpen}
        onClose={() => setDialogOpen(false)}
        onCreated={(key) => {
          refreshIndexList();
          setSelectedIndex(key);
          navigate(`/index/${encodeURIComponent(key)}`);
        }}
      />
    </Box>
  );
}
