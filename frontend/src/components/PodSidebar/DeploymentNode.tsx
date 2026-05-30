import ListItem from '@mui/material/ListItem';
import ListItemButton from '@mui/material/ListItemButton';
import ListItemText from '@mui/material/ListItemText';
import Box from '@mui/material/Box';
import type { DeploymentInfo } from '../../gen/simplelog/v1/log_service_pb.js';
import { useLogStore } from '../../store/logStore.js';

interface Props {
  deployment: DeploymentInfo;
}

export default function DeploymentNode({ deployment }: Props) {
  const { selectedNamespace, selectedDeployment, setSelectedDeployment } = useLogStore();
  const selected =
    selectedNamespace === deployment.namespace &&
    selectedDeployment === deployment.name;

  return (
    <ListItem disablePadding sx={{ pl: 3 }}>
      <ListItemButton
        dense
        selected={selected}
        onClick={() => setSelectedDeployment(deployment.namespace, deployment.name, deployment.jsonLogging)}
      >
        <Box
          component="span"
          sx={{
            width: 8,
            height: 8,
            borderRadius: '20%',
            bgcolor: deployment.active ? 'success.main' : 'text.disabled',
            mr: 1,
            flexShrink: 0,
          }}
        />
        <ListItemText
          primary={deployment.name}
          slotProps={{ primary: { variant: 'body2' } }}
        />
      </ListItemButton>
    </ListItem>
  );
}
