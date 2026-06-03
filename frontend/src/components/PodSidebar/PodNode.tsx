import ListItem from '@mui/material/ListItem';
import ListItemButton from '@mui/material/ListItemButton';
import ListItemText from '@mui/material/ListItemText';
import Box from '@mui/material/Box';
import { useEffect } from 'react';
import { useNavigate } from 'react-router-dom';
import type { PodInfo } from '../../gen/simplelog/v1/log_service_pb.js';
import { useLogStore } from '../../store/logStore.js';

interface Props {
  pod: PodInfo;
}

export default function PodNode({ pod }: Props) {
  const { selectedNamespace, selectedPod, setSelectedPod, setJsonLogging } = useLogStore();
  const navigate = useNavigate();
  const selected = selectedNamespace === pod.namespace && selectedPod === pod.name;

  // Keep the toolbar in sync when polling updates this pod's jsonLogging flag.
  useEffect(() => {
    if (selected) setJsonLogging(pod.jsonLogging);
  }, [selected, pod.jsonLogging, setJsonLogging]);

  return (
    <ListItem disablePadding sx={{ pl: 3 }}>
      <ListItemButton
        dense
        selected={selected}
        onClick={() => {
          setSelectedPod(pod.namespace, pod.name, pod.jsonLogging);
          navigate(`/pod/${encodeURIComponent(pod.namespace)}/${encodeURIComponent(pod.name)}`);
        }}
      >
        <Box
          component="span"
          sx={{
            width: 8,
            height: 8,
            borderRadius: '50%',
            bgcolor: pod.active ? 'success.main' : 'text.disabled',
            mr: 1,
            flexShrink: 0,
          }}
        />
        <ListItemText
          primary={pod.name}
          slotProps={{ primary: { variant: 'body2' } }}
        />
      </ListItemButton>
    </ListItem>
  );
}
