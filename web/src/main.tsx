import { StrictMode } from 'react';
import { createRoot } from 'react-dom/client';
import { QueryClientProvider } from '@tanstack/react-query';
import { queryClient } from '@/lib/query-client';
import { TransportProvider } from '@/lib/transport';
import { ThemeProvider } from '@/components/theme-provider';
import App from './App';
import './index.css';
import '@/lib/i18n';
import { useTranslation } from 'react-i18next';

// 加载中显示的内容
function LoadingFallback() {
  const { t } = useTranslation();
  return (
    <div
      style={{
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        height: '100vh',
        fontSize: '14px',
        color: '#666',
      }}
    >
      {t('common.loading')}
    </div>
  );
}

// 错误时显示的内容
function ErrorFallback(error: Error) {
  const { t } = useTranslation();
  return (
    <div
      style={{
        display: 'flex',
        flexDirection: 'column',
        alignItems: 'center',
        justifyContent: 'center',
        height: '100vh',
        padding: '20px',
        textAlign: 'center',
      }}
    >
      <h2 style={{ color: '#dc2626', marginBottom: '10px' }}>{t('common.initFailed')}</h2>
      <pre
        style={{
          background: '#f3f4f6',
          padding: '10px 20px',
          borderRadius: '4px',
          maxWidth: '600px',
          overflow: 'auto',
        }}
      >
        {error.message}
      </pre>
    </div>
  );
}

// Render the app
createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <ThemeProvider defaultTheme="system" storageKey="maxx-ui-theme">
      <TransportProvider fallback={<LoadingFallback />} errorFallback={ErrorFallback}>
        <QueryClientProvider client={queryClient}>
          <App />
        </QueryClientProvider>
      </TransportProvider>
    </ThemeProvider>
  </StrictMode>
);
