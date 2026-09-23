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

function renderToolbar(props: Partial<React.ComponentProps<typeof LogToolbar>> = {}) {
  return render(
    <LogToolbar
      namespace="default"
      kind="Pod"
      name="my-pod"
      liveEnabled={false}
      onLiveToggle={() => {}}
      {...props}
    />,
    { wrapper: Wrapper },
  );
}

beforeEach(() => {
  useLogStore.setState({
    searchText: '',
    jsonLogging: false,
    jsonFormats: {},
    lines: [],
    selectedContainerFilter: null,
    startTime: 0,
    endTime: 0,
  });
});

describe('LogToolbar — container filter', () => {
  it('does not show the filter when all lines come from one container', () => {
    useLogStore.setState({
      lines: [
        '2026-05-20T10:00:00Z [default/my-pod/app] a',
        '2026-05-20T10:00:01Z [default/my-pod/app] b',
      ],
    });
    renderToolbar();
    expect(screen.queryByLabelText('Filter by container')).not.toBeInTheDocument();
  });

  it('shows the filter when lines span multiple containers, defaulting to all containers', () => {
    useLogStore.setState({
      lines: [
        '2026-05-20T10:00:00Z [default/my-pod/app] a',
        '2026-05-20T10:00:01Z [default/my-pod/sidecar] b',
      ],
    });
    renderToolbar();
    expect(screen.getByText('All containers')).toBeInTheDocument();
  });

  it('does not show a pod filter, even when lines span multiple pods', () => {
    useLogStore.setState({
      lines: [
        '2026-05-20T10:00:00Z [default/pod-a/app] a',
        '2026-05-20T10:00:01Z [default/pod-b/app] b',
      ],
    });
    renderToolbar({ kind: 'Deployment', name: 'web-app' });
    expect(screen.queryByLabelText('Filter by pod')).not.toBeInTheDocument();
  });

  it('selecting a container updates the store', () => {
    useLogStore.setState({
      lines: [
        '2026-05-20T10:00:00Z [default/my-pod/app] a',
        '2026-05-20T10:00:01Z [default/my-pod/sidecar] b',
      ],
    });
    renderToolbar();

    fireEvent.mouseDown(screen.getByLabelText('Filter by container'));
    const listbox = within(screen.getByRole('listbox'));
    fireEvent.click(listbox.getByText('sidecar'));

    expect(useLogStore.getState().selectedContainerFilter).toBe('sidecar');
  });
});

describe('LogToolbar — filter field', () => {
  it('shows the client-side filter field', () => {
    renderToolbar();
    expect(screen.getByPlaceholderText('Filter…')).toBeInTheDocument();
  });

  it('typing updates the store search text', () => {
    renderToolbar();
    fireEvent.change(screen.getByPlaceholderText('Filter…'), { target: { value: 'error' } });
    expect(useLogStore.getState().searchText).toBe('error');
  });
});

describe('LogToolbar — download button', () => {
  it('downloads by the selected workload kind and name, honoring the container filter and time range', () => {
    useLogStore.setState({ selectedContainerFilter: 'sidecar', startTime: 1000, endTime: 2000 });
    const openSpy = vi.spyOn(window, 'open').mockImplementation(() => null);
    renderToolbar({ namespace: 'default', kind: 'Pod', name: 'web-app-abc' });

    fireEvent.click(screen.getByLabelText('Download logs'));

    expect(openSpy).toHaveBeenCalledTimes(1);
    const url = openSpy.mock.calls[0][0] as string;
    expect(url).toContain('/download?');
    expect(url).toContain('ns=default');
    expect(url).toContain('kind=Pod');
    expect(url).toContain('name=web-app-abc');
    expect(url).toContain('container=sidecar');
    expect(url).toContain('from=1000');
    expect(url).toContain('to=2000');
    openSpy.mockRestore();
  });
});
