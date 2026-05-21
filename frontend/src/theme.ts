import { createTheme, type Theme } from '@mui/material/styles';

const baseTheme = {
  typography: {
    fontFamily: '"JetBrains Mono", "Fira Code", "Cascadia Code", monospace, sans-serif',
  },
  components: {
    MuiCssBaseline: {
      styleOverrides: {
        body: { margin: 0, padding: 0 },
      },
    },
  },
};

export const lightTheme: Theme = createTheme({
  ...baseTheme,
  palette: { mode: 'light' },
  components: {
    ...baseTheme.components,
    MuiCssBaseline: {
      styleOverrides: {
        body: { margin: 0, padding: 0 },
        '*': {
          scrollbarWidth: 'thin',
          scrollbarColor: '#c1c4c9 #f1f3f4',
        },
        '*::-webkit-scrollbar': { width: '8px', height: '8px' },
        '*::-webkit-scrollbar-track': { background: '#f1f3f4' },
        '*::-webkit-scrollbar-thumb': {
          background: '#c1c4c9',
          borderRadius: '4px',
          border: '2px solid #f1f3f4',
        },
        '*::-webkit-scrollbar-thumb:hover': { background: '#9ea3a8' },
      },
    },
  },
});

export const darkTheme: Theme = createTheme({
  ...baseTheme,
  palette: {
    mode: 'dark',
    background: { default: '#0d1117', paper: '#161b22' },
  },
  components: {
    ...baseTheme.components,
    MuiCssBaseline: {
      styleOverrides: {
        body: { margin: 0, padding: 0 },
        '*': {
          scrollbarWidth: 'thin',
          scrollbarColor: '#30363d #161b22',
        },
        '*::-webkit-scrollbar': { width: '8px', height: '8px' },
        '*::-webkit-scrollbar-track': { background: '#161b22' },
        '*::-webkit-scrollbar-thumb': {
          background: '#30363d',
          borderRadius: '4px',
          border: '2px solid #161b22',
        },
        '*::-webkit-scrollbar-thumb:hover': { background: '#484f58' },
      },
    },
  },
});
