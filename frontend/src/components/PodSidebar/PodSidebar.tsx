import { useState } from 'react';
import List from '@mui/material/List';
import Typography from '@mui/material/Typography';
import Box from '@mui/material/Box';
import CircularProgress from '@mui/material/CircularProgress';
import MenuItem from '@mui/material/MenuItem';
import Select from '@mui/material/Select';
import FormControl from '@mui/material/FormControl';
import { styled } from '@mui/material/styles';
import { useNamespaces } from '../../hooks/useNamespaces.js';
import NamespaceNode from './NamespaceNode.js';

const ViewModeMenuItem = styled(MenuItem)({ fontSize: '0.8125rem' });

type ViewMode = 'pods' | 'deployments';

export default function PodSidebar() {
  const { namespaces, loading, error } = useNamespaces();
  const [viewMode, setViewMode] = useState<ViewMode>('deployments');

  return (
    <Box sx={{ display: 'flex', flexDirection: 'column', height: '100%' }}>
      <Box sx={{ px: 1.5, py: 1, borderBottom: 1, borderColor: 'divider' }}>
        <FormControl size="small" fullWidth>
          <Select
            value={viewMode}
            onChange={(e) => setViewMode(e.target.value as ViewMode)}
            displayEmpty
            sx={{ fontSize: '0.8125rem' }}
          >
            <ViewModeMenuItem value="pods">Pods</ViewModeMenuItem>
            <ViewModeMenuItem value="deployments">Deployments</ViewModeMenuItem>
          </Select>
        </FormControl>
      </Box>

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
        <List disablePadding>
          {namespaces.map((ns) => (
            <NamespaceNode key={`${ns}-${viewMode}`} namespace={ns} viewMode={viewMode} />
          ))}
        </List>
      </Box>
    </Box>
  );
}
