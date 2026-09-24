import Alert from '@mui/material/Alert';
import Box from '@mui/material/Box';
import Button from '@mui/material/Button';
import CircularProgress from '@mui/material/CircularProgress';
import LinearProgress from '@mui/material/LinearProgress';
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
import { useStats } from '../../hooks/useStats.js';
import { formatBytes } from '../../utils/formatBytes.js';
import { formatDateTime } from '../../utils/formatDateTime.js';
import type { GetStatsResponse } from '../../gen/simplelog/v1/log_service_pb.js';

// Below the low water mark there's plenty of headroom (success); between the
// low and high water marks the disk guard hasn't kicked in yet but is close
// (warning); at or above the high water mark it's actively deleting the
// oldest segments (error). Mirrors the semantics of config.diskLowWaterPercent
// / config.diskHighWaterPercent.
function diskUsageColor(usedPercent: number, highWaterPercent: number, lowWaterPercent: number) {
  if (usedPercent >= highWaterPercent) return 'error';
  if (usedPercent >= lowWaterPercent) return 'warning';
  return 'success';
}

export default function DataDashboard() {
  const { stats, refresh: refreshStats } = useStats();
  const {
    files,
    totalSizeBytes,
    totalLogFileCount,
    totalIndexFileCount,
    diskUsedPercent,
    diskHighWaterPercent,
    diskLowWaterPercent,
    loading,
    error,
    refresh: refreshFiles,
  } = useLogFiles();
  const refresh = () => {
    refreshFiles();
    refreshStats();
  };
  const totalFileCount = totalLogFileCount + totalIndexFileCount;
  const diskColor = diskUsageColor(diskUsedPercent, diskHighWaterPercent, diskLowWaterPercent);

  return (
    <Box sx={{ p: 3, overflow: 'auto', height: '100%' }}>
      <Box sx={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', mb: 3 }}>
        <Box>
          <Typography variant="h4" component="h1">Storage dashboard</Typography>
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
          <Typography variant="body2" color="text.secondary">Log files</Typography>
          <Typography variant="h4" sx={{ fontFamily: 'monospace', mt: 2 }}>
            {totalLogFileCount}
          </Typography>
        </Paper>
        <Paper variant="outlined" sx={{ p: 2.5 }}>
          <Typography variant="body2" color="text.secondary">Index files</Typography>
          <Typography variant="h4" sx={{ fontFamily: 'monospace', mt: 2 }}>
            {totalIndexFileCount}
          </Typography>
        </Paper>
        <Paper variant="outlined" sx={{ p: 2.5 }}>
          <Typography variant="body2" color="text.secondary">Disk usage</Typography>
          <Typography variant="h4" sx={{ fontFamily: 'monospace', mt: 2, color: `${diskColor}.main` }}>
            {diskUsedPercent}%
          </Typography>
          <LinearProgress
            variant="determinate"
            value={Math.min(diskUsedPercent, 100)}
            color={diskColor}
            sx={{ mt: 1.5, mb: 1, height: 6, borderRadius: 3 }}
            aria-label="Disk usage"
          />
          <Typography variant="caption" color="text.secondary">
            Disk guard deletes oldest segments at {diskHighWaterPercent}%, down to {diskLowWaterPercent}%
          </Typography>
        </Paper>
      </Box>

      {stats && <CollectionStats stats={stats} />}

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
              <TableCell>Subject</TableCell>
              <TableCell>File(s)</TableCell>
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
            ) : files.length === 0 ? (
              <TableRow>
                <TableCell colSpan={5} align="center" sx={{ py: 6, color: 'text.secondary' }}>
                  No data files found.
                </TableCell>
              </TableRow>
            ) : files.map((file) => (
              <TableRow key={`${file.namespace}/${file.subject}/${file.name}`} hover>
                <TableCell>{file.kind}</TableCell>
                <TableCell>{file.subject}</TableCell>
                <TableCell sx={{ fontFamily: 'monospace' }}>{file.name}</TableCell>
                <TableCell align="right">{formatBytes(file.sizeBytes)}</TableCell>
                <TableCell align="right">{formatDateTime(file.modifiedAtUnixMs)}</TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </TableContainer>
      {totalFileCount > files.length && (
        <Typography variant="caption" color="text.secondary" sx={{ display: 'block', mt: 1 }}>
          Showing {files.length} summary rows for {totalFileCount.toLocaleString()} persisted files.
          The list is limited to the largest 50 summaries.
        </Typography>
      )}
    </Box>
  );
}

function StatTile({ label, value, caption }: { label: string; value: string; caption?: string }) {
  return (
    <Paper variant="outlined" sx={{ p: 2.5 }}>
      <Typography variant="body2" color="text.secondary">{label}</Typography>
      <Typography variant="h4" sx={{ fontFamily: 'monospace', mt: 2 }}>{value}</Typography>
      {caption && <Typography variant="caption" color="text.secondary">{caption}</Typography>}
    </Paper>
  );
}

/** Collector counters from GetStats, which reset whenever the server restarts. */
function CollectionStats({ stats }: { stats: GetStatsResponse }) {
  const since = stats.startedAtUnixMs > 0n ? formatDateTime(stats.startedAtUnixMs) : null;
  const streams = stats.streamsActiveFile + stats.streamsActiveApi;
  return (
    <Box sx={{ mb: 3 }}>
      <Typography variant="h6" component="h2">Collection</Typography>
      <Typography variant="body2" color="text.secondary" sx={{ mb: 1.5 }}>
        {since ? `Since the server started at ${since}.` : 'Since the server started.'}
      </Typography>
      {stats.linesDroppedTotal > 0n && (
        <Alert severity="warning" sx={{ mb: 2 }}>
          {stats.linesDroppedTotal.toLocaleString()} log lines were dropped because writes to storage failed.
          Check that the log volume isn't full or read-only.
        </Alert>
      )}
      <Box sx={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(220px, 1fr))', gap: 2 }}>
        <StatTile
          label="Active streams"
          value={streams.toLocaleString()}
          caption={`${stats.streamsActiveFile.toLocaleString()} from node files, ${stats.streamsActiveApi.toLocaleString()} from the API`}
        />
        <StatTile
          label="Lines written"
          value={stats.linesWrittenTotal.toLocaleString()}
          caption={formatBytes(stats.bytesWrittenTotal)}
        />
        <StatTile label="Lines dropped" value={stats.linesDroppedTotal.toLocaleString()} />
        <StatTile label="API reconnects" value={stats.apiReconnectsTotal.toLocaleString()} />
      </Box>
    </Box>
  );
}
