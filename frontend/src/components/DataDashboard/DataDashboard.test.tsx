import { fireEvent, render, screen } from '@testing-library/react';
import { ThemeProvider, createTheme } from '@mui/material/styles';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { create } from '@bufbuild/protobuf';
import { useLogFiles } from '../../hooks/useLogFiles.js';
import { useStats } from '../../hooks/useStats.js';
import { GetStatsResponseSchema } from '../../gen/simplelog/v1/log_service_pb.js';
import DataDashboard from './DataDashboard.js';

vi.mock('../../hooks/useLogFiles.js', () => ({
  useLogFiles: vi.fn(),
}));
vi.mock('../../hooks/useStats.js', () => ({
  useStats: vi.fn(),
}));

const mockUseLogFiles = vi.mocked(useLogFiles);
const mockUseStats = vi.mocked(useStats);

beforeEach(() => {
  mockUseStats.mockReturnValue({ stats: null, refresh: vi.fn() });
});
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

describe('DataDashboard — collection stats', () => {
  const stats = create(GetStatsResponseSchema, {
    startedAtUnixMs: 1780912800000n,
    streamsActiveFile: 3n,
    streamsActiveApi: 2n,
    linesWrittenTotal: 12345n,
    bytesWrittenTotal: 2048n,
    apiReconnectsTotal: 4n,
  });

  it('is hidden until stats load', () => {
    mockUseLogFiles.mockReturnValue(baseState);
    render(<DataDashboard />, { wrapper: Wrapper });
    expect(screen.queryByRole('heading', { name: 'Collection' })).not.toBeInTheDocument();
  });

  it('shows stream, write and reconnect counters', () => {
    mockUseLogFiles.mockReturnValue(baseState);
    mockUseStats.mockReturnValue({ stats, refresh: vi.fn() });
    render(<DataDashboard />, { wrapper: Wrapper });

    expect(screen.getByRole('heading', { name: 'Collection' })).toBeInTheDocument();
    expect(screen.getByText('Active streams').parentElement).toHaveTextContent('5');
    expect(screen.getByText('3 from node files, 2 from the API')).toBeInTheDocument();
    expect(screen.getByText('Lines written').parentElement).toHaveTextContent((12345).toLocaleString());
    expect(screen.getByText('2.00 KB')).toBeInTheDocument();
    expect(screen.getByText('API reconnects').parentElement).toHaveTextContent('4');
    expect(screen.queryByRole('alert')).not.toBeInTheDocument();
  });

  it('warns when lines have been dropped', () => {
    mockUseLogFiles.mockReturnValue(baseState);
    mockUseStats.mockReturnValue({ stats: { ...stats, linesDroppedTotal: 7n }, refresh: vi.fn() });
    render(<DataDashboard />, { wrapper: Wrapper });

    expect(screen.getByRole('alert')).toHaveTextContent('7 log lines were dropped');
  });

  it('refresh reloads both storage and stats', () => {
    const refreshFiles = vi.fn();
    const refreshStats = vi.fn();
    mockUseLogFiles.mockReturnValue({ ...baseState, refresh: refreshFiles });
    mockUseStats.mockReturnValue({ stats, refresh: refreshStats });
    render(<DataDashboard />, { wrapper: Wrapper });

    fireEvent.click(screen.getByRole('button', { name: 'Refresh' }));
    expect(refreshFiles).toHaveBeenCalledOnce();
    expect(refreshStats).toHaveBeenCalledOnce();
  });
});
