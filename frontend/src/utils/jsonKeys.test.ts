import { describe, expect, it } from 'vitest';
import { candidateJsonKeys, observedJsonKeys } from './jsonKeys.js';

describe('JSON key discovery', () => {
  const mixedSchemaLines = [
    '2026-06-09T10:00:00Z [default/api/app] {"time":"now","level":30,"msg":"ready"}',
    '2026-06-09T10:00:01Z [default/worker/app] {"timestamp":"later","severity":"info","message":"working"}',
  ];

  it('keeps pod formatting suggestions limited to shared keys', () => {
    expect(candidateJsonKeys(mixedSchemaLines)).toEqual([]);
  });

  it('includes keys from every schema for index formatting', () => {
    expect(observedJsonKeys(mixedSchemaLines)).toEqual([
      'level',
      'message',
      'msg',
      'severity',
      'time',
      'timestamp',
    ]);
  });
});
