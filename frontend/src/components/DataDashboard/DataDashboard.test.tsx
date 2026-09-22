import { render, screen } from '@testing-library/react';
import { ThemeProvider, createTheme } from '@mui/material/styles';
import { describe, expect, it, vi } from 'vitest';
import { useLogFiles } from '../../hooks/useLogFiles.js';
import DataDashboard from './DataDashboard.js';

vi.mock('../../hooks/useLogFiles.js', () => ({
  useLogFiles: vi.fn(),
}));

const mockUseLogFiles = vi.mocked(useLogFiles);
const theme = createTheme();

function Wrapper({ children }: { children: React.ReactNode }) {
  return <ThemeProvider theme={theme}>{children}</ThemeProvider>;
}

const baseState = {
  files: [],
  totalSizeBytes: 0n,
  totalLogFileCount: 0,
  totalIndexFileCount: 0,
  diskUsedPercent: 0,
  diskHighWaterPercent: 90,
  diskLowWaterPercent: 80,
  loading: false,
  error: null,
  refresh: vi.fn(),
};

describe('DataDashboard — disk usage', () => {
  it('shows the current usage percentage and the configured water marks', () => {
    mockUseLogFiles.mockReturnValue({ ...baseState, diskUsedPercent: 42 });
    render(<DataDashboard />, { wrapper: Wrapper });

    expect(screen.getByText('42%')).toBeInTheDocument();
    expect(screen.getByText(/deletes oldest segments at 90%, down to 80%/)).toBeInTheDocument();
  });

  it('renders the disk usage meter at 0% without a configured water mark crossing', () => {
    mockUseLogFiles.mockReturnValue({ ...baseState, diskUsedPercent: 10 });
    render(<DataDashboard />, { wrapper: Wrapper });

    const meter = screen.getByRole('progressbar', { name: 'Disk usage' });
    expect(meter).toHaveAttribute('aria-valuenow', '10');
  });

  it('reflects usage at or above the high water mark in the meter value', () => {
    mockUseLogFiles.mockReturnValue({ ...baseState, diskUsedPercent: 95 });
    render(<DataDashboard />, { wrapper: Wrapper });

    expect(screen.getByText('95%')).toBeInTheDocument();
    const meter = screen.getByRole('progressbar', { name: 'Disk usage' });
    expect(meter).toHaveAttribute('aria-valuenow', '95');
  });
});
