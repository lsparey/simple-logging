import { useMemo } from 'react';
import Box from '@mui/material/Box';
import type { JsonFormat } from '../../store/logStore.js';

// Matches: "TIMESTAMP [namespace/pod/container] message"
const PREFIX_RE = /^(\S+) \[([^/\]]+)\/([^/\]]+)\/[^\]]+\] ([\s\S]*)/;

// Deterministic colour palette for pod name badges.
const POD_BADGE_COLOURS = [
  '#1565c0', '#2e7d32', '#6a1b9a', '#c62828', '#4e342e',
  '#00695c', '#0277bd', '#558b2f', '#ad1457', '#e65100',
  '#283593', '#37474f', '#4a148c', '#880e4f', '#bf360c',
  '#00838f', '#f57f17', '#4527a0', '#0d47a1', '#1b5e20',
];

function podBadgeColour(podName: string): string {
  let h = 0;
  for (let i = 0; i < podName.length; i++) {
    h = (h * 31 + podName.charCodeAt(i)) >>> 0;
  }
  return POD_BADGE_COLOURS[h % POD_BADGE_COLOURS.length];
}

interface ParsedPrefix {
  podName: string;
  message: string;
}

function parsePrefix(line: string): ParsedPrefix | null {
  const m = PREFIX_RE.exec(line);
  if (!m) return null;
  return { podName: m[3], message: m[4] };
}

const ESC = String.fromCharCode(27);
const ANSI_ESCAPE_RE = new RegExp(ESC + '\\[[0-9;]*m', 'g');
const LEVEL_RE = /\b(TRACE|DEBUG|INFO|WARN(?:ING)?|ERROR|FATAL|CRITICAL)\b/i;

// Standard 16-colour ANSI palette (dark then bright)
const ANSI_PALETTE = [
  '#000000', '#cc0000', '#00aa00', '#aa5500',
  '#0000cc', '#aa00aa', '#00aaaa', '#aaaaaa',
  '#555555', '#ff5555', '#55ff55', '#ffff55',
  '#5555ff', '#ff55ff', '#55ffff', '#ffffff',
];

interface Segment {
  text: string;
  color?: string;
  bgColor?: string;
  bold?: boolean;
  dim?: boolean;
  italic?: boolean;
  underline?: boolean;
}

function parseAnsi(input: string): Segment[] {
  const segments: Segment[] = [];
  const re = new RegExp(ESC + '\\[([0-9;]*)m', 'g');
  let state: Omit<Segment, 'text'> = {};
  let lastIndex = 0;
  let match: RegExpExecArray | null;

  while ((match = re.exec(input)) !== null) {
    if (match.index > lastIndex) {
      segments.push({ text: input.slice(lastIndex, match.index), ...state });
    }
    lastIndex = re.lastIndex;

    const codes = match[1] === '' ? [0] : match[1].split(';').map(Number);
    let i = 0;
    while (i < codes.length) {
      const c = codes[i];
      if (c === 0) {
        state = {};
      } else if (c === 1) {
        state = { ...state, bold: true };
      } else if (c === 2) {
        state = { ...state, dim: true };
      } else if (c === 3) {
        state = { ...state, italic: true };
      } else if (c === 4) {
        state = { ...state, underline: true };
      } else if (c >= 30 && c <= 37) {
        state = { ...state, color: ANSI_PALETTE[c - 30] };
      } else if (c === 39) {
        state = { bold: state.bold, dim: state.dim, italic: state.italic, underline: state.underline, bgColor: state.bgColor };
      } else if (c >= 40 && c <= 47) {
        state = { ...state, bgColor: ANSI_PALETTE[c - 40] };
      } else if (c === 49) {
        state = { bold: state.bold, dim: state.dim, italic: state.italic, underline: state.underline, color: state.color };
      } else if (c >= 90 && c <= 97) {
        state = { ...state, color: ANSI_PALETTE[c - 82] };
      } else if (c >= 100 && c <= 107) {
        state = { ...state, bgColor: ANSI_PALETTE[c - 92] };
      } else if ((c === 38 || c === 48) && codes[i + 1] === 2 && i + 4 < codes.length) {
        const rgb = `rgb(${codes[i + 2]},${codes[i + 3]},${codes[i + 4]})`;
        state = c === 38 ? { ...state, color: rgb } : { ...state, bgColor: rgb };
        i += 4;
      } else if ((c === 38 || c === 48) && codes[i + 1] === 5 && i + 2 < codes.length) {
        const n = codes[i + 2];
        if (n < 16) {
          state = c === 38 ? { ...state, color: ANSI_PALETTE[n] } : { ...state, bgColor: ANSI_PALETTE[n] };
        }
        i += 2;
      }
      i++;
    }
  }

  if (lastIndex < input.length) {
    segments.push({ text: input.slice(lastIndex), ...state });
  }
  return segments;
}

const HAS_ANSI_RE = new RegExp(ESC + '\\[');

const DARK_COLOURS: Record<string, string> = {
  TRACE: '#6e7681',
  DEBUG: '#6e7681',
  INFO: 'inherit',
  WARN: '#d29922',
  WARNING: '#d29922',
  ERROR: '#f85149',
  FATAL: '#f85149',
  CRITICAL: '#f85149',
};

