import type { ReactNode } from 'react';

/**
 * Splits text into React nodes with every match of query wrapped in <mark>.
 * Mirrors the server's own matching semantics: a case-insensitive substring
 * search, or the exact (case-sensitive) RE2 pattern when isRegex is true. An
 * invalid regex or empty query renders the text unhighlighted.
 */
export function highlightMatches(text: string, query: string, isRegex: boolean): ReactNode {
  if (!query) return text;

  let re: RegExp;
  try {
    re = isRegex ? new RegExp(query, 'g') : new RegExp(escapeRegExp(query), 'gi');
  } catch {
    return text;
  }

  const nodes: ReactNode[] = [];
  let lastIndex = 0;
  let match: RegExpExecArray | null;
  let key = 0;

  while ((match = re.exec(text)) !== null) {
    if (match[0].length === 0) {
      re.lastIndex++;
      continue;
    }
    if (match.index > lastIndex) {
      nodes.push(text.slice(lastIndex, match.index));
    }
    nodes.push(<mark key={key++}>{match[0]}</mark>);
    lastIndex = match.index + match[0].length;
  }
  if (lastIndex < text.length) {
    nodes.push(text.slice(lastIndex));
  }
  return nodes;
}

function escapeRegExp(s: string): string {
  return s.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
}
