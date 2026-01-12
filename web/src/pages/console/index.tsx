import { useState, useEffect, useRef } from 'react';
import { Terminal, Trash2, Pause, Play, ArrowDown } from 'lucide-react';
import { getTransport } from '@/lib/transport';

const transport = getTransport();

export function ConsolePage() {
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
      <Header
        isPaused={isPaused}
        onTogglePause={() => setIsPaused(!isPaused)}
        onClear={clearLogs}
        logCount={logs.length}
      />

      <div
        ref={containerRef}
        onScroll={handleScroll}
        className="flex-1 overflow-y-auto bg-[#1a1a1a] font-mono text-sm"
      >
        {logs.length === 0 ? (
          <EmptyState />
        ) : (
          <div className="p-4">
            {logs.map((log, index) => (
              <div key={index} className="text-gray-300 py-0.5 hover:bg-black/5 dark:hover:bg-white/5">
                {log}
              </div>
            ))}
            <div ref={logsEndRef} />
          </div>
        )}
      </div>

      {!autoScroll && (
        <button
          onClick={scrollToBottom}
          className="absolute bottom-6 right-6 p-2 bg-accent text-white rounded-full shadow-lg hover:bg-accent-hover"
        >
          <ArrowDown size={20} />
        </button>
      )}
    </div>
  );
}

function Header({
  isPaused,
  onTogglePause,
  onClear,
  logCount,
}: {
  isPaused: boolean;
  onTogglePause: () => void;
  onClear: () => void;
  logCount: number;
}) {
  return (
    <div className="h-[73px] flex items-center justify-between p-lg border-b border-border bg-surface-primary">
      <div className="flex items-center gap-md">
        <div className="w-10 h-10 rounded-lg bg-emerald-400/10 flex items-center justify-center">
          <Terminal size={20} className="text-emerald-400" />
        </div>
        <div>
          <h1 className="text-headline font-semibold text-text-primary">Console</h1>
          <p className="text-caption text-text-secondary">{logCount} lines</p>
        </div>
      </div>
      <div className="flex items-center gap-2">
        <button
          onClick={onTogglePause}
          className={`btn flex items-center gap-2 ${isPaused ? 'bg-amber-500/20 text-amber-400' : 'bg-surface-secondary text-text-primary hover:bg-surface-hover'}`}
        >
          {isPaused ? <Play size={14} /> : <Pause size={14} />}
          {isPaused ? 'Resume' : 'Pause'}
        </button>
        <button
          onClick={onClear}
          className="btn bg-surface-secondary hover:bg-surface-hover text-text-primary flex items-center gap-2"
        >
          <Trash2 size={14} />
          Clear
        </button>
      </div>
    </div>
  );
}

function EmptyState() {
  return (
    <div className="flex flex-col items-center justify-center h-full text-text-muted">
      <Terminal size={48} className="mb-4 opacity-30" />
      <p>Waiting for logs...</p>
      <p className="text-xs mt-1">Logs will appear here in real-time</p>
    </div>
  );
}

export default ConsolePage;
