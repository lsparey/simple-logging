import { render, screen } from '@testing-library/react';
import { describe, it, expect } from 'vitest';
import LogLine from './LogLine.js';
import type { JsonFormat } from '../../store/logStore.js';

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function renderLine(line: string, darkMode = false, jsonFormat?: JsonFormat) {
  return render(<LogLine line={line} darkMode={darkMode} jsonFormat={jsonFormat} />);
}

function expectColor(element: Element | null, color: string) {
  expect(element).not.toBeNull();
  if (color === 'inherit') {
    expect((element as HTMLElement).style.color).toBe('inherit');
    return;
  }
  expect(element).toHaveStyle({ color });
}

// ---------------------------------------------------------------------------
// Plain log lines
// ---------------------------------------------------------------------------

describe('LogLine — plain lines', () => {
  it('renders the full log line text', () => {
    renderLine('2024-01-15T10:00:00Z INFO hello world');
    expect(screen.getByText(/hello world/)).toBeInTheDocument();
  });

  it('renders inside a <pre> element', () => {
    const { container } = renderLine('2024-01-15T10:00:00Z INFO test');
    expect(container.querySelector('pre')).toBeInTheDocument();
  });
});

// ---------------------------------------------------------------------------
// Level colouring
// ---------------------------------------------------------------------------

describe('LogLine — level colours', () => {
  const LEVELS = ['TRACE', 'DEBUG', 'INFO', 'WARN', 'WARNING', 'ERROR', 'FATAL', 'CRITICAL'] as const;

  it.each(LEVELS)('renders %s lines without crashing', (level) => {
    renderLine(`2024-01-15T10:00:00Z ${level} some message`);
    expect(screen.getByText(/some message/)).toBeInTheDocument();
  });

  it('applies dark ERROR colour', () => {
    const { container } = renderLine('2024-01-15T10:00:00Z ERROR boom', true);
    const pre = container.querySelector('pre') as HTMLElement;
    // MUI sx props apply colour via CSS class (emotion); check computed style
    expect(window.getComputedStyle(pre).color).toBe('rgb(248, 81, 73)'); // #f85149
  });

  it('applies light WARN colour', () => {
    const { container } = renderLine('2024-01-15T10:00:00Z WARN careful', false);
    const pre = container.querySelector('pre') as HTMLElement;
    expect(window.getComputedStyle(pre).color).toBe('rgb(154, 103, 0)'); // #9a6700
  });

  it('uses the configured info, trace, and debug colours', () => {
    const { container: infoContainer } = renderLine('2024-01-15T10:00:00Z INFO ready', true);
    const { container: traceContainer } = renderLine('2024-01-15T10:00:00Z TRACE request', true);
    const { container: debugContainer } = renderLine('2024-01-15T10:00:00Z DEBUG detail', true);

    expect(window.getComputedStyle(infoContainer.querySelector('pre') as HTMLElement).color)
      .toBe('rgb(63, 185, 80)'); // #3fb950
    expect(window.getComputedStyle(traceContainer.querySelector('pre') as HTMLElement).color)
      .toBe('rgb(88, 166, 255)'); // #58a6ff
    expect(window.getComputedStyle(debugContainer.querySelector('pre') as HTMLElement).color)
      .toBe('rgb(188, 140, 255)'); // #bc8cff
  });
});

// ---------------------------------------------------------------------------
// Pod badge (structured prefix format)
// ---------------------------------------------------------------------------

describe('LogLine — pod badge', () => {
  const STRUCTURED = '2024-01-15T10:00:00Z [default/web-app/web-app-abc123] INFO started';

  it('renders the full pod name as a badge by default (desktop list view)', () => {
    renderLine(STRUCTURED);
    // PREFIX_RE: [namespace/pod/container] — m[3] is the pod ('web-app'), not the container
    expect(screen.getByText('web-app')).toBeInTheDocument();
  });

  it('collapses to a titled colour dot when compact is set (narrow/mobile viewports)', () => {
    const { container } = render(<LogLine line={STRUCTURED} darkMode={false} compact />);
    expect(screen.queryByText('web-app')).not.toBeInTheDocument();
    const dot = container.querySelector('[title="web-app"]');
    expect(dot).toBeInTheDocument();
    expect(dot).toHaveStyle({ borderRadius: '50%' });
  });

  it('renders the full pod name as a badge in wrap mode even when compact', () => {
    render(<LogLine line={STRUCTURED} darkMode={false} wrap compact />);
    expect(screen.getByText('web-app')).toBeInTheDocument();
  });

  it('renders the message text after stripping the prefix', () => {
    renderLine(STRUCTURED);
    expect(screen.getByText(/INFO started/)).toBeInTheDocument();
  });

  it('does not render a badge for plain lines', () => {
    const { container } = renderLine('2024-01-15T10:00:00Z INFO plain line');
    // Badge is a <span> inside the <pre>; there should be none for plain lines
    const spans = container.querySelectorAll('pre span');
    expect(spans).toHaveLength(0);
  });
});

