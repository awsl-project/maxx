import { useState, useMemo, useEffect, useCallback } from 'react';
import { useParams, useNavigate } from 'react-router-dom';
import { AlertCircle, Loader2 } from 'lucide-react';
import { useMutation, useQueryClient } from '@tanstack/react-query';
import { useTransport } from '@/lib/transport';
import {
  useProxyRequest,
  useProxyUpstreamAttempts,
  useProxyRequestUpdates,
  useProviders,
  useProjects,
  useSessions,
  useRoutes,
  useAPITokens,
  requestKeys,
} from '@/hooks/queries';
import { ResizableHandle, ResizablePanel, ResizablePanelGroup } from '@/components/ui/resizable';
import { RequestHeader } from './detail/RequestHeader';
import { RequestSidebar } from './detail/RequestSidebar';
import { RequestDetailPanel } from './detail/RequestDetailPanel';

// Selection type: either the main request or an attempt
type SelectionType = { type: 'request' } | { type: 'attempt'; attemptId: number };

export function RequestDetailPage() {
  const { id } = useParams<{ id: string }>();
  const navigate = useNavigate();
  const { transport } = useTransport();
  const queryClient = useQueryClient();
  const { data: request, isLoading, error } = useProxyRequest(Number(id));
  const { data: attempts } = useProxyUpstreamAttempts(Number(id));
  const { data: providers } = useProviders();
  const { data: projects } = useProjects();
  const { data: sessions } = useSessions();
  const { data: routes } = useRoutes();
  const { data: apiTokens } = useAPITokens();
  const [selection, setSelection] = useState<SelectionType>({
    type: 'request',
  });
  const [activeTab, setActiveTab] = useState<'request' | 'response' | 'metadata'>('request');

  // Recalculate cost mutation
  const recalculateMutation = useMutation({
    mutationFn: () => transport.recalculateRequestCost(Number(id)),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: requestKeys.detail(Number(id)) });
    },
  });

  const handleRecalculateCost = useCallback(() => {
    recalculateMutation.mutate();
  }, [recalculateMutation]);

  useProxyRequestUpdates();

  // ESC 键返回列表
  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key === 'Escape') {
        navigate('/requests');
      }
    };
    window.addEventListener('keydown', handleKeyDown);
    return () => window.removeEventListener('keydown', handleKeyDown);
  }, [navigate]);

  // Create lookup map for provider names
  const providerMap = useMemo(() => {
    const map = new Map<number, string>();
    providers?.forEach((p) => {
      map.set(p.id, p.name);
    });
    return map;
  }, [providers]);

  // Create lookup map for project names
  const projectMap = useMemo(() => {
    const map = new Map<number, string>();
    projects?.forEach((p) => {
      map.set(p.id, p.name);
    });
    return map;
  }, [projects]);

  // Create lookup map for sessions by sessionID
  const sessionMap = useMemo(() => {
    const map = new Map<string, { clientType: string; projectID: number }>();
    sessions?.forEach((s) => {
      map.set(s.sessionID, {
        clientType: s.clientType,
        projectID: s.projectID,
      });
    });
    return map;
  }, [sessions]);

  // Create lookup map for routes by routeID
  const routeMap = useMemo(() => {
    const map = new Map<number, { projectID: number }>();
    routes?.forEach((r) => {
      map.set(r.id, { projectID: r.projectID });
    });
    return map;
  }, [routes]);

  // Create lookup map for API Token names
  const tokenMap = useMemo(() => {
    const map = new Map<number, string>();
    apiTokens?.forEach((t) => {
      map.set(t.id, t.name);
    });
    return map;
  }, [apiTokens]);

  if (isLoading) {
    return (
      <div className="flex h-full items-center justify-center bg-background">
        <Loader2 className="h-8 w-8 animate-spin text-accent" />
      </div>
    );
  }

  if (error || !request) {
    return (
      <div className="flex flex-col items-center justify-center h-full space-y-4 bg-background">
        <div className="p-4 bg-red-400/10 rounded-full">
          <AlertCircle className="h-12 w-12 text-red-400" />
        </div>
        <h3 className="text-lg font-semibold text-foreground">Request Not Found</h3>
        <p className="text-sm text-muted-foreground">
          The request you're looking for doesn't exist or has been deleted.
        </p>
      </div>
    );
  }

  return (
    <div className="flex flex-col h-screen overflow-hidden bg-background w-full ">
      {/* Header */}
      <RequestHeader
        request={request}
        onBack={() => navigate('/requests')}
        onRecalculateCost={handleRecalculateCost}
        isRecalculating={recalculateMutation.isPending}
      />

      {/* Error Banner */}
      {request.error && (
        <div className="shrink-0 bg-red-400/10 border-b border-red-400/20 px-6 py-3 flex items-start gap-3">
          <AlertCircle className="mt-0.5 h-4 w-4 shrink-0 text-red-400" />
          <div className="flex-1">
            <h4 className="text-sm font-medium text-red-400 mb-1">Request Failed</h4>
            <pre className="whitespace-pre-wrap wrap-break-words font-mono text-xs text-red-400/90 max-h-24 overflow-auto">
              {request.error}
            </pre>
          </div>
        </div>
      )}

      {/* Main Content - Resizable Split View */}
      <div className="flex-1 overflow-hidden">
        <ResizablePanelGroup direction="horizontal" id="request-detail-layout">
          {/* Left Panel: Sidebar */}
          <ResizablePanel defaultSize={20} minSize={20} maxSize={50}>
            <RequestSidebar
              request={request}
              attempts={attempts}
              selection={selection}
              onSelectionChange={setSelection}
              providerMap={providerMap}
              projectMap={projectMap}
              routeMap={routeMap}
            />
          </ResizablePanel>

          {/* Resizable Handle */}
          <ResizableHandle withHandle />

          {/* Right Panel: Detail View */}
          <ResizablePanel defaultSize={80} minSize={50}>
            <RequestDetailPanel
              request={request}
              selection={selection}
              attempts={attempts}
              activeTab={activeTab}
              setActiveTab={setActiveTab}
              providerMap={providerMap}
              projectMap={projectMap}
              sessionMap={sessionMap}
              tokenMap={tokenMap}
            />
          </ResizablePanel>
        </ResizablePanelGroup>
      </div>
    </div>
  );
}
