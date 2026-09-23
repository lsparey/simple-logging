import { renderHook, waitFor } from '@testing-library/react';
import { describe, expect, it, vi, beforeEach } from 'vitest';
import { logClient } from '../grpc/client.js';
import { usePodsByNamespace } from './usePodsByNamespace.js';

vi.mock('../grpc/client.js', () => ({
  logClient: {
    listPods: vi.fn(),
  },
}));

const listPods = vi.mocked(logClient.listPods);

const WEB_APP_POD = { name: 'web-app-abc', namespace: 'default', active: true, jsonLogging: false, containers: ['app'] };
const COREDNS_POD = { name: 'coredns-abc', namespace: 'kube-system', active: true, jsonLogging: false, containers: ['app'] };

beforeEach(() => {
  listPods.mockReset();
});

describe('usePodsByNamespace', () => {
  it('fetches every namespace in parallel', async () => {
    listPods.mockImplementation(async ({ namespace }) => ({
      pods: namespace === 'default' ? [WEB_APP_POD] : [COREDNS_POD],
    }));

    const { result } = renderHook(() => usePodsByNamespace(['default', 'kube-system']));

    await waitFor(() => expect(result.current.loading).toBe(false));
    expect(result.current.podsByNamespace).toEqual({
      default: [WEB_APP_POD],
      'kube-system': [COREDNS_POD],
    });
    expect(listPods).toHaveBeenCalledWith({ namespace: 'default' });
    expect(listPods).toHaveBeenCalledWith({ namespace: 'kube-system' });
  });

  it('does not fetch when there are no namespaces', () => {
    renderHook(() => usePodsByNamespace([]));
    expect(listPods).not.toHaveBeenCalled();
  });

  it('surfaces an error from the RPC', async () => {
    listPods.mockRejectedValue(new Error('boom'));

    const { result } = renderHook(() => usePodsByNamespace(['default']));

    await waitFor(() => expect(result.current.error).toBe('Error: boom'));
    expect(result.current.loading).toBe(false);
  });
});
