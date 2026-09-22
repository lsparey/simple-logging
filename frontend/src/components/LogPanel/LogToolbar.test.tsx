import { render, screen, fireEvent, within } from '@testing-library/react';
import { ThemeProvider, createTheme } from '@mui/material/styles';
import { describe, expect, it, beforeEach, vi } from 'vitest';
import LogToolbar from './LogToolbar.js';
import { useLogStore } from '../../store/logStore.js';

vi.mock('./LogHistogram.js', () => ({
  default: () => <div data-testid="log-histogram">Histogram</div>,
}));

const theme = createTheme();
function Wrapper({ children }: { children: React.ReactNode }) {
  return <ThemeProvider theme={theme}>{children}</ThemeProvider>;
}

beforeEach(() => {
  useLogStore.setState({
    searchText: '',
    jsonLogging: false,
    jsonFormats: {},
    lines: [],
    selectedPodContainers: [],
    selectedContainer: null,
  });
});

describe('LogToolbar — container filter', () => {
  it('does not show a container filter for a single-container pod', () => {
    useLogStore.setState({ selectedPodContainers: ['app'] });
    render(
      <LogToolbar namespace="default" pod="my-pod" liveEnabled={false} onLiveToggle={() => {}} />,
      { wrapper: Wrapper },
    );
    expect(screen.queryByLabelText('Filter by container')).not.toBeInTheDocument();
  });

  it('shows a container filter for a multi-container pod, defaulting to all containers', () => {
    useLogStore.setState({ selectedPodContainers: ['app', 'sidecar'] });
    render(
      <LogToolbar namespace="default" pod="my-pod" liveEnabled={false} onLiveToggle={() => {}} />,
      { wrapper: Wrapper },
    );
    expect(screen.getByText('All containers')).toBeInTheDocument();
  });

  it('does not show a container filter in deployment mode', () => {
    useLogStore.setState({ selectedPodContainers: ['app', 'sidecar'] });
    render(
      <LogToolbar namespace="default" deployment="my-deploy" liveEnabled={false} onLiveToggle={() => {}} />,
      { wrapper: Wrapper },
    );
    expect(screen.queryByLabelText('Filter by container')).not.toBeInTheDocument();
  });

  it('selecting a container updates the store', () => {
    useLogStore.setState({ selectedPodContainers: ['app', 'sidecar'] });
    render(
      <LogToolbar namespace="default" pod="my-pod" liveEnabled={false} onLiveToggle={() => {}} />,
      { wrapper: Wrapper },
    );

    fireEvent.mouseDown(screen.getByLabelText('Filter by container'));
    const listbox = within(screen.getByRole('listbox'));
    fireEvent.click(listbox.getByText('sidecar'));

    expect(useLogStore.getState().selectedContainer).toBe('sidecar');
  });
});
