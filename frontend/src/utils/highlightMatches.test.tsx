import { render } from '@testing-library/react';
import { describe, expect, it } from 'vitest';
import { highlightMatches } from './highlightMatches.js';

describe('highlightMatches', () => {
  it('returns the text unchanged when query is empty', () => {
    expect(highlightMatches('hello world', '', false)).toBe('hello world');
  });

  it('wraps a case-insensitive substring match in <mark>', () => {
    const { container } = render(<div>{highlightMatches('Hello WORLD', 'world', false)}</div>);
    const marks = container.querySelectorAll('mark');
    expect(marks).toHaveLength(1);
    expect(marks[0].textContent).toBe('WORLD');
  });

  it('wraps every substring match', () => {
    const { container } = render(<div>{highlightMatches('foo bar foo', 'foo', false)}</div>);
    expect(container.querySelectorAll('mark')).toHaveLength(2);
  });

  it('treats substring query characters literally, not as regex', () => {
    const { container } = render(<div>{highlightMatches('a.b.c', 'a.b', false)}</div>);
    const marks = container.querySelectorAll('mark');
    expect(marks).toHaveLength(1);
    expect(marks[0].textContent).toBe('a.b');
  });

  it('applies a regex pattern case-sensitively when isRegex is true', () => {
    const { container } = render(<div>{highlightMatches('status=200 status=500', 'status=(2|5)00', true)}</div>);
    expect(container.querySelectorAll('mark')).toHaveLength(2);
  });

  it('falls back to unhighlighted text for an invalid regex', () => {
    const result = highlightMatches('hello', '(', true);
    expect(result).toBe('hello');
  });
});
