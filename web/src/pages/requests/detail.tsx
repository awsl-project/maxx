import { useState, useMemo } from 'react';
import { useParams, useNavigate } from 'react-router-dom';
import { Button, Card, CardContent, CardHeader, CardTitle, Badge } from '@/components/ui';
import { useProxyRequest, useProxyUpstreamAttempts, useProxyRequestUpdates, useProviders } from '@/hooks/queries';
import {
  ArrowLeft,
  Clock,
  Zap,
  AlertCircle,
  Server,
  CheckCircle,
  XCircle,
  Loader2,
  Ban,
  Code,
  Database,
  Info
} from 'lucide-react';
import { statusVariant } from './index';
import type { ProxyUpstreamAttempt, ClientType } from '@/lib/transport';
import { cn } from '@/lib/utils';
import { ClientIcon, getClientName, getClientColor } from '@/components/icons/client-icons';

export function RequestDetailPage() {
  const { id } = useParams<{ id: string }>();
  const navigate = useNavigate();
  const { data: request, isLoading, error } = useProxyRequest(Number(id));
  const { data: attempts } = useProxyUpstreamAttempts(Number(id));
  const { data: providers } = useProviders();
  const [selectedAttemptId, setSelectedAttemptId] = useState<number | null>(null);
  const [activeTab, setActiveTab] = useState<'request' | 'response' | 'metadata'>('request');

  useProxyRequestUpdates();

  const selectedAttempt = useMemo(() => {
    if (selectedAttemptId) {
      return attempts?.find(a => a.id === selectedAttemptId);
    }
    return attempts?.[0];
  }, [attempts, selectedAttemptId]);

  // Select the first attempt by default if none selected
  useMemo(() => {
     if (!selectedAttemptId && attempts && attempts.length > 0) {
        setSelectedAttemptId(attempts[0].id);
     }
  }, [attempts, selectedAttemptId]);


  // Create lookup map for provider names
  const providerMap = useMemo(() => {
    const map = new Map<number, string>();
    providers?.forEach(p => map.set(p.id, p.name));
    return map;
  }, [providers]);

  const formatDuration = (ns: number) => {
    const ms = ns / 1_000_000;
    if (ms < 1000) return `${ms.toFixed(0)}ms`;
    return `${(ms / 1000).toFixed(2)}s`;
  };

  const formatTime = (dateStr: string) => {
    if (!dateStr || dateStr === '0001-01-01T00:00:00Z') return '-';
    return new Date(dateStr).toLocaleString();
  };

  const formatJSON = (obj: unknown): string => {
    if (!obj) return '-';
    try {
      return JSON.stringify(obj, null, 2);
    } catch {
      return String(obj);
    }
  };

  const getStatusIcon = (status: string) => {
    switch (status) {
      case 'COMPLETED':
        return <CheckCircle className="h-4 w-4 text-success" />;
      case 'FAILED':
        return <XCircle className="h-4 w-4 text-error" />;
      case 'CANCELLED':
        return <Ban className="h-4 w-4 text-warning" />;
      case 'IN_PROGRESS':
        return <Loader2 className="h-4 w-4 text-info animate-spin" />;
      default:
        return <Clock className="h-4 w-4 text-text-muted" />;
    }
  };

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
        <div className="p-4 bg-error/10 rounded-full">
          <AlertCircle className="h-12 w-12 text-error" />
        </div>
        <h3 className="text-lg font-semibold text-text-primary">Request Not Found</h3>
        <p className="text-text-secondary">The request doesn't exist or has been deleted.</p>
        <Button variant="outline" onClick={() => navigate('/requests')}>
          <ArrowLeft className="mr-2 h-4 w-4" />
          Back to Requests
        </Button>
      </div>
    );
  }

  return (
    <div className="flex flex-col h-full overflow-hidden bg-background">
      {/* Header */}
      <div className="border-b border-border bg-surface-primary flex-shrink-0 px-4 py-4">
        <div className="flex items-start justify-between gap-6">
          {/* Left: Back + Main Info */}
          <div className="flex items-start gap-3 min-w-0">
            <Button variant="ghost" size="icon" onClick={() => navigate('/requests')} className="h-8 w-8 -ml-2 mt-0.5 text-text-secondary hover:text-text-primary flex-shrink-0">
              <ArrowLeft className="h-5 w-5" />
            </Button>
            <div
              className="w-10 h-10 rounded-lg flex items-center justify-center flex-shrink-0"
              style={{ backgroundColor: `${getClientColor(request.clientType as ClientType)}15` }}
            >
              <ClientIcon type={request.clientType as ClientType} size={24} />
            </div>
            <div className="min-w-0 flex-1">
              <div className="flex items-center gap-2 flex-wrap">
                <h2 className="text-lg font-semibold text-text-primary tracking-tight">
                  {request.requestModel || 'Unknown Model'}
                </h2>
                <Badge variant={statusVariant[request.status]} className="capitalize">
                  {request.status.toLowerCase().replace('_', ' ')}
                </Badge>
              </div>
              <div className="flex items-center gap-3 mt-1 text-xs text-text-muted">
                <span className="font-mono bg-surface-secondary px-1.5 py-0.5 rounded">#{request.id}</span>
                <span>{getClientName(request.clientType as ClientType)}</span>
                <span>·</span>
                <span>{formatTime(request.startTime)}</span>
                {request.responseModel && request.responseModel !== request.requestModel && (
                  <>
                    <span>·</span>
                    <span className="text-text-secondary">Response: <span className="text-text-primary">{request.responseModel}</span></span>
                  </>
                )}
              </div>
            </div>
          </div>

          {/* Right: Stats Grid */}
          <div className="flex items-center gap-4 flex-shrink-0">
            <div className="text-center px-3">
              <div className="text-[10px] uppercase tracking-wider text-text-muted mb-0.5">Duration</div>
              <div className="text-sm font-mono font-medium text-text-primary">
                {request.duration ? formatDuration(request.duration) : '-'}
              </div>
            </div>
            <div className="w-px h-8 bg-border" />
            <div className="text-center px-3">
              <div className="text-[10px] uppercase tracking-wider text-text-muted mb-0.5">Input</div>
              <div className="text-sm font-mono font-medium text-text-secondary">
                {request.inputTokenCount > 0 ? request.inputTokenCount.toLocaleString() : '-'}
              </div>
            </div>
            <div className="w-px h-8 bg-border" />
            <div className="text-center px-3">
              <div className="text-[10px] uppercase tracking-wider text-text-muted mb-0.5">Output</div>
              <div className="text-sm font-mono font-medium text-text-primary">
                {request.outputTokenCount > 0 ? request.outputTokenCount.toLocaleString() : '-'}
              </div>
            </div>
            <div className="w-px h-8 bg-border" />
            <div className="text-center px-3">
              <div className="text-[10px] uppercase tracking-wider text-text-muted mb-0.5">Cache Read</div>
              <div className="text-sm font-mono font-medium text-violet-400">
                {request.cacheReadCount > 0 ? request.cacheReadCount.toLocaleString() : '-'}
              </div>
            </div>
            <div className="w-px h-8 bg-border" />
            <div className="text-center px-3">
              <div className="text-[10px] uppercase tracking-wider text-text-muted mb-0.5">Cache Write</div>
              <div className="text-sm font-mono font-medium text-amber-400">
                {request.cacheWriteCount > 0 ? request.cacheWriteCount.toLocaleString() : '-'}
              </div>
            </div>
            <div className="w-px h-8 bg-border" />
            <div className="text-center px-3">
              <div className="text-[10px] uppercase tracking-wider text-text-muted mb-0.5">Cost</div>
              <div className="text-sm font-mono font-medium text-success">
                {request.cost > 0 ? `$${request.cost.toFixed(4)}` : '-'}
              </div>
            </div>
          </div>
        </div>
      </div>

       {/* Error Banner */}
       {request.error && (
        <div className="flex-shrink-0 bg-error/10 border-b border-error/20 px-6 py-3 flex items-start gap-3">
           <AlertCircle className="mt-0.5 h-4 w-4 shrink-0 text-error" />
           <div className="flex-1">
             <h4 className="text-sm font-medium text-error mb-1">Request Failed</h4>
             <pre className="whitespace-pre-wrap break-words font-mono text-xs text-error/90 max-h-24 overflow-auto">
               {request.error}
             </pre>
           </div>
        </div>
      )}

      {/* Main Content - Split View */}
      <div className="flex-1 flex overflow-hidden">
         {/* Sidebar: Attempts List */}
         <div className="w-80 flex flex-col border-r border-border bg-surface-primary flex-shrink-0">
            <div className="p-3 border-b border-border bg-surface-secondary/50 flex items-center justify-between">
               <span className="text-xs font-semibold text-text-secondary uppercase tracking-wider flex items-center gap-2">
                 <Server size={12} /> Attempts
               </span>
               <Badge variant="outline" className="text-[10px] h-5 px-1.5">{attempts?.length || 0}</Badge>
            </div>
            
            <div className="flex-1 overflow-y-auto">
               {attempts && attempts.length > 0 ? (
                  <div className="divide-y divide-border">
                    {attempts.map((attempt: ProxyUpstreamAttempt, index: number) => (
                      <button
                        key={attempt.id}
                        onClick={() => setSelectedAttemptId(attempt.id)}
                        className={cn(
                          "w-full text-left p-3.5 transition-all outline-none border-l-[3px]",
                          selectedAttempt?.id === attempt.id 
                            ? "bg-accent/5 border-l-accent" 
                            : "border-l-transparent hover:bg-surface-secondary/50"
                        )}
                      >
                        <div className="flex items-center justify-between mb-1.5">
                          <div className="flex items-center gap-2">
                            {getStatusIcon(attempt.status)}
                            <span className={cn("text-sm font-medium transition-colors", selectedAttempt?.id === attempt.id ? "text-text-primary" : "text-text-secondary group-hover:text-text-primary")}>
                              Attempt {index + 1}
                            </span>
                          </div>
                          {attempt.responseInfo && (
                            <span className={cn(
                              "text-[10px] font-mono px-1.5 py-0.5 rounded font-medium",
                              attempt.responseInfo.status >= 400 ? "text-error bg-error/10" : "text-success bg-success/10"
                            )}>
                              {attempt.responseInfo.status}
                            </span>
                          )}
                        </div>
                        <div className="flex items-center justify-between text-xs text-text-muted">
                          <span className="flex items-center gap-1.5 truncate max-w-[140px]" title={providerMap.get(attempt.providerID) || `Provider #${attempt.providerID}`}>
                            {providerMap.get(attempt.providerID) || `Provider #${attempt.providerID}`}
                          </span>
                           <span className="font-mono opacity-70">
                            {formatDuration((new Date(attempt.updatedAt).getTime() - new Date(attempt.createdAt).getTime()) * 1_000_000)}
                          </span>
                        </div>
                      </button>
                    ))}
                  </div>
                ) : (
                  <div className="flex flex-col items-center justify-center h-40 text-text-muted p-6 text-center">
                    <Server className="h-8 w-8 mb-2 opacity-20" />
                    <p className="text-xs">No attempts</p>
                  </div>
                )}
            </div>
         </div>

         {/* Main Panel: Selected Attempt Detail */}
         <div className="flex-1 flex flex-col bg-background min-w-0">
            {selectedAttempt ? (
               <>
                 {/* Detail Header */}
                 <div className="border-b border-border bg-surface-secondary/20 px-6 py-3 flex items-center justify-between flex-shrink-0 backdrop-blur-sm sticky top-0 z-10">
                    <div className="flex items-center gap-4">
                      <div className="w-10 h-10 rounded-lg bg-surface-primary flex items-center justify-center text-text-primary shadow-sm border border-border">
                         <Server size={20} />
                      </div>
                      <div>
                        <h3 className="text-sm font-medium text-text-primary">
                          {providerMap.get(selectedAttempt.providerID) || `Provider #${selectedAttempt.providerID}`}
                        </h3>
                         <div className="flex items-center gap-3 text-xs text-text-secondary mt-0.5">
                           {selectedAttempt.routeID > 0 && <span>Route ID: {selectedAttempt.routeID}</span>}
                           {selectedAttempt.cost > 0 && <span className="text-success font-medium">Cost: ${selectedAttempt.cost.toFixed(6)}</span>}
                         </div>
                      </div>
                    </div>
                    
                    {/* Detail Tabs */}
                     <div className="flex bg-surface-secondary/50 p-1 rounded-lg border border-border">
                      <button
                        onClick={() => setActiveTab('request')}
                        className={cn(
                          "px-4 py-1.5 text-xs font-medium rounded-md transition-all",
                          activeTab === 'request'
                            ? "bg-surface-primary text-text-primary shadow-sm ring-1 ring-border/50"
                            : "text-text-secondary hover:text-text-primary hover:bg-surface-hover/50"
                        )}
                      >
                        Request
                      </button>
                      <button
                        onClick={() => setActiveTab('response')}
                        className={cn(
                          "px-4 py-1.5 text-xs font-medium rounded-md transition-all",
                          activeTab === 'response'
                            ? "bg-surface-primary text-text-primary shadow-sm ring-1 ring-border/50"
                            : "text-text-secondary hover:text-text-primary hover:bg-surface-hover/50"
                        )}
                      >
                        Response
                      </button>
                       <button
                        onClick={() => setActiveTab('metadata')}
                        className={cn(
                          "px-4 py-1.5 text-xs font-medium rounded-md transition-all",
                          activeTab === 'metadata'
                            ? "bg-surface-primary text-text-primary shadow-sm ring-1 ring-border/50"
                            : "text-text-secondary hover:text-text-primary hover:bg-surface-hover/50"
                        )}
                      >
                        Metadata
                      </button>
                    </div>
                 </div>

                 {/* Detail Content */}
                 <div className="flex-1 overflow-hidden flex flex-col bg-background relative">
                    {activeTab === 'request' && (
                      selectedAttempt.requestInfo ? (
                        <div className="flex-1 flex flex-col overflow-hidden p-6 gap-6 animate-fade-in">
                          <div className="flex items-center gap-3 p-3 bg-surface-secondary/30 rounded-lg border border-border flex-shrink-0">
                            <Badge variant="info" className="font-mono text-xs">{selectedAttempt.requestInfo.method}</Badge>
                            <code className="flex-1 font-mono text-xs text-text-primary break-all">
                              {selectedAttempt.requestInfo.url}
                            </code>
                          </div>
                          
                          <div className="flex flex-col min-h-0 flex-1 gap-6">
                            <div className="flex flex-col flex-1 min-h-0 gap-3">
                              <h5 className="text-xs font-semibold text-text-secondary uppercase tracking-wider flex items-center gap-2 flex-shrink-0">
                                <Code size={14} /> Headers
                              </h5>
                              <div className="flex-1 rounded-lg border border-border bg-[#1a1a1a] p-4 overflow-auto shadow-inner relative group">
                                <div className="absolute top-2 right-2 opacity-0 group-hover:opacity-100 transition-opacity">
                                  <Badge variant="outline" className="text-[10px] bg-surface-primary/80 backdrop-blur-sm">JSON</Badge>
                                </div>
                                 <pre className="text-xs font-mono text-text-secondary leading-relaxed">
                                  {formatJSON(selectedAttempt.requestInfo.headers)}
                                </pre>
                              </div>
                            </div>

                            {selectedAttempt.requestInfo.body && (
                              <div className="flex flex-col flex-[2] min-h-0 gap-3">
                                <h5 className="text-xs font-semibold text-text-secondary uppercase tracking-wider flex items-center gap-2 flex-shrink-0">
                                  <Database size={14} /> Body
                                </h5>
                                <div className="flex-1 rounded-lg border border-border bg-[#1a1a1a] p-4 overflow-auto shadow-inner relative group">
                                  <div className="absolute top-2 right-2 opacity-0 group-hover:opacity-100 transition-opacity">
                                    <Badge variant="outline" className="text-[10px] bg-surface-primary/80 backdrop-blur-sm">JSON</Badge>
                                  </div>
                                  <pre className="text-xs font-mono text-text-primary whitespace-pre-wrap leading-relaxed">
                                    {(() => {
                                      try {
                                        return formatJSON(JSON.parse(selectedAttempt.requestInfo.body));
                                      } catch {
                                        return selectedAttempt.requestInfo.body;
                                      }
                                    })()}
                                  </pre>
                                </div>
                              </div>
                            )}
                          </div>
                        </div>
                      ) : (
                         <EmptyState message="No request data available" />
                      )
                    )}

                    {activeTab === 'response' && (
                       selectedAttempt.responseInfo ? (
                        <div className="flex-1 flex flex-col overflow-hidden p-6 gap-6 animate-fade-in">
                           <div className="flex items-center gap-3 p-3 bg-surface-secondary/30 rounded-lg border border-border flex-shrink-0">
                            <div className={cn(
                              "px-2 py-1 rounded text-xs font-bold font-mono",
                              selectedAttempt.responseInfo.status >= 400 ? "bg-error/10 text-error" : "bg-success/10 text-success"
                            )}>
                              {selectedAttempt.responseInfo.status}
                            </div>
                            <span className="text-sm text-text-secondary font-medium">Response Status</span>
                          </div>

                          <div className="flex flex-col min-h-0 flex-1 gap-6">
                            <div className="flex flex-col flex-1 min-h-0 gap-3">
                              <h5 className="text-xs font-semibold text-text-secondary uppercase tracking-wider flex items-center gap-2 flex-shrink-0">
                                <Code size={14} /> Headers
                              </h5>
                              <div className="flex-1 rounded-lg border border-border bg-[#1a1a1a] p-4 overflow-auto shadow-inner relative group">
                                <div className="absolute top-2 right-2 opacity-0 group-hover:opacity-100 transition-opacity">
                                  <Badge variant="outline" className="text-[10px] bg-surface-primary/80 backdrop-blur-sm">JSON</Badge>
                                </div>
                                <pre className="text-xs font-mono text-text-secondary leading-relaxed">
                                  {formatJSON(selectedAttempt.responseInfo.headers)}
                                </pre>
                              </div>
                            </div>

                            {selectedAttempt.responseInfo.body && (
                              <div className="flex flex-col flex-[2] min-h-0 gap-3">
                                <h5 className="text-xs font-semibold text-text-secondary uppercase tracking-wider flex items-center gap-2 flex-shrink-0">
                                  <Database size={14} /> Body
                                </h5>
                                <div className="flex-1 rounded-lg border border-border bg-[#1a1a1a] p-4 overflow-auto shadow-inner relative group">
                                  <div className="absolute top-2 right-2 opacity-0 group-hover:opacity-100 transition-opacity">
                                    <Badge variant="outline" className="text-[10px] bg-surface-primary/80 backdrop-blur-sm">JSON</Badge>
                                  </div>
                                  <pre className="text-xs font-mono text-text-primary whitespace-pre-wrap leading-relaxed">
                                    {(() => {
                                      try {
                                        return formatJSON(JSON.parse(selectedAttempt.responseInfo.body));
                                      } catch {
                                        return selectedAttempt.responseInfo.body;
                                      }
                                    })()}
                                  </pre>
                                </div>
                              </div>
                            )}
                          </div>
                        </div>
                       ) : (
                        <EmptyState message="No response data available" />
                       )
                    )}

                    {activeTab === 'metadata' && (
                        <div className="flex-1 overflow-y-auto p-6 animate-fade-in">
                             <div className="grid grid-cols-1 xl:grid-cols-2 gap-6">
                                <Card className="bg-surface-primary border-border">
                                   <CardHeader className="pb-2 border-b border-border/50">
                                      <CardTitle className="text-sm font-medium flex items-center gap-2">
                                         <Info size={16} className="text-info"/> Request Info
                                      </CardTitle>
                                   </CardHeader>
                                   <CardContent className="pt-4 space-y-4">
                                       <div className="grid grid-cols-1 sm:grid-cols-3 gap-4 items-center">
                                          <dt className="text-xs font-medium text-text-secondary uppercase tracking-wider">Request ID</dt>
                                          <dd className="sm:col-span-2 font-mono text-xs text-text-primary bg-surface-secondary px-2 py-1 rounded select-all break-all">{request.requestID || '-'}</dd>
                                       </div>
                                       <div className="grid grid-cols-1 sm:grid-cols-3 gap-4 items-center">
                                          <dt className="text-xs font-medium text-text-secondary uppercase tracking-wider">Session ID</dt>
                                          <dd className="sm:col-span-2 font-mono text-xs text-text-primary bg-surface-secondary px-2 py-1 rounded select-all break-all">{request.sessionID || '-'}</dd>
                                       </div>
                                       <div className="grid grid-cols-1 sm:grid-cols-3 gap-4 items-center">
                                          <dt className="text-xs font-medium text-text-secondary uppercase tracking-wider">Instance ID</dt>
                                          <dd className="sm:col-span-2 font-mono text-xs text-text-primary bg-surface-secondary px-2 py-1 rounded select-all break-all">{request.instanceID || '-'}</dd>
                                       </div>
                                       <div className="grid grid-cols-1 sm:grid-cols-3 gap-4 items-center">
                                          <dt className="text-xs font-medium text-text-secondary uppercase tracking-wider">Request Model</dt>
                                          <dd className="sm:col-span-2 font-mono text-xs text-text-primary bg-surface-secondary px-2 py-1 rounded">{request.requestModel || '-'}</dd>
                                       </div>
                                       <div className="grid grid-cols-1 sm:grid-cols-3 gap-4 items-center">
                                          <dt className="text-xs font-medium text-text-secondary uppercase tracking-wider">Response Model</dt>
                                          <dd className="sm:col-span-2 font-mono text-xs text-text-primary bg-surface-secondary px-2 py-1 rounded">{request.responseModel || '-'}</dd>
                                       </div>
                                   </CardContent>
                                </Card>

                                <Card className="bg-surface-primary border-border">
                                   <CardHeader className="pb-2 border-b border-border/50">
                                      <CardTitle className="text-sm font-medium flex items-center gap-2">
                                         <Zap size={16} className="text-warning"/> Attempt Usage & Cache
                                      </CardTitle>
                                   </CardHeader>
                                   <CardContent className="pt-4 space-y-4">
                                       <div className="flex justify-between items-center border-b border-border/30 pb-2">
                                          <dt className="text-xs font-medium text-text-secondary uppercase tracking-wider">Input Tokens</dt>
                                          <dd className="text-sm text-text-primary font-mono font-medium">{selectedAttempt.inputTokenCount.toLocaleString()}</dd>
                                       </div>
                                       <div className="flex justify-between items-center border-b border-border/30 pb-2">
                                          <dt className="text-xs font-medium text-text-secondary uppercase tracking-wider">Output Tokens</dt>
                                          <dd className="text-sm text-text-primary font-mono font-medium">{selectedAttempt.outputTokenCount.toLocaleString()}</dd>
                                       </div>
                                       <div className="flex justify-between items-center border-b border-border/30 pb-2">
                                          <dt className="text-xs font-medium text-text-secondary uppercase tracking-wider">Cache Read</dt>
                                          <dd className="text-sm text-violet-400 font-mono font-medium">{selectedAttempt.cacheReadCount.toLocaleString()}</dd>
                                       </div>
                                       <div className="flex justify-between items-center border-b border-border/30 pb-2">
                                          <dt className="text-xs font-medium text-text-secondary uppercase tracking-wider">Cache Write</dt>
                                          <dd className="text-sm text-amber-400 font-mono font-medium">{selectedAttempt.cacheWriteCount.toLocaleString()}</dd>
                                       </div>
                                       {(selectedAttempt.cache5mWriteCount > 0 || selectedAttempt.cache1hWriteCount > 0) && (
                                         <div className="flex justify-between items-center border-b border-border/30 pb-2 pl-4">
                                           <dt className="text-xs font-medium text-text-secondary/70 tracking-wider">
                                             <span className="text-cyan-400/80">5m:</span> {selectedAttempt.cache5mWriteCount}
                                             <span className="mx-2">|</span>
                                             <span className="text-orange-400/80">1h:</span> {selectedAttempt.cache1hWriteCount}
                                           </dt>
                                         </div>
                                       )}
                                       <div className="flex justify-between items-center">
                                          <dt className="text-xs font-medium text-text-secondary uppercase tracking-wider">Cost</dt>
                                          <dd className="text-sm text-success font-mono font-medium">${selectedAttempt.cost.toFixed(6)}</dd>
                                       </div>
                                   </CardContent>
                                </Card>
                             </div>
                        </div>
                    )}
                 </div>
               </>
            ) : (
               <EmptyState message="Select an attempt to view details" icon={<Server className="h-12 w-12 mb-4 opacity-10" />} />
            )}
         </div>
      </div>
    </div>
  );
}

function EmptyState({ message, icon }: { message: string, icon?: React.ReactNode }) {
  return (
    <div className="flex flex-col items-center justify-center h-full text-text-muted p-12 text-center select-none">
      {icon || <Server className="h-12 w-12 mb-3 opacity-10" />}
      <p className="text-sm font-medium">{message}</p>
    </div>
  );
}
