import { useState, useMemo } from 'react';
import Box from '@mui/material/Box';
import TextField from '@mui/material/TextField';
import FormControlLabel from '@mui/material/FormControlLabel';
import Switch from '@mui/material/Switch';
import Chip from '@mui/material/Chip';
import FormControl from '@mui/material/FormControl';
import Select, { type SelectChangeEvent } from '@mui/material/Select';
import MenuItem from '@mui/material/MenuItem';
import { useLogStore, makeFormatKey, linePod, lineContainer } from '../../store/logStore.js';
import JsonFormatModal from './JsonFormatModal.js';
import LogHistogram from './LogHistogram.js';
import { candidateJsonKeys } from '../../utils/jsonKeys.js';

interface Props {
  namespace: string;
  kind: string;
  name: string;
  liveEnabled: boolean;
  onLiveToggle: (on: boolean) => void;
}

export default function LogToolbar({ namespace, kind, name, liveEnabled, onLiveToggle }: Props) {
  const {
    searchText,
    setSearchText,
    jsonLogging,
    jsonFormats,
    setJsonFormat,
    lines,
    selectedPodFilter,
    setSelectedPodFilter,
    selectedContainerFilter,
    setSelectedContainerFilter,
  } = useLogStore();
  const [modalOpen, setModalOpen] = useState(false);

  const formatKey = makeFormatKey(namespace, kind, name);
  const jsonFormat = jsonFormats[formatKey] ?? null;

  const candidateKeys = useMemo(() => candidateJsonKeys(lines), [lines]);

  const podOptions = useMemo(() => {
    const set = new Set<string>();
    for (const line of lines) {
      const pod = linePod(line);
      if (pod) set.add(pod);
    }
    return Array.from(set).sort();
  }, [lines]);

  const containerOptions = useMemo(() => {
    const set = new Set<string>();
    for (const line of lines) {
      const container = lineContainer(line);
      if (container) set.add(container);
    }
    return Array.from(set).sort();
  }, [lines]);

  return (
    <>
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
        <Chip
          label={name}
          size="small"
          variant="outlined"
          sx={{ fontFamily: 'monospace' }}
        />
        {jsonLogging && (
          <Chip
            label="{JSON}"
            size="small"
            variant="outlined"
            onClick={() => setModalOpen(true)}
            sx={{
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
        )}

        <FormControlLabel
          control={
            <Switch
              size="small"
              checked={liveEnabled}
              onChange={(e) => onLiveToggle(e.target.checked)}
              color="success"
            />
          }
          label="Live"
          sx={{ ml: 0.5 }}
        />

        {podOptions.length > 1 && (
          <FormControl size="small" sx={{ minWidth: 150 }}>
            <Select
              value={selectedPodFilter ?? ''}
              displayEmpty
              onChange={(e: SelectChangeEvent) => setSelectedPodFilter(e.target.value || null)}
              inputProps={{ 'aria-label': 'Filter by pod' }}
            >
              <MenuItem value="">All pods</MenuItem>
              {podOptions.map((pod) => (
                <MenuItem key={pod} value={pod}>{pod}</MenuItem>
              ))}
            </Select>
          </FormControl>
        )}

        {containerOptions.length > 1 && (
          <FormControl size="small" sx={{ minWidth: 150 }}>
            <Select
              value={selectedContainerFilter ?? ''}
              displayEmpty
              onChange={(e: SelectChangeEvent) => setSelectedContainerFilter(e.target.value || null)}
              inputProps={{ 'aria-label': 'Filter by container' }}
            >
              <MenuItem value="">All containers</MenuItem>
              {containerOptions.map((container) => (
                <MenuItem key={container} value={container}>{container}</MenuItem>
              ))}
            </Select>
          </FormControl>
        )}

        <LogHistogram />

        <TextField
          size="small"
          placeholder="Search…"
          value={searchText}
          onChange={(e) => setSearchText(e.target.value)}
          sx={{ flex: 1, minWidth: 160 }}
        />
      </Box>

      <JsonFormatModal
        open={modalOpen}
        current={jsonFormat}
        candidateKeys={candidateKeys}
        onSave={(fmt) => { setJsonFormat(formatKey, fmt); setModalOpen(false); }}
        onClear={() => { setJsonFormat(formatKey, null); setModalOpen(false); }}
        onClose={() => setModalOpen(false)}
      />
    </>
  );
}
