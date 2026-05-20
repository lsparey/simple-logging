import { useMemo } from 'react';
import Box from '@mui/material/Box';
import Typography from '@mui/material/Typography';

const LEVEL_RE = /\b(TRACE|DEBUG|INFO|WARN(?:ING)?|ERROR|FATAL|CRITICAL)\b/i;

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

interface Props {
  line: string;
  darkMode: boolean;
}

export default function LogLine({ line, darkMode }: Props) {
  const { colour, parts } = useMemo(() => {
    const match = LEVEL_RE.exec(line);
    if (!match) return { colour: 'inherit', parts: null };

    const palette = darkMode ? DARK_COLOURS : LIGHT_COLOURS;
    const level = match[0].toUpperCase();
    const colour = palette[level] ?? 'inherit';

    const pre = line.slice(0, match.index);
    const keyword = match[0];
    const post = line.slice(match.index + keyword.length);
    return { colour, parts: { pre, keyword, post } };
  }, [line, darkMode]);

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
        whiteSpace: 'pre-wrap',
        wordBreak: 'break-all',
        color: colour,
        '&:hover': { bgcolor: 'action.hover' },
      }}
    >
      {parts ? (
        <>
          {parts.pre}
          <Typography
            component="span"
            sx={{ fontWeight: 700, fontSize: 'inherit', fontFamily: 'inherit' }}
          >
            {parts.keyword}
          </Typography>
          {parts.post}
        </>
      ) : (
        line
      )}
    </Box>
  );
}
