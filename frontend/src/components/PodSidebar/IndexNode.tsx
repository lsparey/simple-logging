import ListItem from '@mui/material/ListItem';
import ListItemButton from '@mui/material/ListItemButton';
import ListItemText from '@mui/material/ListItemText';
import Box from '@mui/material/Box';
import { useNavigate } from 'react-router-dom';
import type { LogIndexInfo } from '../../gen/simplelog/v1/log_service_pb.js';
import { useLogStore } from '../../store/logStore.js';

interface Props {
  index: LogIndexInfo;
  onSelect?: () => void;
}

export default function IndexNode({ index, onSelect }: Props) {
  const { selectedIndexKey, setSelectedIndex } = useLogStore();
  const navigate = useNavigate();
  const selected = selectedIndexKey === index.key;

  return (
    <ListItem disablePadding sx={{ pl: 2 }}>
      <ListItemButton
        dense
        selected={selected}
        onClick={() => {
          setSelectedIndex(index.key);
          navigate(`/index/${encodeURIComponent(index.key)}`);
          onSelect?.();
        }}
      >
        <Box
          component="span"
          sx={{
            width: 8,
            height: 8,
            borderRadius: '50%',
            bgcolor: 'info.main',
            mr: 1,
            flexShrink: 0,
          }}
        />
        <ListItemText
          primary={index.key}
          slotProps={{ primary: { variant: 'body2', sx: { fontFamily: 'monospace' } } }}
        />
      </ListItemButton>
    </ListItem>
  );
}
