import List from '@mui/material/List';
import Typography from '@mui/material/Typography';
import Box from '@mui/material/Box';
import CircularProgress from '@mui/material/CircularProgress';
import { useNamespaces } from '../../hooks/useNamespaces.js';
import NamespaceNode from './NamespaceNode.js';

export default function PodSidebar() {
  const { namespaces, loading, error } = useNamespaces();

  return (
    <Box sx={{ overflow: 'auto', height: '100%' }}>
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
      <List disablePadding>
        {namespaces.map((ns) => (
          <NamespaceNode key={ns} namespace={ns} />
        ))}
      </List>
    </Box>
  );
}
