import { renderHook, waitFor } from '@testing-library/react';
import { describe, expect, it, vi, beforeEach } from 'vitest';
import { logClient } from '../grpc/client.js';
import { useWorkloadsByNamespace } from './useWorkloadsByNamespace.js';

vi.mock('../grpc/client.js', () => ({
  logClient: {
    listWorkloads: vi.fn(),
  },
}));

const listWorkloads = vi.mocked(logClient.listWorkloads);

const WEB_APP = { kind: 'Deployment', name: 'web-app', namespace: 'default', active: true, jsonLogging: false, pods: ['web-app-abc'] };
const CACHE = { kind: 'StatefulSet', name: 'cache', namespace: 'default', active: true, jsonLogging: false, pods: ['cache-0'] };
const COREDNS = { kind: 'Deployment', name: 'coredns', namespace: 'kube-system', active: true, jsonLogging: false, pods: ['coredns-abc'] };

beforeEach(() => {
  listWorkloads.mockReset();
});

describe('useWorkloadsByNamespace', () => {
  it('fetches every namespace in parallel and filters each to the given kind', async () => {
    listWorkloads.mockImplementation(async ({ namespace }) => ({
      workloads: namespace === 'default' ? [WEB_APP, CACHE] : [COREDNS],
    }));

    const { result } = renderHook(() => useWorkloadsByNamespace(['default', 'kube-system'], 'Deployment'));

    await waitFor(() => expect(result.current.loading).toBe(false));
    expect(result.current.workloadsByNamespace).toEqual({
      default: [WEB_APP],
      'kube-system': [COREDNS],
    });
    expect(listWorkloads).toHaveBeenCalledWith({ namespace: 'default' });
    expect(listWorkloads).toHaveBeenCalledWith({ namespace: 'kube-system' });
  });

  it('returns an empty entry for a namespace with nothing of that kind', async () => {
    listWorkloads.mockResolvedValue({ workloads: [WEB_APP] });

    const { result } = renderHook(() => useWorkloadsByNamespace(['default'], 'StatefulSet'));

    await waitFor(() => expect(result.current.loading).toBe(false));
    expect(result.current.workloadsByNamespace).toEqual({ default: [] });
  });

  it('does not fetch when there are no namespaces', () => {
    renderHook(() => useWorkloadsByNamespace([], 'Deployment'));
    expect(listWorkloads).not.toHaveBeenCalled();
  });

  it('surfaces an error from the RPC', async () => {
    listWorkloads.mockRejectedValue(new Error('boom'));

    const { result } = renderHook(() => useWorkloadsByNamespace(['default'], 'Deployment'));

    await waitFor(() => expect(result.current.error).toBe('Error: boom'));
    expect(result.current.loading).toBe(false);
  });
});
