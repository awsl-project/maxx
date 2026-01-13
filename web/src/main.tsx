import { StrictMode } from 'react';
import { createRoot } from 'react-dom/client';
import { QueryClientProvider } from '@tanstack/react-query';
import { queryClient } from '@/lib/query-client';
import { getTransport } from '@/lib/transport';
import { ThemeProvider } from '@/components/theme-provider';
import App from './App';
import './index.css';

// Initialize transport connection
const transport = getTransport();
transport.connect().catch(console.error);

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <ThemeProvider defaultTheme="light" storageKey="maxx-ui-theme">
      <QueryClientProvider client={queryClient}>
        <App />
      </QueryClientProvider>
    </ThemeProvider>
  </StrictMode>
);
