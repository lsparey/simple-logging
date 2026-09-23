import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import Box from '@mui/material/Box';
import TextField from '@mui/material/TextField';
import Autocomplete from '@mui/material/Autocomplete';
import Button from '@mui/material/Button';
import FormControl from '@mui/material/FormControl';
import InputLabel from '@mui/material/InputLabel';
import Select, { type SelectChangeEvent } from '@mui/material/Select';
import MenuItem from '@mui/material/MenuItem';
import FormControlLabel from '@mui/material/FormControlLabel';
import Checkbox from '@mui/material/Checkbox';
import CircularProgress from '@mui/material/CircularProgress';
import Typography from '@mui/material/Typography';
import Chip from '@mui/material/Chip';
import Alert from '@mui/material/Alert';
import { logClient } from '../../grpc/client.js';
import type { SearchLogsResponse } from '../../gen/simplelog/v1/log_service_pb.js';
import { useLogStore } from '../../store/logStore.js';
import { useNamespaces } from '../../hooks/useNamespaces.js';
import { useWorkloadsByNamespace } from '../../hooks/useWorkloadsByNamespace.js';
import { usePodsByNamespace } from '../../hooks/usePodsByNamespace.js';
import { WORKLOAD_KIND_SECTIONS } from '../PodSidebar/sidebarSections.js';
import { highlightMatches } from '../../utils/highlightMatches.js';
import { formatDateTime } from '../../utils/formatDateTime.js';

const LINE_RE = /^(\S+) \[[^\]]*\] ([\s\S]*)/;

function splitLine(line: string): { tsMs: number | null; message: string } {
  const m = LINE_RE.exec(line);
  if (!m) return { tsMs: null, message: line };
  const parsed = Date.parse(m[1]);
  return { tsMs: Number.isNaN(parsed) ? null : parsed, message: m[2] };
}

// How far around a search hit's timestamp to load when jumping to context,
// so the surrounding lines are visible without pulling in unrelated history.
const JUMP_CONTEXT_WINDOW_MS = 5 * 60 * 1000;

