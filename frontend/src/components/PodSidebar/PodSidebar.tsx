import { useState } from 'react';
import List from '@mui/material/List';
import Typography from '@mui/material/Typography';
import Box from '@mui/material/Box';
import CircularProgress from '@mui/material/CircularProgress';
import MenuItem from '@mui/material/MenuItem';
import Select from '@mui/material/Select';
import FormControl from '@mui/material/FormControl';
import { styled } from '@mui/material/styles';
import { useLocation, useNavigate } from 'react-router-dom';
import { useNamespaces } from '../../hooks/useNamespaces.js';
import { useLogStore } from '../../store/logStore.js';
import NamespaceNode from './NamespaceNode.js';
import IndexSidebar from './IndexSidebar.js';

const ViewModeMenuItem = styled(MenuItem)({ fontSize: '0.8125rem' });

type ViewMode = 'pods' | 'deployments' | 'indexes';

export default function PodSidebar() {
  const { namespaces, loading, error } = useNamespaces();
  const location = useLocation();
  const navigate = useNavigate();
  const { enterIndexMode, leaveIndexMode } = useLogStore();
  const [viewMode, setViewMode] = useState<ViewMode>(
    location.pathname.startsWith('/index/') || location.pathname.startsWith('/indexes')
      ? 'indexes'
      : location.pathname.startsWith('/pod/')
        ? 'pods'
        : 'deployments',
  );

  return (
    <Box sx={{ display: 'flex', flexDirection: 'column', height: '100%' }}>
      <Box sx={{ px: 1.5, py: 1, borderBottom: 1, borderColor: 'divider' }}>
        <FormControl size="small" fullWidth>
          <Select
            value={viewMode}
            onChange={(e) => {
              const next = e.target.value as ViewMode;
              setViewMode(next);
              if (next === 'indexes') {
                enterIndexMode();
                navigate('/indexes');
              } else {
                leaveIndexMode();
              }
            }}
            displayEmpty
            sx={{ fontSize: '0.8125rem' }}
          >
            <ViewModeMenuItem value="pods">Pods</ViewModeMenuItem>
            <ViewModeMenuItem value="deployments">Deployments</ViewModeMenuItem>
            <ViewModeMenuItem value="indexes">Indexes</ViewModeMenuItem>
          </Select>
        </FormControl>
      </Box>

      {viewMode === 'indexes' ? (
        <IndexSidebar />
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
          <List disablePadding>
            {namespaces.map((ns) => (
              <NamespaceNode key={`${ns}-${viewMode}`} namespace={ns} viewMode={viewMode} />
            ))}
          </List>
        </Box>
      )}
    </Box>
  );
}
