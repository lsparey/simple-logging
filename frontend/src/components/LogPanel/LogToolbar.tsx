import { useState, useMemo } from 'react';
import Box from '@mui/material/Box';
import TextField from '@mui/material/TextField';
import FormControlLabel from '@mui/material/FormControlLabel';
import Switch from '@mui/material/Switch';
import Chip from '@mui/material/Chip';
import FormControl from '@mui/material/FormControl';
import Select, { type SelectChangeEvent } from '@mui/material/Select';
import MenuItem from '@mui/material/MenuItem';
import { useLogStore, makeFormatKey } from '../../store/logStore.js';
import JsonFormatModal from './JsonFormatModal.js';
import LogHistogram from './LogHistogram.js';
import { candidateJsonKeys } from '../../utils/jsonKeys.js';

interface Props {
  namespace: string;
  /** Either `pod` or `deployment` is set, not both. */
  pod?: string;
  deployment?: string;
  liveEnabled: boolean;
  onLiveToggle: (on: boolean) => void;
}

export default function LogToolbar({ namespace, pod, deployment, liveEnabled, onLiveToggle }: Props) {
  const {
    searchText,
    setSearchText,
    jsonLogging,
    jsonFormats,
    setJsonFormat,
    lines,
    selectedPodContainers,
    selectedContainer,
    setSelectedContainer,
  } = useLogStore();
  const [modalOpen, setModalOpen] = useState(false);

  const formatKey = makeFormatKey(namespace, pod, deployment);
  const jsonFormat = jsonFormats[formatKey] ?? null;

  const label = deployment ? deployment : pod;

  const candidateKeys = useMemo(() => candidateJsonKeys(lines), [lines]);

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
          label={label}
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

        {pod && selectedPodContainers.length > 1 && (
          <FormControl size="small" sx={{ minWidth: 150 }}>
            <Select
              value={selectedContainer ?? ''}
              displayEmpty
              onChange={(e: SelectChangeEvent) => setSelectedContainer(e.target.value || null)}
              inputProps={{ 'aria-label': 'Filter by container' }}
            >
              <MenuItem value="">All containers</MenuItem>
              {selectedPodContainers.map((container) => (
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
