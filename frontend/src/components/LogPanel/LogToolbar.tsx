import { useState, useMemo } from 'react';
import Box from '@mui/material/Box';
import TextField from '@mui/material/TextField';
import FormControlLabel from '@mui/material/FormControlLabel';
import Switch from '@mui/material/Switch';
import Chip from '@mui/material/Chip';
import { DateTimePicker } from '@mui/x-date-pickers/DateTimePicker';
import { AdapterDayjs } from '@mui/x-date-pickers/AdapterDayjs';
import { LocalizationProvider } from '@mui/x-date-pickers/LocalizationProvider';
import dayjs, { type Dayjs } from 'dayjs';
import { useLogStore, makeFormatKey } from '../../store/logStore.js';
import JsonFormatModal from './JsonFormatModal.js';

interface Props {
  namespace: string;
  /** Either `pod` or `deployment` is set, not both. */
  pod?: string;
  deployment?: string;
  liveEnabled: boolean;
  onLiveToggle: (on: boolean) => void;
}

export default function LogToolbar({ namespace, pod, deployment, liveEnabled, onLiveToggle }: Props) {
  const { searchText, setSearchText, startTime, endTime, setTimeRange, jsonLogging, jsonFormats, setJsonFormat, lines } = useLogStore();
  const [modalOpen, setModalOpen] = useState(false);

  const formatKey = makeFormatKey(namespace, pod, deployment);
  const jsonFormat = jsonFormats[formatKey] ?? null;

  const label = deployment ? deployment : pod;

  // Extract JSON property keys that appear in every sampled JSON log line.
  // Sampling up to 100 lines keeps this cheap even for large buffers.
  const candidateKeys = useMemo(() => {
    const SAMPLE = 100;
    const PREFIX_JSON_RE = /^(\S+) \[\S+\] ([\s\S]*)$/;
    const keyCounts = new Map<string, number>();
    let jsonLineCount = 0;

    const sample = lines.length > SAMPLE
      ? lines.slice(0, Math.ceil(SAMPLE / 2)).concat(lines.slice(-Math.floor(SAMPLE / 2)))
      : lines;

    for (const line of sample) {
      const m = PREFIX_JSON_RE.exec(line);
      const payload = m ? m[2] : line;
      const trimmed = payload.trimStart();
      if (trimmed[0] !== '{') continue;
      try {
        const obj = JSON.parse(trimmed) as Record<string, unknown>;
        if (obj === null || typeof obj !== 'object') continue;
        jsonLineCount++;
        for (const key of Object.keys(obj)) {
          keyCounts.set(key, (keyCounts.get(key) ?? 0) + 1);
        }
      } catch { /* not json */ }
    }

    if (jsonLineCount === 0) return [];

    // Keep keys that appear in every parsed JSON line, sorted alphabetically.
    return [...keyCounts.entries()]
      .filter(([, count]) => count === jsonLineCount)
      .map(([key]) => key)
      .sort();
  }, [lines]);

  return (
    <LocalizationProvider dateAdapter={AdapterDayjs}>
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
            label="{..}"
            size="small"
            variant="outlined"
            onClick={() => setModalOpen(true)}
            sx={{
              height: 20,
              fontSize: '0.7rem',
              fontFamily: 'monospace',
              cursor: 'pointer',
              color: 'warning.main',
              borderColor: jsonFormat ? 'warning.main' : 'warning.main',
              borderStyle: jsonFormat ? 'solid' : 'dashed',
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

        <DateTimePicker
          label="Start"
          value={startTime ? dayjs.unix(startTime) : null}
          onChange={(v: Dayjs | null) =>
            setTimeRange(v ? v.unix() : 0, endTime)
          }
          disabled={liveEnabled}
          slotProps={{ textField: { size: 'small', sx: { width: 200 } } }}
        />

        <DateTimePicker
          label="End"
          value={endTime ? dayjs.unix(endTime) : null}
          onChange={(v: Dayjs | null) =>
            setTimeRange(startTime, v ? v.unix() : 0)
          }
          disabled={liveEnabled}
          slotProps={{ textField: { size: 'small', sx: { width: 200 } } }}
        />

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
    </LocalizationProvider>
  );
}
