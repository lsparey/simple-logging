import { useCallback, useEffect, useRef, useState } from 'react';
import Box from '@mui/material/Box';
import TextField from '@mui/material/TextField';
import Button from '@mui/material/Button';
import FormControlLabel from '@mui/material/FormControlLabel';
import Checkbox from '@mui/material/Checkbox';
import CircularProgress from '@mui/material/CircularProgress';
import Typography from '@mui/material/Typography';
import Chip from '@mui/material/Chip';
import Alert from '@mui/material/Alert';
import { logClient } from '../../grpc/client.js';
import type { SearchLogsResponse } from '../../gen/simplelog/v1/log_service_pb.js';
import { highlightMatches } from '../../utils/highlightMatches.js';
import { formatDateTime } from '../../utils/formatDateTime.js';

const LINE_RE = /^(\S+) \[[^\]]*\] ([\s\S]*)/;

function splitLine(line: string): { tsMs: number | null; message: string } {
  const m = LINE_RE.exec(line);
  if (!m) return { tsMs: null, message: line };
  const parsed = Date.parse(m[1]);
  return { tsMs: Number.isNaN(parsed) ? null : parsed, message: m[2] };
}

export interface JumpTarget {
  namespace: string;
  pod: string;
  container: string;
  tsMs: number | null;
}

interface Props {
  namespace: string;
  kind: string;
  name: string;
  onJumpToContext: (target: JumpTarget) => void;
}

export default function SearchPanel({ namespace, kind, name, onJumpToContext }: Props) {
  const [query, setQuery] = useState('');
  const [regex, setRegex] = useState(false);
  const [everywhere, setEverywhere] = useState(false);
  const [newestFirst, setNewestFirst] = useState(true);
  const [results, setResults] = useState<SearchLogsResponse[]>([]);
  const [searching, setSearching] = useState(false);
  const [truncated, setTruncated] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const abortRef = useRef<AbortController | null>(null);

  const runSearch = useCallback(async () => {
    if (!query) return;
    abortRef.current?.abort();
    const controller = new AbortController();
    abortRef.current = controller;

    setResults([]);
    setTruncated(false);
    setError(null);
    setSearching(true);
    try {
      const stream = logClient.searchLogs(
        {
          namespace: everywhere ? '' : namespace,
          workloadKind: everywhere ? '' : kind,
          workloadName: everywhere ? '' : name,
          query,
          regex,
          newestFirst,
        },
        { signal: controller.signal },
      );
      for await (const msg of stream) {
        if (msg.truncated) {
          setTruncated(true);
          continue;
        }
        setResults((prev) => [...prev, msg]);
      }
    } catch (e) {
      if (!controller.signal.aborted) setError(String(e));
    } finally {
      if (!controller.signal.aborted) setSearching(false);
    }
  }, [namespace, kind, name, query, regex, everywhere, newestFirst]);

  useEffect(() => () => abortRef.current?.abort(), []);

  return (
    <Box sx={{ display: 'flex', flexDirection: 'column', flex: 1, overflow: 'hidden' }}>
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
        <TextField
          size="small"
          placeholder="Search server-side…"
          value={query}
          onChange={(e) => setQuery(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === 'Enter') runSearch();
          }}
          sx={{ flex: 1, minWidth: 200 }}
        />
        <FormControlLabel
          control={<Checkbox size="small" checked={regex} onChange={(e) => setRegex(e.target.checked)} />}
          label="Regex"
        />
        <FormControlLabel
          control={<Checkbox size="small" checked={everywhere} onChange={(e) => setEverywhere(e.target.checked)} />}
          label="Everywhere"
        />
        <FormControlLabel
          control={<Checkbox size="small" checked={newestFirst} onChange={(e) => setNewestFirst(e.target.checked)} />}
          label="Newest first"
        />
        <Button variant="contained" size="small" disabled={!query || searching} onClick={runSearch}>
          Search
        </Button>
        {searching && <CircularProgress size={18} />}
      </Box>

      <Box sx={{ overflow: 'auto', flex: 1, px: 2, py: 1 }}>
        {error && <Alert severity="error" sx={{ mb: 1 }}>{error}</Alert>}
        {!searching && !error && results.length === 0 && (
          <Typography variant="body2" color="text.disabled">
            {query ? 'No matches yet.' : 'Enter a query and press Search.'}
          </Typography>
        )}
        {results.map((r, i) => {
          const { tsMs, message } = splitLine(r.line);
          return (
            <Box
              key={i}
              onClick={() => onJumpToContext({ namespace: r.namespace, pod: r.pod, container: r.container, tsMs })}
              sx={{
                display: 'flex',
                gap: 1,
                alignItems: 'flex-start',
                py: 0.5,
                fontFamily: 'monospace',
                fontSize: '0.8125rem',
                cursor: 'pointer',
                borderBottom: 1,
                borderColor: 'divider',
                '&:hover': { bgcolor: 'action.hover' },
              }}
            >
              <Typography variant="caption" color="text.disabled" sx={{ whiteSpace: 'nowrap', flexShrink: 0 }}>
                {tsMs !== null ? formatDateTime(BigInt(tsMs)) : ''}
              </Typography>
              <Chip label={`${r.pod}/${r.container}`} size="small" variant="outlined" sx={{ height: 18, fontSize: '0.6875rem', flexShrink: 0 }} />
              <Typography component="span" variant="body2" sx={{ fontFamily: 'monospace', fontSize: '0.8125rem', wordBreak: 'break-word' }}>
                {highlightMatches(message, query, regex)}
              </Typography>
            </Box>
          );
        })}
        {truncated && (
          <Alert severity="info" sx={{ mt: 1 }}>
            Results truncated — narrow the query or time range to see more.
          </Alert>
        )}
      </Box>
    </Box>
  );
}
