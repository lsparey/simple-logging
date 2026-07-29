import { useState } from 'react';
import Box from '@mui/material/Box';
import Button from '@mui/material/Button';
import Dialog from '@mui/material/Dialog';
import DialogActions from '@mui/material/DialogActions';
import DialogContent from '@mui/material/DialogContent';
import DialogTitle from '@mui/material/DialogTitle';
import ContentCopyIcon from '@mui/icons-material/ContentCopy';
import CheckIcon from '@mui/icons-material/Check';
import LogLine from './LogLine.js';
import type { JsonFormat } from '../../store/logStore.js';

interface Props {
  open: boolean;
  line: string | null;
  darkMode: boolean;
  jsonFormat?: JsonFormat | null;
  onClose: () => void;
}

export default function LogMessageModal({ open, line, darkMode, jsonFormat, onClose }: Props) {
  const [copied, setCopied] = useState(false);

  async function handleCopy() {
    if (line === null) return;
    await navigator.clipboard.writeText(line);
    setCopied(true);
    setTimeout(() => setCopied(false), 1500);
  }

  return (
    <Dialog open={open} onClose={onClose} maxWidth="md" fullWidth>
      <DialogTitle>Log message</DialogTitle>
      <DialogContent sx={{ pt: '16px !important' }}>
        <Box
          sx={{
            p: 1.5,
            bgcolor: 'action.hover',
            borderRadius: 1,
            userSelect: 'text',
          }}
        >
          {line !== null && <LogLine line={line} darkMode={darkMode} jsonFormat={jsonFormat} wrap />}
        </Box>
      </DialogContent>
      <DialogActions sx={{ px: 3, pb: 2 }}>
        <Button
          size="small"
          startIcon={copied ? <CheckIcon /> : <ContentCopyIcon />}
          onClick={handleCopy}
        >
          {copied ? 'Copied!' : 'Copy'}
        </Button>
        <Button size="small" variant="contained" onClick={onClose}>Close</Button>
      </DialogActions>
    </Dialog>
  );
}