const LIGHT_COLOURS: Record<string, string> = {
  TRACE: '#57606a',
  DEBUG: '#57606a',
  INFO: 'inherit',
  WARN: '#9a6700',
  WARNING: '#9a6700',
  ERROR: '#cf222e',
  FATAL: '#cf222e',
  CRITICAL: '#cf222e',
};

const JSON_MESSAGE_DARK = '#4dd0e1';
const JSON_MESSAGE_LIGHT = '#00695c';
const JSON_DIM_DARK = '#484f58';
const JSON_DIM_LIGHT = '#8c959f';

/** Format an ISO string or Unix timestamp (seconds or ms) to a short local datetime. */
function formatJsonTimestamp(value: unknown): string {
  if (value === undefined || value === null || value === '') return '';
  let d: Date | null = null;
  if (typeof value === 'number') {
    // Heuristic: Unix seconds < 1e10, millis >= 1e10
    d = new Date(value < 1e10 ? value * 1000 : value);
  } else if (typeof value === 'string') {
    d = new Date(value);
  }
  if (!d || isNaN(d.getTime())) return String(value);
  return d.toLocaleString(undefined, {
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
    hour12: false,
  });
}

interface Props {
  line: string;
  darkMode: boolean;
  jsonFormat?: JsonFormat | null;
}

export default function LogLine({ line, darkMode, jsonFormat }: Props) {
  const { colour, prefix, message, segments, jsonParsed } = useMemo(() => {
    const parsed = parsePrefix(line);
    const displayMessage = parsed ? parsed.message : line;
    const stripped = displayMessage.replace(ANSI_ESCAPE_RE, '');
    const palette = darkMode ? DARK_COLOURS : LIGHT_COLOURS;

    // Try JSON parsing when a format is configured
    let jsonParsed: { ts: string; level: string; levelColour: string; msg: string; raw: string } | null = null;
    if (jsonFormat) {
      try {
        const obj = JSON.parse(stripped) as Record<string, unknown>;
        if (obj !== null && typeof obj === 'object') {
          const level = jsonFormat.levelKey ? String(obj[jsonFormat.levelKey] ?? '').toUpperCase() : '';
          const msg = jsonFormat.messageKey ? String(obj[jsonFormat.messageKey] ?? '') : '';
          const levelColour = level ? (palette[level] ?? 'inherit') : 'inherit';
          const ts = jsonFormat.timestampKey ? formatJsonTimestamp(obj[jsonFormat.timestampKey]) : '';
          jsonParsed = { ts, level, levelColour, msg, raw: stripped };
        }
      } catch { /* not JSON */ }
    }

    const match = jsonParsed ? null : LEVEL_RE.exec(stripped);
    const colour = match ? (palette[match[0].toUpperCase()] ?? 'inherit') : 'inherit';
    const segments = !jsonParsed && HAS_ANSI_RE.test(displayMessage) ? parseAnsi(displayMessage) : null;
    return {
      colour,
      prefix: parsed ? { podName: parsed.podName } : null,
      message: displayMessage,
      segments,
      jsonParsed,
    };
  }, [line, darkMode, jsonFormat]);

  return (
    <Box
      component="pre"
      sx={{
        m: 0,
        px: 1,
        py: 0.125,
        fontFamily: 'inherit',
        fontSize: '0.75rem',
        lineHeight: 1.6,
        whiteSpace: 'pre',
        overflow: 'hidden',
        color: colour,
        '&:hover': { bgcolor: 'action.hover' },
      }}
    >
      {prefix && (
        <Box
          component="span"
          sx={{
            display: 'inline-block',
            bgcolor: podBadgeColour(prefix.podName),
            color: '#fff',
            borderRadius: '4px',
            px: 0.75,
            py: 0,
            mr: 0.75,
            fontSize: '0.7rem',
            fontWeight: 600,
            lineHeight: 1.5,
            verticalAlign: 'middle',
            userSelect: 'none',
          }}
        >
          {prefix.podName}
        </Box>
      )}
      {jsonParsed ? (
        <>
          {jsonParsed.ts && (
            <span>
              {jsonParsed.ts}{' '}
            </span>
          )}
          {jsonParsed.level && (
            <span style={{ color: jsonParsed.levelColour, fontWeight: 600 }}>
              {jsonParsed.level}{' '}
            </span>
          )}
          {jsonParsed.msg && (
            <span style={{ color: darkMode ? JSON_MESSAGE_DARK : JSON_MESSAGE_LIGHT }}>
              {jsonParsed.msg}{' '}
            </span>
          )}
          <span style={jsonParsed.ts || jsonParsed.level || jsonParsed.msg ? { color: darkMode ? JSON_DIM_DARK : JSON_DIM_LIGHT } : undefined}>
            {jsonParsed.raw}
          </span>
        </>
      ) : segments
        ? segments.map((seg, i) => (
            <span
              key={i}
              style={{
                ...(seg.color && { color: seg.color }),
                ...(seg.bgColor && { backgroundColor: seg.bgColor }),
                ...(seg.bold && { fontWeight: 'bold' }),
                ...(seg.dim && { opacity: 0.5 }),
                ...(seg.italic && { fontStyle: 'italic' }),
                ...(seg.underline && { textDecoration: 'underline' }),
              }}
            >
              {seg.text}
            </span>
          ))
        : message}
    </Box>
  );
}
