import { fireEvent, render, screen } from '@testing-library/react';
import { ThemeProvider, createTheme } from '@mui/material/styles';
import { MemoryRouter } from 'react-router-dom';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { useIndexValues } from '../../hooks/useIndexValues.js';
import { makeIndexFormatKey, useLogStore } from '../../store/logStore.js';
import { formatDateTime } from '../../utils/formatDateTime.js';
import IndexPanel from './IndexPanel.js';

vi.mock('../../hooks/useIndexLogHistory.js', () => ({
  useIndexLogHistory: vi.fn(),
}));

vi.mock('../../hooks/useIndexValues.js', () => ({
  useIndexValues: vi.fn(),
}));

vi.mock('./LogList.js', () => ({
  default: () => <div>Log list</div>,
}));

const mockUseIndexValues = vi.mocked(useIndexValues);
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
    mockUseIndexValues.mockReturnValue({
      values: [],
      loading: false,
      error: null,
      hasNextPage: false,
      hasPrevPage: false,
      nextPage: vi.fn(),
      prevPage: vi.fn(),
      reload: vi.fn(),
    });
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

  it('shows single-row dates, pagination controls, and no JSON button', () => {
    const nextPage = vi.fn();
    mockUseIndexValues.mockReturnValue({
      values: [{
        value: 'company-1',
        count: 12n,
        lastUpdatedUnixMs: 1_749_470_400_000n,
      }],
      loading: false,
      error: null,
      hasNextPage: true,
      hasPrevPage: false,
      nextPage,
      prevPage: vi.fn(),
      reload: vi.fn(),
    });

    render(<IndexPanel />, { wrapper: Wrapper });

    expect(screen.getByText('company-1')).toBeInTheDocument();
    expect(screen.getByText(formatDateTime(1_749_470_400_000n))).toBeInTheDocument();
    expect(screen.queryByText(/Last updated/)).not.toBeInTheDocument();
    expect(screen.queryByText('{JSON}')).not.toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Previous' })).toBeDisabled();
    fireEvent.click(screen.getByRole('button', { name: 'Next' }));
    expect(nextPage).toHaveBeenCalledOnce();
  });

  it('opens the shared formatter and saves a format for the selected index', () => {
    useLogStore.setState({
      selectedIndexValue: 'company-1',
      mode: 'history',
    });
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
