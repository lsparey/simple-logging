import Box from '@mui/material/Box';
import CircularProgress from '@mui/material/CircularProgress';
import List from '@mui/material/List';
import Typography from '@mui/material/Typography';
import { useEffect } from 'react';
import { useIndexList } from '../../hooks/useIndexList.js';
import { useLogStore } from '../../store/logStore.js';
import IndexNode from './IndexNode.js';

export default function IndexSidebar() {
  const { indexes, loading, error, reload } = useIndexList();
  const indexListVersion = useLogStore((s) => s.indexListVersion);

  useEffect(() => {
    if (indexListVersion > 0) void reload();
  }, [indexListVersion, reload]);

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
      {!loading && indexes.length === 0 && (
        <Typography variant="caption" color="text.secondary" sx={{ display: 'block', px: 2, py: 1.5 }}>
          No indexes
        </Typography>
      )}
      <List disablePadding>
        {indexes.map((idx) => <IndexNode key={idx.key} index={idx} />)}
      </List>
    </Box>
  );
}
