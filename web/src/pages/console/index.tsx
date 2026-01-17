import { useState, useEffect, useRef } from 'react';
import { useTranslation } from 'react-i18next';
import { Terminal, Trash2, Pause, Play, ArrowDown } from 'lucide-react';
import { getTransport } from '@/lib/transport';
import { Button } from '@/components/ui';
import { PageHeader } from '@/components/layout/page-header';

export function ConsolePage() {
  const { t } = useTranslation();
  const [logs, setLogs] = useState<string[]>([]);
  const [isPaused, setIsPaused] = useState(false);
  const [autoScroll, setAutoScroll] = useState(true);
  const logsEndRef = useRef<HTMLDivElement>(null);
  const containerRef = useRef<HTMLDivElement>(null);
  const pausedRef = useRef(isPaused);

  // Keep pausedRef in sync
  useEffect(() => {
    pausedRef.current = isPaused;
  }, [isPaused]);

  // Subscribe to log_message events (only real-time logs from this session)
  useEffect(() => {
    const transport = getTransport();
    const unsubscribe = transport.subscribe<string>('log_message', (message) => {
      if (pausedRef.current) return;
      setLogs((prev) => [...prev.slice(-999), message]);
    });

    return () => unsubscribe();
  }, []);

  // Auto-scroll to bottom
  useEffect(() => {
    if (autoScroll && logsEndRef.current) {
      logsEndRef.current.scrollIntoView({ behavior: 'smooth' });
    }
  }, [logs, autoScroll]);

  const handleScroll = () => {
    if (!containerRef.current) return;
    const { scrollTop, scrollHeight, clientHeight } = containerRef.current;
    const isAtBottom = scrollHeight - scrollTop - clientHeight < 50;
    setAutoScroll(isAtBottom);
  };

  const clearLogs = () => {
    setLogs([]);
  };

  const scrollToBottom = () => {
    logsEndRef.current?.scrollIntoView({ behavior: 'smooth' });
    setAutoScroll(true);
  };

  return (
    <div className="flex flex-col h-full">
      <PageHeader
        icon={Terminal}
        iconClassName="text-slate-500"
        title={t('console.title')}
        description={t('console.description', { count: logs.length })}
      >
        <Button
          onClick={() => setIsPaused(!isPaused)}
          variant="outline"
          className={`h-9 px-4 lg:px-6 gap-2 transition-all duration-300 font-medium ${
            isPaused
              ? 'bg-amber-500/10 text-amber-600 dark:text-amber-400 border-amber-500/20 hover:bg-amber-500/20 hover:border-amber-500/40'
              : 'hover:bg-accent hover:text-accent-foreground border-border/50'
          }`}
        >
          {isPaused ? (
            <Play size={16} className="fill-current" />
          ) : (
            <Pause size={16} className="fill-current" />
          )}
          {isPaused ? t('console.resume') : t('console.pause')}
        </Button>
        <Button
          onClick={clearLogs}
          variant="outline"
          className="h-9 px-4 gap-2 text-muted-foreground hover:text-red-600 hover:bg-red-50 dark:hover:bg-red-950/30 hover:border-red-200 dark:hover:border-red-900 border-border/50 transition-all duration-300"
        >
          <Trash2 size={16} />
          <span className="hidden sm:inline">{t('console.clear')}</span>
        </Button>
      </PageHeader>

      <div
        ref={containerRef}
        onScroll={handleScroll}
        className="flex-1 overflow-y-auto bg-muted/30 font-mono text-xs md:text-sm scroll-smooth"
      >
        {logs.length === 0 ? (
          <EmptyState />
        ) : (
          <div className="p-4 space-y-0.5 min-h-full">
            {logs.map((log, index) => (
              <div
                key={index}
                className="text-xs px-3 py-1.5 rounded-md hover:bg-accent text-foreground transition-colors wrap-break-words whitespace-pre-wrap"
              >
                <div className="opacity-50 select-none inline-block w-8 mr-2 text-right">
                  {index + 1}
                </div>
                {log}
              </div>
            ))}
            <div ref={logsEndRef} className="h-4" />
          </div>
        )}
      </div>

      {!autoScroll && (
        <Button
          onClick={scrollToBottom}
          className="absolute bottom-8 right-8 p-3 bg-primary text-primary-foreground rounded-full shadow-xl hover:bg-primary/90 hover:scale-105 active:scale-95 transition-all z-50 animate-in fade-in zoom-in duration-200"
        >
          <ArrowDown size={20} />
        </Button>
      )}
    </div>
  );
}

function EmptyState() {
  const { t } = useTranslation();
  return (
    <div className="flex flex-col items-center justify-center h-full min-h-[400px] text-muted-foreground/50">
      <div className="w-20 h-20 rounded-full bg-muted flex items-center justify-center mb-6 ring-8 ring-muted/50">
        <Terminal size={40} className="opacity-50" />
      </div>
      <p className="text-lg font-medium text-foreground/80">{t('console.waitingForLogs')}</p>
      <p className="text-sm mt-2 max-w-screen-sm text-center leading-relaxed">
        {t('console.waitingForLogsHint')}
      </p>
    </div>
  );
}

export default ConsolePage;