export default function SearchPage() {
  const navigate = useNavigate();
  const [namespace, setNamespace] = useState('');
  const [workloadKind, setWorkloadKind] = useState('');
  const [workloadName, setWorkloadName] = useState('');
  const [query, setQuery] = useState('');
  const [regex, setRegex] = useState(false);
  const [newestFirst, setNewestFirst] = useState(true);
  const [results, setResults] = useState<SearchLogsResponse[]>([]);
  const [searching, setSearching] = useState(false);
  const [truncated, setTruncated] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const abortRef = useRef<AbortController | null>(null);

  const { namespaces } = useNamespaces();
  const nameScope = namespace ? [namespace] : namespaces;
  // Kind "Pod" means "every pod" (see usePodsByNamespace), not the narrower
  // ListWorkloads kind "Pod" of genuinely unowned pods, so name suggestions
  // for it come from a different source than every other kind.
  const { workloadsByNamespace } = useWorkloadsByNamespace(
    workloadKind !== 'Pod' ? nameScope : [],
    workloadKind || undefined,
  );
  const { podsByNamespace } = usePodsByNamespace(workloadKind === 'Pod' ? nameScope : []);
  const nameOptions = useMemo(() => {
    const set = new Set<string>();
    for (const workloads of Object.values(workloadsByNamespace)) {
      for (const w of workloads) set.add(w.name);
    }
    for (const pods of Object.values(podsByNamespace)) {
      for (const p of pods) set.add(p.name);
    }
    return Array.from(set).sort();
  }, [workloadsByNamespace, podsByNamespace]);

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
          namespace: namespace.trim(),
          workloadKind: workloadKind.trim(),
          workloadName: workloadName.trim(),
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
  }, [namespace, workloadKind, workloadName, query, regex, newestFirst]);

  useEffect(() => () => abortRef.current?.abort(), []);

  // Selects a hit's own pod directly (kind "Pod" resolves any pod by name
  // regardless of its real owner), filtered to the hit's container and, if
  // the hit has a timestamp, with the time range narrowed to a window
  // around it so the surrounding context loads instead of just the latest
  // page.
  const jumpToContext = useCallback(async (target: { namespace: string; pod: string; container: string; tsMs: number | null }) => {
    let jsonLogging: boolean | undefined;
    try {
      const resp = await logClient.listPods({ namespace: target.namespace });
      jsonLogging = resp.pods.find((p) => p.name === target.pod)?.jsonLogging;
    } catch {
      // best-effort: default jsonLogging to false on failure
    }

    const store = useLogStore.getState();
    store.setSelectedWorkload(target.namespace, 'Pod', target.pod, jsonLogging);
    store.setSelectedContainerFilter(target.container);
    if (target.tsMs !== null) {
      store.setTimeRange(
        Math.floor((target.tsMs - JUMP_CONTEXT_WINDOW_MS) / 1000),
        Math.floor((target.tsMs + JUMP_CONTEXT_WINDOW_MS) / 1000),
      );
    }
    navigate(`/ns/${encodeURIComponent(target.namespace)}/Pod/${encodeURIComponent(target.pod)}`);
  }, [navigate]);

  return (
    <Box sx={{ flex: 1, display: 'flex', flexDirection: 'column', overflow: 'hidden' }}>
      <Box sx={{ px: 2, py: 1.5, borderBottom: 1, borderColor: 'divider' }}>
        <Typography variant="h6">Search</Typography>
      </Box>

      <Box
        sx={{
          display: 'flex',
          flexWrap: 'wrap',
          alignItems: 'center',
          gap: 1.5,
          px: 2,
          py: 1.5,
          borderBottom: 1,
          borderColor: 'divider',
        }}
      >
        <FormControl size="small" sx={{ width: 160 }}>
          <InputLabel shrink>Namespace</InputLabel>
          <Select
            label="Namespace"
            displayEmpty
            value={namespace}
            onChange={(e: SelectChangeEvent) => { setNamespace(e.target.value); setWorkloadName(''); }}
            inputProps={{ 'aria-label': 'Namespace' }}
          >
            <MenuItem value="">All</MenuItem>
            {namespaces.map((ns) => (
              <MenuItem key={ns} value={ns}>{ns}</MenuItem>
            ))}
          </Select>
        </FormControl>
        <FormControl size="small" sx={{ width: 160 }}>
          <InputLabel shrink>Workload kind</InputLabel>
          <Select
            label="Workload kind"
            displayEmpty
            value={workloadKind}
            onChange={(e: SelectChangeEvent) => { setWorkloadKind(e.target.value); setWorkloadName(''); }}
            inputProps={{ 'aria-label': 'Workload kind' }}
          >
            <MenuItem value="">All</MenuItem>
            {WORKLOAD_KIND_SECTIONS.map(({ key, label }) => (
              <MenuItem key={key} value={key}>{label}</MenuItem>
            ))}
          </Select>
        </FormControl>
        <Autocomplete
          freeSolo
          size="small"
          options={nameOptions}
          value={workloadName}
          onInputChange={(_, value) => setWorkloadName(value)}
          renderInput={(params) => <TextField {...params} label="Workload name" />}
          sx={{ width: 200 }}
        />
        <TextField
          size="small"
          placeholder="Query…"
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
            {query ? 'No matches yet.' : 'Enter a query and press Search. Leave namespace/kind/name blank to search everywhere.'}
          </Typography>
        )}
        {results.map((r, i) => {
          const { tsMs, message } = splitLine(r.line);
          return (
            <Box
              key={i}
              onClick={() => jumpToContext({ namespace: r.namespace, pod: r.pod, container: r.container, tsMs })}
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
              <Chip label={`${r.namespace}/${r.pod}/${r.container}`} size="small" variant="outlined" sx={{ height: 18, fontSize: '0.6875rem', flexShrink: 0 }} />
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
