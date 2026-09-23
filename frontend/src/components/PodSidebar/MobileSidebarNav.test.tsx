import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { vi, describe, it, expect, beforeEach } from 'vitest';
import { ThemeProvider, createTheme } from '@mui/material/styles';
import { MemoryRouter } from 'react-router-dom';
import MobileSidebarNav from './MobileSidebarNav.js';
import { useLogStore } from '../../store/logStore.js';

vi.mock('../../hooks/useNamespaces.js', () => ({
  useNamespaces: vi.fn(),
}));
vi.mock('../../hooks/useWorkloadList.js', () => ({
  useWorkloadList: vi.fn(),
}));
vi.mock('../../hooks/useIndexList.js', () => ({
  useIndexList: vi.fn(),
}));

import { useNamespaces } from '../../hooks/useNamespaces.js';
import { useWorkloadList } from '../../hooks/useWorkloadList.js';
import { useIndexList } from '../../hooks/useIndexList.js';

const mockUseNamespaces = vi.mocked(useNamespaces);
const mockUseWorkloadList = vi.mocked(useWorkloadList);
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
  mockUseWorkloadList.mockReturnValue({
    workloads: [{ kind: 'Deployment', name: 'web-app', namespace: 'default', active: true, jsonLogging: false, pods: ['web-app-abc'] }],
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
  it('shows no namespace content until a bottom icon is tapped', () => {
    render(<MobileSidebarNav />, { wrapper: Wrapper });
    expect(screen.queryByText('default')).not.toBeInTheDocument();
  });

  it('opens the overlay to drill into a namespace, and closes it once a workload is picked', async () => {
    render(<MobileSidebarNav />, { wrapper: Wrapper });

    fireEvent.click(screen.getByRole('button', { name: 'Workloads' }));
    await waitFor(() => expect(screen.getByText('default')).toBeInTheDocument());

    fireEvent.click(screen.getByText('default'));
    await waitFor(() => expect(screen.getByText('web-app')).toBeInTheDocument());

    fireEvent.click(screen.getByText('web-app'));

    await waitFor(() => expect(screen.queryByText('default')).not.toBeInTheDocument());
    expect(useLogStore.getState().selectedWorkloadKind).toBe('Deployment');
    expect(useLogStore.getState().selectedWorkloadName).toBe('web-app');
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
