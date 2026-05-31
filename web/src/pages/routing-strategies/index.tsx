import { useState } from 'react';
import {
  Button,
  Card,
  CardContent,
  CardHeader,
  CardTitle,
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
  Badge,
} from '@/components/ui';
import {
  useRoutingStrategies,
  useCreateRoutingStrategy,
  useUpdateRoutingStrategy,
  useDeleteRoutingStrategy,
  useProjects,
} from '@/hooks/queries';
import { PageHeader } from '@/components/layout/page-header';
import { Plus, Trash2, Pencil, Workflow } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import type { RoutingStrategy, RoutingStrategyType, RoutingStickyScope } from '@/lib/transport';
import { Input } from '@/components/ui';
import { useDialog } from '@/contexts/dialog-context';

export function RoutingStrategiesPage() {
  const { t } = useTranslation();
  const { confirm } = useDialog();
  const { data: strategies, isLoading } = useRoutingStrategies();
  const { data: projects } = useProjects();
  const createStrategy = useCreateRoutingStrategy();
  const updateStrategy = useUpdateRoutingStrategy();
  const deleteStrategy = useDeleteRoutingStrategy();
  const [showForm, setShowForm] = useState(false);
  const [editingStrategy, setEditingStrategy] = useState<RoutingStrategy | undefined>();

  const [projectID, setProjectID] = useState('0');
  const [type, setType] = useState<RoutingStrategyType>('priority');
  const [stickyEnabled, setStickyEnabled] = useState(false);
  const [stickyScope, setStickyScope] = useState<RoutingStickyScope>('token');
  const [stickyTTL, setStickyTTL] = useState('1800');

  const resetForm = () => {
    setProjectID('0');
    setType('priority');
    setStickyEnabled(false);
    setStickyScope('token');
    setStickyTTL('1800');
  };

  const handleEdit = (strategy: RoutingStrategy) => {
    setEditingStrategy(strategy);
    setProjectID(String(strategy.projectID));
    setType(strategy.type);
    setStickyEnabled(strategy.config?.stickyEnabled ?? false);
    setStickyScope(strategy.config?.stickyScope ?? 'token');
    setStickyTTL(String(strategy.config?.stickyTTLSeconds ?? 1800));
    setShowForm(true);
  };

  const handleCloseForm = () => {
    setShowForm(false);
    setEditingStrategy(undefined);
    resetForm();
  };

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    // Sticky config is only meaningful for weighted_random; collapse it to
    // null for priority so the JSON stays clean.
    const ttlNum = Number(stickyTTL);
    const config =
      type === 'weighted_random' && stickyEnabled
        ? {
            stickyEnabled: true,
            stickyScope,
            stickyTTLSeconds: Number.isFinite(ttlNum) && ttlNum > 0 ? ttlNum : 1800,
          }
        : null;
    const data = {
      projectID: Number(projectID),
      type,
      config,
    };

    if (editingStrategy) {
      updateStrategy.mutate({ id: editingStrategy.id, data }, { onSuccess: handleCloseForm });
    } else {
      createStrategy.mutate(data, { onSuccess: handleCloseForm });
    }
  };

  const handleDelete = async (id: number) => {
    const confirmed = await confirm({
      title: t('common.confirm'),
      description: t('routingStrategies.deleteConfirm'),
      confirmText: t('common.delete'),
      confirmVariant: 'destructive',
    });
    if (!confirmed) return;

    deleteStrategy.mutate(id);
  };

  const getProjectName = (pid: number) => {
    if (pid === 0) return t('common.global');
    return projects?.find((p) => p.id === pid)?.name ?? `#${pid}`;
  };

  const isPending = createStrategy.isPending || updateStrategy.isPending;

  return (
    <div className="flex flex-col h-full">
      <PageHeader
        icon={Workflow}
        iconClassName="text-cyan-500"
        title={t('routingStrategies.title')}
        description={t('routingStrategies.description')}
        actions={
          <Button size="sm" onClick={() => setShowForm(true)}>
            <Plus className="mr-2 h-4 w-4" />
            {t('routingStrategies.addStrategy')}
          </Button>
        }
      />

      <div className="flex-1 overflow-auto p-4 md:p-6">
        <div className="space-y-6 max-w-7xl mx-auto">
          {showForm && (
            <Card>
              <CardHeader>
                <CardTitle>
                  {editingStrategy
                    ? t('routingStrategies.editTitle')
                    : t('routingStrategies.newTitle')}
                </CardTitle>
              </CardHeader>
              <CardContent>
                <form onSubmit={handleSubmit} className="space-y-4">
                  <div className="grid gap-4 md:grid-cols-2">
                    <div>
                      <label className="mb-1 block text-sm font-medium">
                        {t('common.project')}
                      </label>
                      <select
                        value={projectID}
                        onChange={(e) => setProjectID(e.target.value)}
                        className="w-full rounded-md border border-input bg-transparent px-3 py-2 text-sm shadow-xs focus:border-ring focus:ring-2 focus:ring-ring/50 outline-none"
                      >
                        <option value="0">{t('common.global')}</option>
                        {projects?.map((p) => (
                          <option key={p.id} value={p.id}>
                            {p.name}
                          </option>
                        ))}
                      </select>
                    </div>
                    <div>
                      <label className="mb-1 block text-sm font-medium">{t('common.type')}</label>
                      <select
                        value={type}
                        onChange={(e) => setType(e.target.value as RoutingStrategyType)}
                        className="w-full rounded-md border border-input bg-transparent px-3 py-2 text-sm shadow-xs focus:border-ring focus:ring-2 focus:ring-ring/50 outline-none"
                      >
                        <option value="priority">
                          {t('routingStrategies.priorityByPosition')}
                        </option>
                        <option value="weighted_random">
                          {t('routingStrategies.weightedRandom')}
                        </option>
                      </select>
                    </div>
                  </div>

                  {type === 'weighted_random' && (
                    <div className="rounded-md border border-input bg-muted/30 p-4 space-y-4">
                      <div className="flex items-center gap-2">
                        <input
                          type="checkbox"
                          id="stickyEnabled"
                          checked={stickyEnabled}
                          onChange={(e) => setStickyEnabled(e.target.checked)}
                          className="h-4 w-4 rounded border-gray-300"
                        />
                        <label htmlFor="stickyEnabled" className="text-sm font-medium">
                          {t('routingStrategies.stickyEnabled')}
                        </label>
                      </div>
                      <p className="text-xs text-text-secondary">
                        {t('routingStrategies.stickyHelp')}
                      </p>
                      {stickyEnabled && (
                        <div className="grid gap-4 md:grid-cols-2">
                          <div>
                            <label className="mb-1 block text-sm font-medium">
                              {t('routingStrategies.stickyScope')}
                            </label>
                            <select
                              value={stickyScope}
                              onChange={(e) => setStickyScope(e.target.value as RoutingStickyScope)}
                              className="w-full rounded-md border border-input bg-transparent px-3 py-2 text-sm shadow-xs focus:border-ring focus:ring-2 focus:ring-ring/50 outline-none"
                            >
                              <option value="token">
                                {t('routingStrategies.stickyScopeToken')}
                              </option>
                              <option value="conversation">
                                {t('routingStrategies.stickyScopeConversation')}
                              </option>
                            </select>
                            <p className="mt-1 text-xs text-text-secondary">
                              {stickyScope === 'token'
                                ? t('routingStrategies.stickyScopeTokenHelp')
                                : t('routingStrategies.stickyScopeConversationHelp')}
                            </p>
                          </div>
                          <div>
                            <label className="mb-1 block text-sm font-medium">
                              {t('routingStrategies.stickyTTL')}
                            </label>
                            <Input
                              type="number"
                              value={stickyTTL}
                              onChange={(e) => setStickyTTL(e.target.value)}
                              min="60"
                              step="60"
                            />
                            <p className="mt-1 text-xs text-text-secondary">
                              {t('routingStrategies.stickyTTLHelp')}
                            </p>
                          </div>
                        </div>
                      )}
                    </div>
                  )}

                  <div className="flex justify-end gap-2">
                    <Button type="button" variant="outline" onClick={handleCloseForm}>
                      {t('common.cancel')}
                    </Button>
                    <Button type="submit" disabled={isPending}>
                      {isPending
                        ? t('common.saving')
                        : editingStrategy
                          ? t('routes.update')
                          : t('routes.create')}
                    </Button>
                  </div>
                </form>
              </CardContent>
            </Card>
          )}

          <Card>
            <CardHeader>
              <CardTitle>{t('routingStrategies.allStrategies')}</CardTitle>
            </CardHeader>
            <CardContent>
              {isLoading ? (
                <p className="text-gray-500">{t('common.loading')}</p>
              ) : (
                <Table>
                  <TableHeader>
                    <TableRow>
                      <TableHead>{t('routingStrategies.id')}</TableHead>
                      <TableHead>{t('common.project')}</TableHead>
                      <TableHead>{t('common.type')}</TableHead>
                      <TableHead>{t('common.actions')}</TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {strategies?.map((strategy) => (
                      <TableRow key={strategy.id}>
                        <TableCell className="font-mono">{strategy.id}</TableCell>
                        <TableCell>
                          <span className={strategy.projectID === 0 ? 'text-gray-400' : ''}>
                            {getProjectName(strategy.projectID)}
                          </span>
                        </TableCell>
                        <TableCell>
                          <Badge variant={strategy.type === 'priority' ? 'info' : 'warning'}>
                            {strategy.type === 'priority'
                              ? t('routingStrategies.priority')
                              : t('routingStrategies.weightedRandom')}
                          </Badge>
                        </TableCell>
                        <TableCell>
                          <div className="flex gap-1">
                            <Button variant="ghost" size="sm" onClick={() => handleEdit(strategy)}>
                              <Pencil className="h-4 w-4" />
                            </Button>
                            <Button
                              variant="ghost"
                              size="sm"
                              onClick={() => handleDelete(strategy.id)}
                              disabled={deleteStrategy.isPending}
                            >
                              <Trash2 className="h-4 w-4 text-red-500" />
                            </Button>
                          </div>
                        </TableCell>
                      </TableRow>
                    ))}
                    {(!strategies || strategies.length === 0) && (
                      <TableRow>
                        <TableCell colSpan={4} className="text-center text-gray-500">
                          {t('routingStrategies.noStrategies')}
                        </TableCell>
                      </TableRow>
                    )}
                  </TableBody>
                </Table>
              )}
            </CardContent>
          </Card>
        </div>
      </div>
    </div>
  );
}