// ---------------------------------------------------------------------------
// ANSI escape codes
// ---------------------------------------------------------------------------

describe('LogLine — ANSI sequences', () => {
  const ESC = '\u001b';

  it('renders ANSI-coloured text as multiple spans', () => {
    const { container } = renderLine(`${ESC}[31mred text${ESC}[0m normal`);
    const spans = container.querySelectorAll('pre span');
    expect(spans.length).toBeGreaterThanOrEqual(2);
  });

  it('strips ANSI codes so raw escape sequences are not shown', () => {
    renderLine(`${ESC}[32mgreen${ESC}[0m`);
    // No raw ESC character should appear in the document text
    expect(document.body.textContent).not.toContain('\u001b[');
  });

  it('applies bold styling for SGR 1', () => {
    const { container } = renderLine(`${ESC}[1mbold text${ESC}[0m`);
    const boldSpan = Array.from(container.querySelectorAll('pre span')).find(
      (el) => (el as HTMLElement).style.fontWeight === 'bold',
    );
    expect(boldSpan).toBeDefined();
  });
});

// ---------------------------------------------------------------------------
// JSON formatting
// ---------------------------------------------------------------------------

describe('LogLine — JSON formatting', () => {
  const FORMAT: JsonFormat = {
    timestampKey: 'timestamp',
    levelKey: 'level',
    messageKey: 'message',
  };
  const LINE = '2024-01-15T10:00:00Z [default/api/app] {"timestamp":"2024-01-15T10:00:00Z","level":"info","message":"request complete"}';

  it.each([
    '{"level":"error","message":"raw string severity"}',
    '{"level":50,"message":"raw numeric severity"}',
  ])('keeps unformatted JSON on the normal foreground: %s', (payload) => {
    const line = `2024-01-15T10:00:00Z [default/api/app] ${payload}`;
    const { container } = renderLine(line, true);

    expect(window.getComputedStyle(container.querySelector('pre') as HTMLElement).color)
      .toBe('rgb(255, 255, 255)');
  });

  it('keeps unformatted JSON readable in light mode', () => {
    const line = '2024-01-15T10:00:00Z [default/api/app] {"level":"error","message":"raw"}';
    const { container } = renderLine(line, false);

    expect(window.getComputedStyle(container.querySelector('pre') as HTMLElement).color)
      .toBe('rgb(31, 35, 40)');
  });

  it('renders a muted timestamp and white message in dark mode', () => {
    const { container } = renderLine(LINE, true, FORMAT);

    expect(container.querySelector('[data-json-field="timestamp"]')).toHaveStyle({ color: '#8c959f' });
    expect(container.querySelector('[data-json-field="message"]')).toHaveStyle({ color: '#ffffff' });
  });

  it('uses readable foreground colours in light mode', () => {
    const { container } = renderLine(LINE, false, FORMAT);

    expect(container.querySelector('[data-json-field="timestamp"]')).toHaveStyle({ color: '#57606a' });
    expect(container.querySelector('[data-json-field="message"]')).toHaveStyle({ color: '#1f2328' });
  });

  it.each([
    [10, '#58a6ff'],
    [20, '#bc8cff'],
    [30, '#3fb950'],
    [40, '#d29922'],
    [50, '#f85149'],
    [60, '#ffffff'],
  ])('maps Pino level %i while keeping the numeric label', (level, color) => {
    const line = `2024-01-15T10:00:00Z [default/api/app] {"level":${level},"message":"pino"}`;
    const { container } = renderLine(line, true, { levelKey: 'level', messageKey: 'message' });
    const levelElement = container.querySelector('[data-json-field="level"]');

    expect(levelElement).toHaveTextContent(String(level));
    expectColor(levelElement, color);
  });

  it.each([
    [0, '#f85149'],
    [1, '#d29922'],
    [2, '#3fb950'],
    [5, '#bc8cff'],
    [6, '#bc8cff'],
  ])('maps Winston npm level %i', (level, color) => {
    const line = `2024-01-15T10:00:00Z [default/api/app] {"level":${level},"message":"winston"}`;
    const { container } = renderLine(line, true, { levelKey: 'level', messageKey: 'message' });

    expectColor(container.querySelector('[data-json-field="level"]'), color);
  });

  it.each([
    [2, '#f85149'],
    [4, '#d29922'],
    [6, '#3fb950'],
    [7, '#bc8cff'],
  ])('maps syslog severity %i when the key identifies severity', (level, color) => {
    const line = `2024-01-15T10:00:00Z [default/api/app] {"severity":${level},"message":"syslog"}`;
    const { container } = renderLine(line, true, { levelKey: 'severity', messageKey: 'message' });

    expectColor(container.querySelector('[data-json-field="level"]'), color);
  });

  it('renders fatal severity as white text on red like pino-pretty', () => {
    const line = '2024-01-15T10:00:00Z [default/api/app] {"level":60,"message":"fatal"}';
    const { container } = renderLine(line, true, { levelKey: 'level', messageKey: 'message' });
    const levelElement = container.querySelector('[data-json-field="level"]');

    expect(levelElement).toHaveStyle({
      color: '#ffffff',
      backgroundColor: '#b62324',
    });
  });
});

