import Box from '@mui/material/Box';
import IndexesNode from './IndexesNode.js';
import WorkloadTree from './WorkloadTree.js';

export default function PodSidebar() {
  return (
    <Box sx={{ display: 'flex', flexDirection: 'column', height: '100%' }}>
      <Box sx={{ overflow: 'auto', flex: 1 }}>
        <IndexesNode />
        <WorkloadTree />
      </Box>
    </Box>
  );
}
