import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui';
import { useProviders, useRoutes, useProjects, useProxyRequests } from '@/hooks/queries';
import { Activity, Server, Route, FolderKanban, Zap, ArrowRight, CheckCircle, XCircle, Ban, LayoutDashboard } from 'lucide-react';
import { Link } from 'react-router-dom';

export function OverviewPage() {
  const { data: providers } = useProviders();
  const { data: routes } = useRoutes();
  const { data: projects } = useProjects();
  const { data: requestsData } = useProxyRequests({ limit: 10 });

  const requests = requestsData?.items ?? [];

  const stats = [
    { label: 'Providers', value: providers?.length ?? 0, icon: Server, color: 'text-info', href: '/providers' },
    { label: 'Routes', value: routes?.length ?? 0, icon: Route, color: 'text-accent', href: '/routes/claude' },
    { label: 'Projects', value: projects?.length ?? 0, icon: FolderKanban, color: 'text-warning', href: '/projects' },
    { label: 'Recent Requests', value: requests.length, icon: Activity, color: 'text-success', href: '/requests' },
  ];

  const completedRequests = requests.filter((r) => r.status === 'COMPLETED').length;
  const failedRequests = requests.filter((r) => r.status === 'FAILED').length;
  const cancelledRequests = requests.filter((r) => r.status === 'CANCELLED').length;
  const hasProviders = (providers?.length ?? 0) > 0;

  return (
    <div className="flex flex-col h-full bg-background">
      {/* Header */}
      <div className="h-[73px] flex items-center justify-between px-6 border-b border-border bg-surface-primary flex-shrink-0">
        <div className="flex items-center gap-3">
          <div className="p-2 bg-accent/10 rounded-lg">
             <LayoutDashboard size={20} className="text-accent" />
          </div>
          <div>
            <h2 className="text-lg font-semibold text-text-primary leading-tight">Dashboard</h2>
            <p className="text-xs text-text-secondary">Overview of your proxy gateway</p>
          </div>
        </div>
      </div>

      {/* Content */}
      <div className="flex-1 overflow-y-auto p-6">
        <div className="space-y-6 animate-fade-in max-w-7xl mx-auto">
      {/* Welcome Section */}
      {!hasProviders && (
        <div className="text-center py-12">
          <div className="w-16 h-16 rounded-2xl bg-accent/10 flex items-center justify-center mx-auto mb-6">
            <Zap size={32} className="text-accent" />
          </div>
          <h1 className="text-2xl font-bold text-text-primary mb-3">Welcome to Maxx Next</h1>
          <p className="text-sm text-text-secondary max-w-md mx-auto mb-8">
            AI API Proxy Gateway - Route your AI requests through multiple providers with intelligent failover and load balancing.
          </p>
          <Link
            to="/providers"
            className="inline-flex items-center gap-2 bg-accent text-white px-6 py-2.5 rounded-lg hover:bg-accent-hover transition-colors font-medium text-sm"
          >
            Get Started
            <ArrowRight className="h-4 w-4" />
          </Link>
        </div>
      )}

      {/* Stats Grid */}
      <div className="grid gap-6 md:grid-cols-2 lg:grid-cols-4">
        {stats.map((stat) => {
          const Icon = stat.icon;
          return (
            <Link key={stat.label} to={stat.href}>
              <Card className="hover:shadow-card-hover cursor-pointer border-border bg-surface-primary transition-all duration-200 hover:border-accent/50">
                <CardContent className="p-6">
                  <div className="flex items-center justify-between">
                    <div>
                      <p className="text-xs font-medium text-text-secondary uppercase tracking-wider">{stat.label}</p>
                      <p className="text-2xl font-bold text-text-primary mt-1">{stat.value}</p>
                    </div>
                    <div className={`p-3 rounded-lg bg-surface-secondary ${stat.color}`}>
                      <Icon className="h-5 w-5" />
                    </div>
                  </div>
                </CardContent>
              </Card>
            </Link>
          );
        })}
      </div>

      {/* Status Cards */}
      <div className="grid gap-6 md:grid-cols-2">
        <Card className="border-border bg-surface-primary">
          <CardHeader className="border-b border-border py-4">
            <CardTitle className="text-base font-medium">Request Status</CardTitle>
          </CardHeader>
          <CardContent className="p-6">
            <div className="space-y-3">
              <div className="flex items-center justify-between p-3 rounded-lg bg-surface-secondary/50 border border-border">
                <div className="flex items-center gap-3">
                  <CheckCircle className="h-4 w-4 text-success" />
                  <span className="text-sm font-medium text-text-secondary">Completed</span>
                </div>
                <span className="text-lg font-bold text-success font-mono">{completedRequests}</span>
              </div>
              <div className="flex items-center justify-between p-3 rounded-lg bg-surface-secondary/50 border border-border">
                <div className="flex items-center gap-3">
                  <XCircle className="h-4 w-4 text-error" />
                  <span className="text-sm font-medium text-text-secondary">Failed</span>
                </div>
                <span className="text-lg font-bold text-error font-mono">{failedRequests}</span>
              </div>
              <div className="flex items-center justify-between p-3 rounded-lg bg-surface-secondary/50 border border-border">
                <div className="flex items-center gap-3">
                  <Ban className="h-4 w-4 text-warning" />
                  <span className="text-sm font-medium text-text-secondary">Cancelled</span>
                </div>
                <span className="text-lg font-bold text-warning font-mono">{cancelledRequests}</span>
              </div>
            </div>
          </CardContent>
        </Card>

        <Card className="border-border bg-surface-primary">
          <CardHeader className="border-b border-border py-4">
            <CardTitle className="text-base font-medium">Quick Actions</CardTitle>
          </CardHeader>
          <CardContent className="p-6">
            <div className="space-y-3">
              <Link
                to="/providers"
                className="flex items-center justify-between p-3 rounded-lg bg-surface-secondary/50 border border-border hover:bg-surface-hover hover:border-accent/30 transition-all group"
              >
                <div className="flex items-center gap-3">
                  <Server className="h-4 w-4 text-info" />
                  <span className="text-sm font-medium text-text-primary">Manage Providers</span>
                </div>
                <ArrowRight className="h-4 w-4 text-text-muted group-hover:text-text-primary transition-colors" />
              </Link>
              <Link
                to="/routes"
                className="flex items-center justify-between p-3 rounded-lg bg-surface-secondary/50 border border-border hover:bg-surface-hover hover:border-accent/30 transition-all group"
              >
                <div className="flex items-center gap-3">
                  <Route className="h-4 w-4 text-accent" />
                  <span className="text-sm font-medium text-text-primary">Configure Routes</span>
                </div>
                <ArrowRight className="h-4 w-4 text-text-muted group-hover:text-text-primary transition-colors" />
              </Link>
              <Link
                to="/requests"
                className="flex items-center justify-between p-3 rounded-lg bg-surface-secondary/50 border border-border hover:bg-surface-hover hover:border-accent/30 transition-all group"
              >
                <div className="flex items-center gap-3">
                  <Activity className="h-4 w-4 text-success" />
                  <span className="text-sm font-medium text-text-primary">View Requests</span>
                </div>
                <ArrowRight className="h-4 w-4 text-text-muted group-hover:text-text-primary transition-colors" />
              </Link>
            </div>
          </CardContent>
        </Card>
      </div>

      {/* Features */}
      {!hasProviders && (
        <div className="grid grid-cols-3 gap-6">
          <div className="bg-surface-secondary/30 border border-border rounded-xl p-6 text-center">
            <div className="w-10 h-10 rounded-lg bg-success/10 flex items-center justify-center mx-auto mb-4">
              <CheckCircle className="h-5 w-5 text-success" />
            </div>
            <h3 className="text-sm font-semibold text-text-primary">Secure</h3>
            <p className="text-xs text-text-secondary mt-1">End-to-end encryption</p>
          </div>
          <div className="bg-surface-secondary/30 border border-border rounded-xl p-6 text-center">
            <div className="w-10 h-10 rounded-lg bg-accent/10 flex items-center justify-center mx-auto mb-4">
              <Zap className="h-5 w-5 text-accent" />
            </div>
            <h3 className="text-sm font-semibold text-text-primary">Fast</h3>
            <p className="text-xs text-text-secondary mt-1">Low latency routing</p>
          </div>
          <div className="bg-surface-secondary/30 border border-border rounded-xl p-6 text-center">
            <div className="w-10 h-10 rounded-lg bg-info/10 flex items-center justify-center mx-auto mb-4">
              <Activity className="h-5 w-5 text-info" />
            </div>
            <h3 className="text-sm font-semibold text-text-primary">Insights</h3>
            <p className="text-xs text-text-secondary mt-1">Real-time analytics</p>
          </div>
        </div>
      )}
        </div>
      </div>
    </div>
  );
}