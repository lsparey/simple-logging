import Box from '@mui/material/Box';
import List from '@mui/material/List';
import ListItem from '@mui/material/ListItem';
import ListItemButton from '@mui/material/ListItemButton';
import ListItemIcon from '@mui/material/ListItemIcon';
import ListItemText from '@mui/material/ListItemText';
import Typography from '@mui/material/Typography';
import CircularProgress from '@mui/material/CircularProgress';
import ArrowBackIcon from '@mui/icons-material/ArrowBack';
import { useNamespaces } from '../../hooks/useNamespaces.js';
import { useWorkloadsByNamespace } from '../../hooks/useWorkloadsByNamespace.js';
import NamespaceNode from './NamespaceNode.js';
import IndexSidebar from './IndexSidebar.js';
import { SIDEBAR_SECTIONS, type SidebarSection } from './sidebarSections.js';

interface Props {
  section: SidebarSection;
  onBack: () => void;
  onLeafSelect?: () => void;
}

// Stable reference so the indexes branch doesn't pass a fresh `[]` literal
// to useWorkloadsByNamespace on every render.
const NO_NAMESPACES: string[] = [];

export default function SidebarSectionView({ section, onBack, onLeafSelect }: Props) {
  const { namespaces, loading: namespacesLoading, error: namespacesError } = useNamespaces();
  const label = SIDEBAR_SECTIONS.find((s) => s.key === section)?.label ?? '';

  // Fetched eagerly (not lazily per-expand) so namespaces with nothing of
  // this kind can be hidden from the list up front, instead of only being
  // discovered empty after the user expands them.
  const workloadKind = section === 'indexes' ? null : section;
  const { workloadsByNamespace, loading: workloadsLoading, error: workloadsError } = useWorkloadsByNamespace(
    workloadKind ? namespaces : NO_NAMESPACES,
    workloadKind ?? '',
  );
  const namespacesWithItems = namespaces.filter((ns) => (workloadsByNamespace[ns]?.length ?? 0) > 0);

  const loading = namespacesLoading || (workloadKind !== null && workloadsLoading);
  const error = namespacesError ?? (workloadKind !== null ? workloadsError : null);

  return (
    <Box sx={{ display: 'flex', flexDirection: 'column', height: '100%' }}>
      <List disablePadding>
        <ListItem disablePadding sx={{ borderBottom: 1, borderColor: 'divider' }}>
          <ListItemButton onClick={onBack} dense aria-label="Back">
            <ListItemIcon sx={{ minWidth: 32 }}>
              <ArrowBackIcon fontSize="small" />
            </ListItemIcon>
            <ListItemText primary={label} slotProps={{ primary: { variant: 'body2' } }} />
          </ListItemButton>
        </ListItem>
      </List>

      {section === 'indexes' ? (
        <IndexSidebar onLeafSelect={onLeafSelect} />
      ) : (
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
              No {label}
            </Typography>
          )}
          <List disablePadding>
            {namespacesWithItems.map((ns) => (
              <NamespaceNode
                key={`${ns}-${section}`}
                namespace={ns}
                viewMode={section}
                workloads={workloadsByNamespace[ns] ?? []}
                onLeafSelect={onLeafSelect}
              />
            ))}
          </List>
        </Box>
      )}
    </Box>
  );
}