// ---------------------------------------------------------------------------
// wrap mode — pretty-printed, syntax-highlighted JSON
// ---------------------------------------------------------------------------

describe('LogLine — wrap mode', () => {
  it('does not pretty-print JSON when wrap is false (list rendering unaffected)', () => {
    const line = '2024-01-15T10:00:00Z [default/api/app] {"level":"info","message":"hi"}';
    const { container } = render(<LogLine line={line} darkMode={false} />);
    expect(container.querySelector('pre')?.textContent).toBe('api{"level":"info","message":"hi"}');
  });

  it('collapses to a colour dot (no text) when compact and not wrapped', () => {
    const line = '2024-01-15T10:00:00Z [default/api/app] {"level":"info","message":"hi"}';
    const { container } = render(<LogLine line={line} darkMode={false} compact />);
    expect(container.querySelector('pre')?.textContent).toBe('{"level":"info","message":"hi"}');
  });

  it('pretty-prints unformatted JSON when wrap is true', () => {
    const line = '{"level":"info","message":"hi"}';
    const { container } = render(<LogLine line={line} darkMode={false} wrap />);
    const pre = container.querySelector('pre');
    expect(pre?.textContent).toContain('\n');
    expect(pre?.textContent).toContain('"level": "info"');
  });

  it('colours JSON keys, strings, numbers, and booleans distinctly when wrapped', () => {
    const line = '{"name":"svc","count":3,"active":true}';
    const { container } = render(<LogLine line={line} darkMode={true} wrap />);
    const spans = Array.from(container.querySelectorAll('pre span'));

    const keySpan = spans.find((el) => el.textContent === '"name":');
    const stringSpan = spans.find((el) => el.textContent === '"svc"');
    const numberSpan = spans.find((el) => el.textContent === '3');
    const boolSpan = spans.find((el) => el.textContent === 'true');

    expect(keySpan).toHaveStyle({ color: '#79c0ff' });
    expect(stringSpan).toHaveStyle({ color: '#7ee787' });
    expect(numberSpan).toHaveStyle({ color: '#d2a8ff' });
    expect(boolSpan).toHaveStyle({ color: '#ffa657' });
  });

  it('pretty-prints the raw JSON of a configured jsonFormat message when wrapped', () => {
    const line = '{"timestamp":"2024-01-15T10:00:00Z","level":"info","message":"done","extra":"field"}';
    const { container } = render(
      <LogLine
        line={line}
        darkMode={false}
        wrap
        jsonFormat={{ timestampKey: 'timestamp', levelKey: 'level', messageKey: 'message' }}
      />,
    );
    const pre = container.querySelector('pre');
    // Extracted fields still shown, plus the full pretty-printed object (including the extra key).
    expect(pre?.textContent).toContain('done');
    expect(pre?.textContent).toContain('"extra": "field"');
  });

  it('wraps long lines instead of clipping them', () => {
    const { container } = render(<LogLine line="plain long message" darkMode={false} wrap />);
    const pre = container.querySelector('pre') as HTMLElement;
    expect(window.getComputedStyle(pre).whiteSpace).toBe('pre-wrap');
  });
});
