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
});

export const darkTheme: Theme = createTheme({
  ...baseTheme,
  palette: {
    mode: 'dark',
    background: { default: '#0d1117', paper: '#161b22' },
  },
});
