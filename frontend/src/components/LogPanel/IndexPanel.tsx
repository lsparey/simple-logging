import { useCallback, useMemo, useState } from 'react';
import type { FormEvent } from 'react';
import AddIcon from '@mui/icons-material/Add';
import DeleteIcon from '@mui/icons-material/Delete';
import MenuIcon from '@mui/icons-material/Menu';
import Autocomplete from '@mui/material/Autocomplete';
import Box from '@mui/material/Box';
import Button from '@mui/material/Button';
import Chip from '@mui/material/Chip';
import CircularProgress from '@mui/material/CircularProgress';
import IconButton from '@mui/material/IconButton';
import List from '@mui/material/List';
import ListItemIcon from '@mui/material/ListItemIcon';
import ListItemButton from '@mui/material/ListItemButton';
import Menu from '@mui/material/Menu';
import MenuItem from '@mui/material/MenuItem';
import TextField from '@mui/material/TextField';
import Tooltip from '@mui/material/Tooltip';
import Typography from '@mui/material/Typography';
import { useNavigate } from 'react-router-dom';
import { logClient } from '../../grpc/client.js';
import { useIndexLogHistory } from '../../hooks/useIndexLogHistory.js';
import { useIndexValues } from '../../hooks/useIndexValues.js';
import { makeIndexFormatKey, useFilteredLines, useLogStore } from '../../store/logStore.js';
import { formatDateTime } from '../../utils/formatDateTime.js';
import { observedJsonKeys } from '../../utils/jsonKeys.js';
import CreateIndexDialog from './CreateIndexDialog.js';
import JsonFormatModal from './JsonFormatModal.js';
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
    enterIndexMode,
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
    lines,
    jsonFormats,
    setJsonFormat,
  } = useLogStore();
  const navigate = useNavigate();
  const [dialogOpen, setDialogOpen] = useState(false);
  const [formatModalOpen, setFormatModalOpen] = useState(false);
  const [actionsAnchor, setActionsAnchor] = useState<HTMLElement | null>(null);
  const [valueDraft, setValueDraft] = useState({ key: selectedIndexKey ?? '', value: selectedIndexValue });
  const [autoScroll, setAutoScroll] = useState(true);
  const [prependKey, setPrependKey] = useState(0);
  const [prependCount, setPrependCount] = useState(0);

  const {
    values,
    loading: valuesLoading,
    error: valuesError,
    hasNextPage: hasNextValuesPage,
    hasPrevPage: hasPrevValuesPage,
    nextPage: nextValuesPage,
    prevPage: prevValuesPage,
  } = useIndexValues(selectedIndexKey || null);
  useIndexLogHistory(selectedIndexKey, selectedIndexValue || null);
  const filteredLines = useFilteredLines();
  const formatKey = selectedIndexKey ? makeIndexFormatKey(selectedIndexKey) : '';
  const jsonFormat = formatKey ? (jsonFormats[formatKey] ?? null) : null;
  const candidateKeys = useMemo(() => observedJsonKeys(lines), [lines]);
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

  const openCreateDialog = useCallback(() => {
    setActionsAnchor(null);
    setDialogOpen(true);
  }, []);

  const deleteSelectedIndex = useCallback(async () => {
    if (!selectedIndexKey) return;
    setActionsAnchor(null);
    try {
      await logClient.deleteIndex({ key: selectedIndexKey });
      refreshIndexList();
      enterIndexMode();
      navigate('/indexes');
    } catch (err) {
      window.alert(err instanceof Error ? err.message : 'Failed to delete index');
    }
  }, [enterIndexMode, navigate, refreshIndexList, selectedIndexKey]);

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
        {selectedIndexKey ? (
          <>
            <Tooltip title="Index actions">
              <IconButton
                aria-label="Index actions"
                size="small"
                color="primary"
                onClick={(e) => setActionsAnchor(e.currentTarget)}
              >
                <MenuIcon fontSize="small" />
              </IconButton>
            </Tooltip>
            <Menu
              anchorEl={actionsAnchor}
              open={Boolean(actionsAnchor)}
              onClose={() => setActionsAnchor(null)}
            >
              <MenuItem onClick={openCreateDialog} sx={{ fontSize: '0.8125rem' }}>
                <ListItemIcon sx={{ minWidth: 30 }}>
                  <AddIcon fontSize="small" />
                </ListItemIcon>
                Create another index
              </MenuItem>
              <MenuItem onClick={deleteSelectedIndex} sx={{ fontSize: '0.8125rem' }}>
                <ListItemIcon sx={{ minWidth: 30 }}>
                  <DeleteIcon fontSize="small" />
                </ListItemIcon>
                Delete index
              </MenuItem>
            </Menu>
          </>
        ) : (
          <Button
            size="small"
            startIcon={<AddIcon />}
            variant="contained"
            onClick={openCreateDialog}
          >
            Create Index
          </Button>
        )}
        {selectedIndexKey && (
          <Chip
            label={selectedIndexKey}
            size="small"
            variant="outlined"
            sx={{ fontFamily: 'monospace' }}
          />
        )}
        {selectedIndexKey && !selectedIndexValue && (
          <Box
            component="form"
            onSubmit={handleValueSubmit}
            sx={{ display: 'flex', gap: 1, ml: 'auto', minWidth: 280 }}
          >
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
              sx={{ minWidth: 280, width: 420 }}
            />
            <Button type="submit" size="small" variant="contained" disabled={!draftValue.trim()}>
              Go
            </Button>
          </Box>
        )}
        {selectedIndexKey && selectedIndexValue && (
          <Box sx={{ display: 'flex', gap: 1, flex: 1, minWidth: 280 }}>
            <Chip
              label="{JSON}"
              size="small"
              variant="outlined"
              onClick={() => setFormatModalOpen(true)}
              sx={{
                alignSelf: 'center',
                height: 20,
                fontSize: '0.7rem',
                fontFamily: 'monospace',
                cursor: 'pointer',
                color: 'warning.main',
                borderColor: 'warning.main',
                borderStyle: 'solid',
                '& .MuiChip-label': { px: 0.75 },
              }}
            />
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
            <>
              <List disablePadding>
                {values.map((item) => (
                  <ListItemButton
                    key={item.value}
                    divider
                    onClick={() => selectValue(item.value)}
                    sx={{ px: 2, py: 1 }}
                  >
                    <Typography
                      sx={{
                        flex: 1,
                        minWidth: 0,
                        fontFamily: 'monospace',
                        overflow: 'hidden',
                        textOverflow: 'ellipsis',
                        whiteSpace: 'nowrap',
                      }}
                    >
                      {previewValue(item.value)}
                    </Typography>
                    <Typography
                      variant="body2"
                      color="text.secondary"
                      sx={{ ml: 2, whiteSpace: 'nowrap' }}
                    >
                      {item.lastUpdatedUnixMs > 0n ? formatDateTime(item.lastUpdatedUnixMs) : '-'}
                    </Typography>
                    <Chip size="small" label={formatCount(item.count)} sx={{ ml: 2 }} />
                  </ListItemButton>
                ))}
              </List>
              {(hasPrevValuesPage || hasNextValuesPage) && (
                <Box
                  sx={{
                    display: 'flex',
                    justifyContent: 'flex-end',
                    gap: 1,
                    px: 2,
                    py: 1.5,
                    borderTop: 1,
                    borderColor: 'divider',
                  }}
                >
                  <Button size="small" disabled={!hasPrevValuesPage || valuesLoading} onClick={prevValuesPage}>
                    Previous
                  </Button>
                  <Button size="small" disabled={!hasNextValuesPage || valuesLoading} onClick={nextValuesPage}>
                    Next
                  </Button>
                </Box>
              )}
            </>
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
          jsonFormat={jsonFormat}
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
      <JsonFormatModal
        open={formatModalOpen}
        current={jsonFormat}
        candidateKeys={candidateKeys}
        onSave={(format) => {
          setJsonFormat(formatKey, format);
          setFormatModalOpen(false);
        }}
        onClear={() => {
          setJsonFormat(formatKey, null);
          setFormatModalOpen(false);
        }}
        onClose={() => setFormatModalOpen(false)}
      />
    </Box>
  );
}
