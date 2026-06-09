import { fireEvent, render, screen } from '@testing-library/react';
import { ThemeProvider, createTheme } from '@mui/material/styles';
import { MemoryRouter } from 'react-router-dom';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { makeIndexFormatKey, useLogStore } from '../../store/logStore.js';
import IndexPanel from './IndexPanel.js';

vi.mock('../../hooks/useIndexLogHistory.js', () => ({
  useIndexLogHistory: vi.fn(),
}));

vi.mock('../../hooks/useIndexValues.js', () => ({
  useIndexValues: vi.fn(() => ({
    values: [],
    loading: false,
    error: null,
  })),
}));

const theme = createTheme();

function Wrapper({ children }: { children: React.ReactNode }) {
  return (
    <MemoryRouter>
      <ThemeProvider theme={theme}>{children}</ThemeProvider>
    </MemoryRouter>
  );
}

describe('IndexPanel JSON formatting', () => {
  beforeEach(() => {
    localStorage.removeItem('simple-logging.jsonFormats');
    useLogStore.setState({
      selectedIndexKey: 'companyUuid',
      selectedIndexValue: '',
      lines: [
        '2026-06-09T10:00:00Z [default/api/app] {"time":"now","level":30,"msg":"ready"}',
        '2026-06-09T10:00:01Z [default/worker/app] {"timestamp":"later","severity":"info","message":"working"}',
      ],
      jsonFormats: {},
      mode: 'idle',
    });
  });

  it('opens the shared formatter and saves a format for the selected index', () => {
    render(<IndexPanel />, { wrapper: Wrapper });

    fireEvent.click(screen.getByText('{JSON}'));

    expect(screen.getByRole('dialog')).toHaveTextContent('Format JSON Log Properties');
    expect(screen.getByLabelText('Timestamp')).toHaveValue('time');
    expect(screen.getByLabelText('Level')).toHaveValue('level');
    expect(screen.getByLabelText('Message')).toHaveValue('message');

    fireEvent.click(screen.getByRole('button', { name: 'Save' }));

    expect(useLogStore.getState().jsonFormats[makeIndexFormatKey('companyUuid')]).toEqual({
      timestampKey: 'time',
      levelKey: 'level',
      messageKey: 'message',
    });
  });
});
