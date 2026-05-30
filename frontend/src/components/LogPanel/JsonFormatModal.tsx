import { useState, useEffect } from 'react';
import Dialog from '@mui/material/Dialog';
import DialogTitle from '@mui/material/DialogTitle';
import DialogContent from '@mui/material/DialogContent';
import DialogActions from '@mui/material/DialogActions';
import Autocomplete from '@mui/material/Autocomplete';
import TextField from '@mui/material/TextField';
import Button from '@mui/material/Button';
import Box from '@mui/material/Box';
import Typography from '@mui/material/Typography';
import type { JsonFormat } from '../../store/logStore.js';

interface Props {
  open: boolean;
  current: JsonFormat | null;
  candidateKeys: string[];
  onSave: (format: JsonFormat) => void;
  onClear: () => void;
  onClose: () => void;
}

export default function JsonFormatModal({ open, current, candidateKeys, onSave, onClear, onClose }: Props) {
  const [levelKey, setLevelKey] = useState('');
  const [messageKey, setMessageKey] = useState('');

  // Sync fields when modal opens
  useEffect(() => {
    if (!open) return;
    if (current) {
      setLevelKey(current.levelKey);
      setMessageKey(current.messageKey);
      return;
    }
    // Auto-detect from candidate keys using known synonyms; otherwise leave blank.
    const LEVEL_SYNONYMS = ['level', 'lvl', 'severity', 'loglevel', 'log_level', 'log.level', 'logseverity'];
    const MSG_SYNONYMS = ['message', 'msg', 'text', 'body', 'content', 'log'];
    const detectedLevel = candidateKeys.find((k) => LEVEL_SYNONYMS.includes(k.toLowerCase())) ?? '';
    const detectedMsg = candidateKeys.find((k) => MSG_SYNONYMS.includes(k.toLowerCase())) ?? '';
    // Avoid setting both fields to the same key
    setLevelKey(detectedLevel);
    setMessageKey(detectedMsg === detectedLevel ? '' : detectedMsg);
  }, [open, current, candidateKeys]);

  const canSave = levelKey.trim() !== '' && messageKey.trim() !== '';

  function handleSave() {
    if (!canSave) return;
    onSave({ levelKey: levelKey.trim(), messageKey: messageKey.trim() });
  }

  return (
    <Dialog open={open} onClose={onClose} maxWidth="xs" fullWidth>
      <DialogTitle sx={{ pb: 1 }}>JSON Log Format</DialogTitle>
      <DialogContent>
        <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
          Specify the JSON property names used for log level and message. When set,
          log lines will be formatted with a colourised level, teal message, and
          greyed-out full JSON payload.
        </Typography>
        <Box sx={{ display: 'flex', flexDirection: 'column', gap: 2 }}>
          <Autocomplete
            freeSolo
            options={candidateKeys}
            value={levelKey}
            onInputChange={(_, v) => setLevelKey(v)}
            size="small"
            renderInput={(params) => (
              <TextField
                {...params}
                label="Level property"
                placeholder="e.g. level, severity"
                onKeyDown={(e) => { if (e.key === 'Enter' && canSave) handleSave(); }}
              />
            )}
          />
          <Autocomplete
            freeSolo
            options={candidateKeys}
            value={messageKey}
            onInputChange={(_, v) => setMessageKey(v)}
            size="small"
            renderInput={(params) => (
              <TextField
                {...params}
                label="Message property"
                placeholder="e.g. message, msg"
                onKeyDown={(e) => { if (e.key === 'Enter' && canSave) handleSave(); }}
              />
            )}
          />
        </Box>
      </DialogContent>
      <DialogActions sx={{ px: 3, pb: 2, gap: 1 }}>
        {current && (
          <Button
            size="small"
            color="error"
            variant="text"
            onClick={onClear}
            sx={{ mr: 'auto' }}
          >
            Clear
          </Button>
        )}
        <Button size="small" onClick={onClose}>Cancel</Button>
        <Button size="small" variant="contained" disabled={!canSave} onClick={handleSave}>
          Save
        </Button>
      </DialogActions>
    </Dialog>
  );
}
