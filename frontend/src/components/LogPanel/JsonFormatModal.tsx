import Dialog from '@mui/material/Dialog';
import DialogTitle from '@mui/material/DialogTitle';
import DialogContent from '@mui/material/DialogContent';
import DialogActions from '@mui/material/DialogActions';
import Autocomplete from '@mui/material/Autocomplete';
import TextField from '@mui/material/TextField';
import Button from '@mui/material/Button';
import Box from '@mui/material/Box';
import { useState } from 'react';
import type { JsonFormat } from '../../store/logStore.js';

interface Props {
  open: boolean;
  current: JsonFormat | null;
  candidateKeys: string[];
  onSave: (format: JsonFormat) => void;
  onClear: () => void;
  onClose: () => void;
}

// ── Helpers ────────────────────────────────────────────────────────────────────

const TS_SYNONYMS = ['timestamp', 'ts', 'time', 'datetime', '@timestamp', 'date', 'logged_at', 'log_time'];
const LEVEL_SYNONYMS = ['level', 'lvl', 'severity', 'loglevel', 'log_level', 'log.level', 'logseverity'];
const MSG_SYNONYMS = ['message', 'msg', 'text', 'body', 'content', 'log'];

function computeInitialFields(current: JsonFormat | null, candidateKeys: string[]) {
  if (current) {
    return {
      timestampKey: current.timestampKey ?? '',
      levelKey: current.levelKey ?? '',
      messageKey: current.messageKey ?? '',
    };
  }
  const detectedTs = candidateKeys.find((k) => TS_SYNONYMS.includes(k.toLowerCase())) ?? '';
  const detectedLevel = candidateKeys.find((k) => LEVEL_SYNONYMS.includes(k.toLowerCase())) ?? '';
  const detectedMsg = candidateKeys.find((k) => MSG_SYNONYMS.includes(k.toLowerCase())) ?? '';
  return {
    timestampKey: detectedTs,
    levelKey: detectedLevel === detectedTs ? '' : detectedLevel,
    messageKey: detectedMsg === detectedLevel || detectedMsg === detectedTs ? '' : detectedMsg,
  };
}

// ── Form (only mounted while the dialog is open) ───────────────────────────────

interface FieldsProps {
  current: JsonFormat | null;
  candidateKeys: string[];
  onSave: (format: JsonFormat) => void;
  onClear: () => void;
  onClose: () => void;
}

function JsonFormatFields({ current, candidateKeys, onSave, onClear, onClose }: FieldsProps) {
  const [fields, setFields] = useState(() => computeInitialFields(current, candidateKeys));
  const { timestampKey, levelKey, messageKey } = fields;

  function handleSave() {
    onSave({
      ...(timestampKey.trim() ? { timestampKey: timestampKey.trim() } : {}),
      ...(levelKey.trim() ? { levelKey: levelKey.trim() } : {}),
      ...(messageKey.trim() ? { messageKey: messageKey.trim() } : {}),
    });
  }

  return (
    <>
      <DialogContent style={{ paddingTop: 16 }}>
        <Box sx={{ display: 'flex', flexDirection: 'column', gap: 2 }}>
          <Autocomplete
            freeSolo
            options={candidateKeys}
            value={timestampKey}
            onInputChange={(_, v) => setFields((f) => ({ ...f, timestampKey: v }))}
            size="small"
            renderInput={(params) => (
              <TextField
                {...params}
                label="Timestamp"
                placeholder="e.g. ts, timestamp, time"
                onKeyDown={(e) => { if (e.key === 'Enter') handleSave(); }}
              />
            )}
          />
          <Autocomplete
            freeSolo
            options={candidateKeys}
            value={levelKey}
            onInputChange={(_, v) => setFields((f) => ({ ...f, levelKey: v }))}
            size="small"
            renderInput={(params) => (
              <TextField
                {...params}
                label="Level"
                placeholder="e.g. level, severity"
                onKeyDown={(e) => { if (e.key === 'Enter') handleSave(); }}
              />
            )}
          />
          <Autocomplete
            freeSolo
            options={candidateKeys}
            value={messageKey}
            onInputChange={(_, v) => setFields((f) => ({ ...f, messageKey: v }))}
            size="small"
            renderInput={(params) => (
              <TextField
                {...params}
                label="Message"
                placeholder="e.g. message, msg"
                onKeyDown={(e) => { if (e.key === 'Enter') handleSave(); }}
              />
            )}
          />
        </Box>
      </DialogContent>
      <DialogActions sx={{ px: 3, pb: 2, gap: 1 }}>
        {current && (
          <Button size="small" color="error" variant="text" onClick={onClear} sx={{ mr: 'auto' }}>
            Clear
          </Button>
        )}
        <Button size="small" onClick={onClose}>Cancel</Button>
        <Button size="small" variant="contained" onClick={handleSave}>
          Save
        </Button>
      </DialogActions>
    </>
  );
}

// ── Modal shell ────────────────────────────────────────────────────────────────

export default function JsonFormatModal({ open, current, candidateKeys, onSave, onClear, onClose }: Props) {
  return (
    <Dialog open={open} onClose={onClose} maxWidth="xs" fullWidth>
      <DialogTitle sx={{ pb: 1 }}>Format JSON Log Properties</DialogTitle>
      {open && (
        <JsonFormatFields
          current={current}
          candidateKeys={candidateKeys}
          onSave={onSave}
          onClear={onClear}
          onClose={onClose}
        />
      )}
    </Dialog>
  );
}
