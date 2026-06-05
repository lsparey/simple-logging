import { useMemo, useState } from 'react';
import Autocomplete from '@mui/material/Autocomplete';
import Button from '@mui/material/Button';
import Dialog from '@mui/material/Dialog';
import DialogActions from '@mui/material/DialogActions';
import DialogContent from '@mui/material/DialogContent';
import DialogTitle from '@mui/material/DialogTitle';
import TextField from '@mui/material/TextField';
import { logClient } from '../../grpc/client.js';
import { useLogStore } from '../../store/logStore.js';
import { candidateJsonKeys } from '../../utils/jsonKeys.js';

interface Props {
  open: boolean;
  onClose: () => void;
  onCreated: (key: string) => void;
}

export default function CreateIndexDialog({ open, onClose, onCreated }: Props) {
  const lines = useLogStore((s) => s.lines);
  const suggestions = useMemo(() => candidateJsonKeys(lines), [lines]);
  const [key, setKey] = useState('');
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);

  async function handleCreate() {
    const trimmed = key.trim();
    if (!trimmed) return;
    setSaving(true);
    setError(null);
    try {
      const resp = await logClient.createIndex({ key: trimmed });
      onCreated(resp.index?.key ?? trimmed);
      setKey('');
      onClose();
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to create index');
    } finally {
      setSaving(false);
    }
  }

  return (
    <Dialog open={open} onClose={saving ? undefined : onClose} maxWidth="xs" fullWidth>
      <DialogTitle>Create Index</DialogTitle>
      <DialogContent sx={{ pt: '16px !important' }}>
        <Autocomplete
          freeSolo
          options={suggestions}
          value={key}
          onInputChange={(_, value) => setKey(value)}
          renderInput={(params) => (
            <TextField
              {...params}
              autoFocus
              label="JSON key"
              error={!!error}
              helperText={error ?? ' '}
              onKeyDown={(e) => {
                if (e.key === 'Enter') void handleCreate();
              }}
            />
          )}
        />
      </DialogContent>
      <DialogActions>
        <Button onClick={onClose} disabled={saving}>Cancel</Button>
        <Button onClick={handleCreate} disabled={saving || !key.trim()} variant="contained">
          Create
        </Button>
      </DialogActions>
    </Dialog>
  );
}
