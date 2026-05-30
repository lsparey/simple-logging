import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { vi, describe, it, expect, beforeEach } from 'vitest';
import { act } from 'react';
import { ThemeProvider, createTheme } from '@mui/material/styles';
import NamespaceNode from './NamespaceNode.js';
import PodSidebar from './PodSidebar.js';
import { useLogStore } from '../../store/logStore.js';

// ---------------------------------------------------------------------------
// Mock hooks that make gRPC calls
// ---------------------------------------------------------------------------

vi.mock('../../hooks/useNamespaces.js', () => ({
  useNamespaces: vi.fn(),
}));
vi.mock('../../hooks/usePodList.js', () => ({
  usePodList: vi.fn(),
}));
vi.mock('../../hooks/useDeploymentList.js', () => ({
  useDeploymentList: vi.fn(),
}));

import { useNamespaces } from '../../hooks/useNamespaces.js';
import { usePodList } from '../../hooks/usePodList.js';
import { useDeploymentList } from '../../hooks/useDeploymentList.js';

const mockUseNamespaces = vi.mocked(useNamespaces);
const mockUsePodList = vi.mocked(usePodList);
const mockUseDeploymentList = vi.mocked(useDeploymentList);

// Minimal MUI theme wrapper so MUI components render without warnings
const theme = createTheme();
function Wrapper({ children }: { children: React.ReactNode }) {
  return <ThemeProvider theme={theme}>{children}</ThemeProvider>;
}

// ---------------------------------------------------------------------------
// Shared setup
// ---------------------------------------------------------------------------

beforeEach(() => {
  vi.clearAllMocks();
  // Default: no pods/deployments until the namespace is expanded
  mockUsePodList.mockReturnValue({ pods: [], loading: false, error: null });
  mockUseDeploymentList.mockReturnValue({ deployments: [], loading: false, error: null });

  // Reset store selection state
  useLogStore.setState({
    selectedNamespace: null,
    selectedPod: null,
    selectedDeployment: null,
    lines: [],
    searchText: '',
    prevPageToken: '',
    nextPageToken: '',
    mode: 'idle',
  });
});

// ---------------------------------------------------------------------------
// PodSidebar
// ---------------------------------------------------------------------------

describe('PodSidebar', () => {
  it('shows a loading spinner while namespaces are loading', () => {
    mockUseNamespaces.mockReturnValue({ namespaces: [], loading: true, error: null });
    render(<PodSidebar />, { wrapper: Wrapper });
    expect(screen.getByRole('progressbar')).toBeInTheDocument();
  });

  it('shows an error message when namespace fetch fails', () => {
    mockUseNamespaces.mockReturnValue({ namespaces: [], loading: false, error: 'connection refused' });
    render(<PodSidebar />, { wrapper: Wrapper });
    expect(screen.getByText(/connection refused/)).toBeInTheDocument();
  });

  it('renders namespace buttons after loading', () => {
    mockUseNamespaces.mockReturnValue({
      namespaces: ['default', 'kube-system'],
      loading: false,
      error: null,
    });
    render(<PodSidebar />, { wrapper: Wrapper });
    expect(screen.getByText('default')).toBeInTheDocument();
    expect(screen.getByText('kube-system')).toBeInTheDocument();
  });

  it('defaults to deployments view mode', () => {
    mockUseNamespaces.mockReturnValue({ namespaces: [], loading: false, error: null });
    render(<PodSidebar />, { wrapper: Wrapper });
    expect(screen.getByRole('combobox')).toHaveTextContent('Deployments');
  });
});

// ---------------------------------------------------------------------------
// NamespaceNode — expand / collapse
// ---------------------------------------------------------------------------

describe('NamespaceNode', () => {
  it('renders the namespace label', () => {
    render(<NamespaceNode namespace="default" viewMode="deployments" />, { wrapper: Wrapper });
    expect(screen.getByText('default')).toBeInTheDocument();
  });

  it('does not show children before the node is expanded', () => {
    mockUseDeploymentList.mockReturnValue({
      deployments: [{ name: 'web-app', namespace: 'default', active: true }],
      loading: false,
      error: null,
    });
    render(<NamespaceNode namespace="default" viewMode="deployments" />, { wrapper: Wrapper });
    expect(screen.queryByText('web-app')).not.toBeInTheDocument();
  });

  it('shows deployments after expanding the namespace', async () => {
    mockUseDeploymentList.mockReturnValue({
      deployments: [
        { name: 'web-app', namespace: 'default', active: true },
        { name: 'api-server', namespace: 'default', active: false },
      ],
      loading: false,
      error: null,
    });
    render(<NamespaceNode namespace="default" viewMode="deployments" />, { wrapper: Wrapper });

    fireEvent.click(screen.getByText('default'));

    await waitFor(() => {
      expect(screen.getByText('web-app')).toBeInTheDocument();
      expect(screen.getByText('api-server')).toBeInTheDocument();
    });
  });

  it('shows pods after expanding in pods view mode', async () => {
    mockUsePodList.mockReturnValue({
      pods: [
        { name: 'web-app-6d8c7f', namespace: 'default', active: true },
      ],
      loading: false,
      error: null,
    });
    render(<NamespaceNode namespace="default" viewMode="pods" />, { wrapper: Wrapper });
    fireEvent.click(screen.getByText('default'));

    await waitFor(() => {
      expect(screen.getByText('web-app-6d8c7f')).toBeInTheDocument();
    });
  });

  it('collapses back after a second click', async () => {
    mockUseDeploymentList.mockReturnValue({
      deployments: [{ name: 'web-app', namespace: 'default', active: true }],
      loading: false,
      error: null,
    });
    render(<NamespaceNode namespace="default" viewMode="deployments" />, { wrapper: Wrapper });

    fireEvent.click(screen.getByText('default')); // expand
    await waitFor(() => expect(screen.getByText('web-app')).toBeInTheDocument());

    fireEvent.click(screen.getByText('default')); // collapse
    await waitFor(() => expect(screen.queryByText('web-app')).not.toBeInTheDocument());
  });
});

// ---------------------------------------------------------------------------
// DeploymentNode — selection updates the store
// ---------------------------------------------------------------------------

describe('NamespaceNode — deployment selection', () => {
  it('selecting a deployment updates the store', async () => {
    mockUseDeploymentList.mockReturnValue({
      deployments: [{ name: 'web-app', namespace: 'default', active: true }],
      loading: false,
      error: null,
    });
    render(<NamespaceNode namespace="default" viewMode="deployments" />, { wrapper: Wrapper });
    fireEvent.click(screen.getByText('default'));

    await waitFor(() => expect(screen.getByText('web-app')).toBeInTheDocument());

    act(() => fireEvent.click(screen.getByText('web-app')));

    const s = useLogStore.getState();
    expect(s.selectedNamespace).toBe('default');
    expect(s.selectedDeployment).toBe('web-app');
  });
});
