import ListItem from '@mui/material/ListItem';
import ListItemButton from '@mui/material/ListItemButton';
import ListItemText from '@mui/material/ListItemText';
import Box from '@mui/material/Box';
import Chip from '@mui/material/Chip';
import Tooltip from '@mui/material/Tooltip';
import { useEffect } from 'react';
import { useNavigate } from 'react-router-dom';
import type { SidebarWorkload } from './sidebarSections.js';
import { useLogStore } from '../../store/logStore.js';

interface Props {
  workload: SidebarWorkload;
  onSelect?: () => void;
}

export default function WorkloadNode({ workload, onSelect }: Props) {
  const { selectedNamespace, selectedWorkloadKind, selectedWorkloadName, setSelectedWorkload, setJsonLogging } = useLogStore();
  const navigate = useNavigate();
  const selected =
    selectedNamespace === workload.namespace &&
    selectedWorkloadKind === workload.kind &&
    selectedWorkloadName === workload.name;

  // Keep the toolbar in sync when polling updates this workload's jsonLogging flag.
  useEffect(() => {
    if (selected) setJsonLogging(workload.jsonLogging);
  }, [selected, workload.jsonLogging, setJsonLogging]);

  return (
    <ListItem disablePadding sx={{ pl: 4 }}>
      <ListItemButton
        dense
        selected={selected}
        onClick={() => {
          setSelectedWorkload(workload.namespace, workload.kind, workload.name, workload.jsonLogging);
          navigate(
            `/ns/${encodeURIComponent(workload.namespace)}/${encodeURIComponent(workload.kind)}/${encodeURIComponent(workload.name)}`,
          );
          onSelect?.();
        }}
      >
        <Box
          component="span"
          sx={{
            width: 8,
            height: 8,
            borderRadius: '50%',
            bgcolor: workload.active ? 'success.main' : 'text.disabled',
            mr: 1,
            flexShrink: 0,
          }}
        />
        <ListItemText
          primary={workload.name}
          slotProps={{ primary: { variant: 'body2', noWrap: true } }}
        />
        {workload.pods.length > 1 && (
          <Tooltip title={workload.pods.join(', ')}>
            <Chip
              label={workload.pods.length}
              size="small"
              variant="outlined"
              sx={{ height: 18, fontSize: '0.6875rem', ml: 1, flexShrink: 0 }}
            />
          </Tooltip>
        )}
      </ListItemButton>
    </ListItem>
  );
}
