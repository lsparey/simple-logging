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
    selectedPodFilter: null,
    selectedContainerFilter: null,
  });
});

describe('LogToolbar — pod and container filters', () => {
  it('does not show either filter when all lines come from one pod/container', () => {
    useLogStore.setState({
      lines: [
        '2026-05-20T10:00:00Z [default/my-pod/app] a',
        '2026-05-20T10:00:01Z [default/my-pod/app] b',
      ],
    });
    render(
      <LogToolbar namespace="default" kind="Pod" name="my-pod" liveEnabled={false} onLiveToggle={() => {}} />,
      { wrapper: Wrapper },
    );
    expect(screen.queryByLabelText('Filter by pod')).not.toBeInTheDocument();
    expect(screen.queryByLabelText('Filter by container')).not.toBeInTheDocument();
  });

  it('shows a container filter when lines span multiple containers, defaulting to all containers', () => {
    useLogStore.setState({
      lines: [
        '2026-05-20T10:00:00Z [default/my-pod/app] a',
        '2026-05-20T10:00:01Z [default/my-pod/sidecar] b',
      ],
    });
    render(
      <LogToolbar namespace="default" kind="Pod" name="my-pod" liveEnabled={false} onLiveToggle={() => {}} />,
      { wrapper: Wrapper },
    );
    expect(screen.getByText('All containers')).toBeInTheDocument();
  });

  it('shows a pod filter when lines span multiple pods, defaulting to all pods', () => {
    useLogStore.setState({
      lines: [
        '2026-05-20T10:00:00Z [default/pod-a/app] a',
        '2026-05-20T10:00:01Z [default/pod-b/app] b',
      ],
    });
    render(
      <LogToolbar namespace="default" kind="Deployment" name="web-app" liveEnabled={false} onLiveToggle={() => {}} />,
      { wrapper: Wrapper },
    );
    expect(screen.getByText('All pods')).toBeInTheDocument();
  });

  it('selecting a container updates the store', () => {
    useLogStore.setState({
      lines: [
        '2026-05-20T10:00:00Z [default/my-pod/app] a',
        '2026-05-20T10:00:01Z [default/my-pod/sidecar] b',
      ],
    });
    render(
      <LogToolbar namespace="default" kind="Pod" name="my-pod" liveEnabled={false} onLiveToggle={() => {}} />,
      { wrapper: Wrapper },
    );

    fireEvent.mouseDown(screen.getByLabelText('Filter by container'));
    const listbox = within(screen.getByRole('listbox'));
    fireEvent.click(listbox.getByText('sidecar'));

    expect(useLogStore.getState().selectedContainerFilter).toBe('sidecar');
  });

  it('selecting a pod updates the store', () => {
    useLogStore.setState({
      lines: [
        '2026-05-20T10:00:00Z [default/pod-a/app] a',
        '2026-05-20T10:00:01Z [default/pod-b/app] b',
      ],
    });
    render(
      <LogToolbar namespace="default" kind="Deployment" name="web-app" liveEnabled={false} onLiveToggle={() => {}} />,
      { wrapper: Wrapper },
    );

    fireEvent.mouseDown(screen.getByLabelText('Filter by pod'));
    const listbox = within(screen.getByRole('listbox'));
    fireEvent.click(listbox.getByText('pod-b'));

    expect(useLogStore.getState().selectedPodFilter).toBe('pod-b');
  });
});
