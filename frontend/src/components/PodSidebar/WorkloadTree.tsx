import Box from '@mui/material/Box';
import List from '@mui/material/List';
import Typography from '@mui/material/Typography';
import CircularProgress from '@mui/material/CircularProgress';
import { useNamespaces } from '../../hooks/useNamespaces.js';
import { useWorkloadsByNamespace } from '../../hooks/useWorkloadsByNamespace.js';
import { usePodsByNamespace } from '../../hooks/usePodsByNamespace.js';
import NamespaceNode from './NamespaceNode.js';
import type { SidebarWorkload } from './sidebarSections.js';
import type { PodInfo } from '../../gen/simplelog/v1/log_service_pb.js';

interface Props {
  onLeafSelect?: () => void;
}

// The Pods kind means "every individual pod", not "workloads owned by
// nothing" (ListWorkloads' kind "Pod" is the latter, and mostly empty in a
// real cluster since almost every pod has an owner) — so its entries come
// from ListPods instead, replacing whatever ListWorkloads reported for kind
// "Pod".
function podAsSidebarWorkload(pod: PodInfo): SidebarWorkload {
  return { kind: 'Pod', name: pod.name, namespace: pod.namespace, active: pod.active, jsonLogging: pod.jsonLogging, pods: [pod.name] };
}

/**
 * The namespace -> workload kind -> workload accordion shared by the desktop
 * sidebar and the mobile "Workloads" drawer.
 */
export default function WorkloadTree({ onLeafSelect }: Props) {
  const { namespaces, loading: namespacesLoading, error: namespacesError } = useNamespaces();
  const { workloadsByNamespace: groupedByNamespace, loading: groupedLoading, error: groupedError } = useWorkloadsByNamespace(namespaces);
  const { podsByNamespace, loading: podsLoading, error: podsError } = usePodsByNamespace(namespaces);

  const workloadsByNamespace: Record<string, SidebarWorkload[]> = {};
  for (const ns of namespaces) {
    const grouped = (groupedByNamespace[ns] ?? []).filter((w) => w.kind !== 'Pod');
    const pods = (podsByNamespace[ns] ?? []).map(podAsSidebarWorkload);
    workloadsByNamespace[ns] = [...grouped, ...pods];
  }
  const namespacesWithItems = namespaces.filter((ns) => (workloadsByNamespace[ns]?.length ?? 0) > 0);

  const loading = namespacesLoading || groupedLoading || podsLoading;
  const error = namespacesError ?? groupedError ?? podsError;

  return (
    <Box sx={{ overflow: 'auto', flex: 1 }}>
      {loading && (
        <Box sx={{ display: 'flex', justifyContent: 'center', mt: 3 }}>
          <CircularProgress size={20} />
        </Box>
      )}
      {error && (
        <Typography variant="caption" color="error" sx={{ px: 2 }}>
          {error}
        </Typography>
      )}
      {!loading && !error && namespacesWithItems.length === 0 && (
        <Typography variant="body2" color="text.disabled" sx={{ px: 2, py: 1 }}>
          No workloads
        </Typography>
      )}
      <List disablePadding>
        {namespacesWithItems.map((ns) => (
          <NamespaceNode key={ns} namespace={ns} workloads={workloadsByNamespace[ns] ?? []} onLeafSelect={onLeafSelect} />
        ))}
      </List>
    </Box>
  );
}
