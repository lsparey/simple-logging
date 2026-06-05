import { useCallback, useState } from 'react';
import type { FormEvent } from 'react';
import AddIcon from '@mui/icons-material/Add';
import Autocomplete from '@mui/material/Autocomplete';
import Box from '@mui/material/Box';
import Button from '@mui/material/Button';
import Chip from '@mui/material/Chip';
import CircularProgress from '@mui/material/CircularProgress';
import List from '@mui/material/List';
import ListItemButton from '@mui/material/ListItemButton';
import ListItemText from '@mui/material/ListItemText';
import TextField from '@mui/material/TextField';
import Typography from '@mui/material/Typography';
import { useNavigate } from 'react-router-dom';
import { logClient } from '../../grpc/client.js';
import { useIndexLogHistory } from '../../hooks/useIndexLogHistory.js';
import { useIndexValues } from '../../hooks/useIndexValues.js';
import { useFilteredLines, useLogStore } from '../../store/logStore.js';
import CreateIndexDialog from './CreateIndexDialog.js';
import LogList from './LogList.js';

function formatCount(count: bigint) {
  return new Intl.NumberFormat().format(Number(count));
}

function previewValue(value: string) {
  return value.length > 240 ? `${value.slice(0, 240)}...` : value;
}

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
  const [valueDraft, setValueDraft] = useState({ key: selectedIndexKey ?? '', value: selectedIndexValue });
  const [autoScroll, setAutoScroll] = useState(true);
  const [prependKey, setPrependKey] = useState(0);
  const [prependCount, setPrependCount] = useState(0);

  const {
    values,
    loading: valuesLoading,
    error: valuesError,
  } = useIndexValues(selectedIndexKey || null);
  useIndexLogHistory(selectedIndexKey, selectedIndexValue || null);
  const filteredLines = useFilteredLines();
  const draftValue = valueDraft.key === (selectedIndexKey ?? '') ? valueDraft.value : selectedIndexValue;
  const valueOptions = values.map((item) => item.value);

  function handleValueSubmit(e: FormEvent) {
    e.preventDefault();
    setSelectedIndexValue(draftValue.trim());
    setAutoScroll(true);
  }

  const selectValue = useCallback((value: string) => {
    setValueDraft({ key: selectedIndexKey ?? '', value });
    setSelectedIndexValue(value);
    setAutoScroll(true);
  }, [selectedIndexKey, setSelectedIndexValue]);

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

  const handleScrollUp = useCallback(() => setAutoScroll(false), []);
  const handleScrollBottom = useCallback(() => setAutoScroll(true), []);

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
        {selectedIndexKey && !selectedIndexValue && (
          <Box component="form" onSubmit={handleValueSubmit} sx={{ display: 'flex', gap: 1, flex: 1, minWidth: 280 }}>
            <Autocomplete
              freeSolo
              options={valueOptions}
              inputValue={draftValue}
              getOptionLabel={(option) => previewValue(option)}
              onInputChange={(_, value) => setValueDraft({ key: selectedIndexKey ?? '', value })}
              renderOption={(props, option) => {
                const { key, ...optionProps } = props;
                return (
                  <Box component="li" key={key} {...optionProps} sx={{ fontFamily: 'monospace' }}>
                    {previewValue(option)}
                  </Box>
                );
              }}
              renderInput={(params) => <TextField {...params} size="small" label="Value" />}
              sx={{ minWidth: 180, flex: '0 1 320px' }}
            />
            <Button type="submit" size="small" variant="contained" disabled={!draftValue.trim()}>
              Go
            </Button>
          </Box>
        )}
        {selectedIndexKey && selectedIndexValue && (
          <Box sx={{ display: 'flex', gap: 1, flex: 1, minWidth: 280 }}>
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
        <Box sx={{ flex: 1, overflow: 'auto' }}>
          {valuesLoading ? (
            <Box sx={{ display: 'flex', justifyContent: 'center', mt: 4 }}>
              <CircularProgress size={32} />
            </Box>
          ) : valuesError ? (
            <Box sx={{ display: 'flex', alignItems: 'center', justifyContent: 'center', height: '100%', color: 'error.main' }}>
              <Typography>{valuesError}</Typography>
            </Box>
          ) : values.length === 0 ? (
            <Box sx={{ display: 'flex', alignItems: 'center', justifyContent: 'center', height: '100%', color: 'text.disabled' }}>
              <Typography>No values found for {selectedIndexKey}.</Typography>
            </Box>
          ) : (
            <List disablePadding>
              {values.map((item, idx) => (
                <ListItemButton
                  key={`${idx}-${item.count}`}
                  divider
                  onClick={() => selectValue(item.value)}
                  sx={{ px: 2, py: 1 }}
                  >
                  <ListItemText
                    primary={previewValue(item.value)}
                    slotProps={{
                      primary: {
                        sx: {
                          fontFamily: 'monospace',
                          overflow: 'hidden',
                          textOverflow: 'ellipsis',
                          whiteSpace: 'nowrap',
                        },
                      },
                    }}
                  />
                  <Chip size="small" label={formatCount(item.count)} />
                </ListItemButton>
              ))}
            </List>
          )}
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
          onScrollUp={handleScrollUp}
          onScrollBottom={handleScrollBottom}
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
