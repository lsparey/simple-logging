import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { vi, describe, it, expect, beforeEach } from 'vitest';
import { ThemeProvider, createTheme } from '@mui/material/styles';
import { MemoryRouter } from 'react-router-dom';
import MobileSidebarNav from './MobileSidebarNav.js';
import { useLogStore } from '../../store/logStore.js';

vi.mock('../../hooks/useNamespaces.js', () => ({
  useNamespaces: vi.fn(),
}));
vi.mock('../../hooks/useWorkloadsByNamespace.js', () => ({
  useWorkloadsByNamespace: vi.fn(),
}));
vi.mock('../../hooks/useIndexList.js', () => ({
  useIndexList: vi.fn(),
}));

import { useNamespaces } from '../../hooks/useNamespaces.js';
import { useWorkloadsByNamespace } from '../../hooks/useWorkloadsByNamespace.js';
import { useIndexList } from '../../hooks/useIndexList.js';

const mockUseNamespaces = vi.mocked(useNamespaces);
const mockUseWorkloadsByNamespace = vi.mocked(useWorkloadsByNamespace);
const mockUseIndexList = vi.mocked(useIndexList);

const theme = createTheme();
function Wrapper({ children }: { children: React.ReactNode }) {
  return (
    <MemoryRouter>
      <ThemeProvider theme={theme}>{children}</ThemeProvider>
    </MemoryRouter>
  );
}

beforeEach(() => {
  vi.clearAllMocks();
  mockUseNamespaces.mockReturnValue({ namespaces: ['default'], loading: false, error: null });
  mockUseWorkloadsByNamespace.mockReturnValue({
    workloadsByNamespace: {
      default: [{ kind: 'Deployment', name: 'web-app', namespace: 'default', active: true, jsonLogging: false, pods: ['web-app-abc'] }],
    },
    loading: false,
    error: null,
  });
  mockUseIndexList.mockReturnValue({
    indexes: [{ key: 'idx1' }],
    loading: false,
    error: null,
    reload: vi.fn(),
  });

  useLogStore.setState({
    selectedNamespace: null,
    selectedWorkloadKind: null,
    selectedWorkloadName: null,
    selectedIndexKey: null,
    lines: [],
    searchText: '',
    prevPageToken: '',
    nextPageToken: '',
    mode: 'idle',
  });
});

describe('MobileSidebarNav', () => {
  it('shows only the Indexes and Workloads bottom icons', () => {
    render(<MobileSidebarNav />, { wrapper: Wrapper });
    expect(screen.getByRole('button', { name: 'Indexes' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Workloads' })).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Deployments' })).not.toBeInTheDocument();
    expect(screen.queryByText('default')).not.toBeInTheDocument();
  });

  it('tapping Workloads opens a kind picker before any namespace is shown', () => {
    render(<MobileSidebarNav />, { wrapper: Wrapper });

    fireEvent.click(screen.getByRole('button', { name: 'Workloads' }));

    expect(screen.getByText('Deployments')).toBeInTheDocument();
    expect(screen.getByText('StatefulSets')).toBeInTheDocument();
    expect(screen.getByText('DaemonSets')).toBeInTheDocument();
    expect(screen.getByText('Jobs')).toBeInTheDocument();
    expect(screen.getByText('CronJobs')).toBeInTheDocument();
    expect(screen.getByText('Pods')).toBeInTheDocument();
    expect(screen.queryByText('default')).not.toBeInTheDocument();
  });

  it('drills from the kind picker into namespaces and a workload, closing the overlay once selected', async () => {
    render(<MobileSidebarNav />, { wrapper: Wrapper });

    fireEvent.click(screen.getByRole('button', { name: 'Workloads' }));
    fireEvent.click(screen.getByText('Deployments'));
    await waitFor(() => expect(screen.getByText('default')).toBeInTheDocument());

    fireEvent.click(screen.getByText('default'));
    await waitFor(() => expect(screen.getByText('web-app')).toBeInTheDocument());

    fireEvent.click(screen.getByText('web-app'));

    await waitFor(() => expect(screen.queryByText('default')).not.toBeInTheDocument());
    expect(useLogStore.getState().selectedWorkloadKind).toBe('Deployment');
    expect(useLogStore.getState().selectedWorkloadName).toBe('web-app');
  });

  it('the back arrow from a kind\'s namespace list returns to the kind picker, not straight to the icons', async () => {
    render(<MobileSidebarNav />, { wrapper: Wrapper });

    fireEvent.click(screen.getByRole('button', { name: 'Workloads' }));
    fireEvent.click(screen.getByText('Deployments'));
    await waitFor(() => expect(screen.getByText('default')).toBeInTheDocument());

    fireEvent.click(screen.getByRole('button', { name: 'Back' }));

    expect(screen.queryByText('default')).not.toBeInTheDocument();
    expect(screen.getByText('Deployments')).toBeInTheDocument();
    expect(screen.getByText('Pods')).toBeInTheDocument();
  });

  it('opens the overlay for Indexes without immediately closing it', async () => {
    render(<MobileSidebarNav />, { wrapper: Wrapper });

    fireEvent.click(screen.getByRole('button', { name: 'Indexes' }));

    // enterIndexMode() bumps selectionKey; the overlay must stay open regardless.
    await waitFor(() => expect(screen.getByText('idx1')).toBeInTheDocument());

    fireEvent.click(screen.getByText('idx1'));
    await waitFor(() => expect(screen.queryByText('idx1')).not.toBeInTheDocument());
    expect(useLogStore.getState().selectedIndexKey).toBe('idx1');
  });
});
