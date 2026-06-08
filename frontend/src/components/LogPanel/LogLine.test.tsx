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
});

// ---------------------------------------------------------------------------
// Pod badge (structured prefix format)
// ---------------------------------------------------------------------------

describe('LogLine — pod badge', () => {
  const STRUCTURED = '2024-01-15T10:00:00Z [default/web-app/web-app-abc123] INFO started';

  it('renders the pod name as a badge', () => {
    renderLine(STRUCTURED);
    // PREFIX_RE: [namespace/pod/container] — m[3] is the pod ('web-app'), not the container
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
});
