import ListItem from '@mui/material/ListItem';
import ListItemButton from '@mui/material/ListItemButton';
import ListItemIcon from '@mui/material/ListItemIcon';
import ListItemText from '@mui/material/ListItemText';
import KeyIcon from '@mui/icons-material/Key';
import { useNavigate } from 'react-router-dom';
import type { LogIndexInfo } from '../../gen/simplelog/v1/log_service_pb.js';
import { useLogStore } from '../../store/logStore.js';

interface Props {
  index: LogIndexInfo;
}

export default function IndexNode({ index }: Props) {
  const { selectedIndexKey, setSelectedIndex } = useLogStore();
  const navigate = useNavigate();
  const selected = selectedIndexKey === index.key;

  return (
    <ListItem disablePadding>
      <ListItemButton
        dense
        selected={selected}
        onClick={() => {
          setSelectedIndex(index.key);
          navigate(`/index/${encodeURIComponent(index.key)}`);
        }}
      >
        <ListItemIcon sx={{ minWidth: 32 }}>
          <KeyIcon fontSize="small" />
        </ListItemIcon>
        <ListItemText
          primary={index.key}
          slotProps={{ primary: { variant: 'body2', sx: { fontFamily: 'monospace' } } }}
        />
      </ListItemButton>
    </ListItem>
  );
}
