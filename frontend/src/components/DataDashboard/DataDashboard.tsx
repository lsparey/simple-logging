import Alert from '@mui/material/Alert';
import Box from '@mui/material/Box';
import Button from '@mui/material/Button';
import CircularProgress from '@mui/material/CircularProgress';
import Paper from '@mui/material/Paper';
import Table from '@mui/material/Table';
import TableBody from '@mui/material/TableBody';
import TableCell from '@mui/material/TableCell';
import TableContainer from '@mui/material/TableContainer';
import TableHead from '@mui/material/TableHead';
import TableRow from '@mui/material/TableRow';
import Typography from '@mui/material/Typography';
import RefreshIcon from '@mui/icons-material/Refresh';
import StorageIcon from '@mui/icons-material/Storage';
import { useLogFiles } from '../../hooks/useLogFiles.js';
import { formatBytes } from '../../utils/formatBytes.js';
import { formatDateTime } from '../../utils/formatDateTime.js';

export default function DataDashboard() {
  const { files, totalSizeBytes, loading, error, refresh } = useLogFiles();
  const sortedFiles = [...files].sort((a, b) => {
    if (a.sizeBytes === b.sizeBytes) {
      return `${a.namespace}/${a.name}`.localeCompare(`${b.namespace}/${b.name}`);
    }
    return a.sizeBytes > b.sizeBytes ? -1 : 1;
  });

  return (
    <Box sx={{ p: 3, overflow: 'auto', height: '100%' }}>
      <Box sx={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', mb: 3 }}>
        <Box>
          <Typography variant="h4" component="h1">Data dashboard</Typography>
          <Typography color="text.secondary">Storage used by persisted log and index files.</Typography>
        </Box>
        <Button startIcon={<RefreshIcon />} onClick={refresh} disabled={loading}>
          Refresh
        </Button>
      </Box>

      <Box sx={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(220px, 1fr))', gap: 2, mb: 3 }}>
        <Paper variant="outlined" sx={{ p: 2.5 }}>
          <StorageIcon color="primary" sx={{ mb: 1 }} />
          <Typography variant="body2" color="text.secondary">Total size</Typography>
          <Typography variant="h4" sx={{ fontFamily: 'monospace' }}>
            {formatBytes(totalSizeBytes)}
          </Typography>
          <Typography variant="caption" color="text.secondary">
            {totalSizeBytes.toLocaleString()} bytes
          </Typography>
        </Paper>
        <Paper variant="outlined" sx={{ p: 2.5 }}>
          <Typography variant="body2" color="text.secondary">Data files</Typography>
          <Typography variant="h4" sx={{ fontFamily: 'monospace', mt: 2 }}>
            {files.length}
          </Typography>
        </Paper>
      </Box>

      {error && (
        <Alert
          severity="error"
          action={<Button color="inherit" size="small" onClick={refresh}>Retry</Button>}
          sx={{ mb: 2 }}
        >
          {error}
        </Alert>
      )}

      <TableContainer component={Paper} variant="outlined">
        <Table aria-label="Data file sizes">
          <TableHead>
            <TableRow>
              <TableCell>Type</TableCell>
              <TableCell>Namespace</TableCell>
              <TableCell>File</TableCell>
              <TableCell align="right">Size</TableCell>
              <TableCell align="right">Last updated</TableCell>
            </TableRow>
          </TableHead>
          <TableBody>
            {loading && files.length === 0 ? (
              <TableRow>
                <TableCell colSpan={5} align="center" sx={{ py: 6 }}>
                  <CircularProgress size={28} aria-label="Loading data files" />
                </TableCell>
              </TableRow>
            ) : sortedFiles.length === 0 ? (
              <TableRow>
                <TableCell colSpan={5} align="center" sx={{ py: 6, color: 'text.secondary' }}>
                  No data files found.
                </TableCell>
              </TableRow>
            ) : sortedFiles.map((file) => (
              <TableRow key={`${file.namespace}/${file.name}`} hover>
                <TableCell>{file.kind}</TableCell>
                <TableCell>{file.namespace}</TableCell>
                <TableCell sx={{ fontFamily: 'monospace' }}>{file.name}</TableCell>
                <TableCell align="right">{formatBytes(file.sizeBytes)}</TableCell>
                <TableCell align="right">{formatDateTime(file.modifiedAtUnixMs)}</TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </TableContainer>
    </Box>
  );
}
