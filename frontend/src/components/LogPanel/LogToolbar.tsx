import Box from '@mui/material/Box';
import TextField from '@mui/material/TextField';
import FormControlLabel from '@mui/material/FormControlLabel';
import Switch from '@mui/material/Switch';
import Chip from '@mui/material/Chip';
import { DateTimePicker } from '@mui/x-date-pickers/DateTimePicker';
import { AdapterDayjs } from '@mui/x-date-pickers/AdapterDayjs';
import { LocalizationProvider } from '@mui/x-date-pickers/LocalizationProvider';
import dayjs, { type Dayjs } from 'dayjs';
import { useLogStore } from '../../store/logStore.js';

interface Props {
  namespace: string;
  /** Either `pod` or `deployment` is set, not both. */
  pod?: string;
  deployment?: string;
  liveEnabled: boolean;
  onLiveToggle: (on: boolean) => void;
}

export default function LogToolbar({ pod, deployment, liveEnabled, onLiveToggle }: Props) {
  const { searchText, setSearchText, startTime, endTime, setTimeRange, jsonLogging } = useLogStore();

  const label = deployment ? deployment : pod;

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
            sx={{
              height: 20,
              fontSize: '0.7rem',
              fontFamily: 'monospace',
              color: 'warning.main',
              borderColor: 'warning.main',
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
    </LocalizationProvider>
  );
}
