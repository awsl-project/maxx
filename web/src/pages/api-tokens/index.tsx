import { useId, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  Button,
  Card,
  CardContent,
  Input,
  Switch,
  Badge,
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui';
import {
  useAPITokens,
  useCreateAPIToken,
  useUpdateAPIToken,
  useDeleteAPIToken,
  useProjects,
  useProxyStatus,
  useSettings,
  useUpdateSetting,
} from '@/hooks/queries';
import {
  Plus,
  X,
  Key,
  Loader2,
  Pencil,
  Trash2,
  Copy,
  Check,
  Clock,
  Hash,
  FolderKanban,
  Shield,
  Terminal,
  CircleCheck,
  CircleAlert,
} from 'lucide-react';
import { PageHeader } from '@/components/layout';
import type { APIToken } from '@/lib/transport';
import { buildCodexConfigBundle, buildProxyBaseUrl } from '@/lib/codex-config';

type CodexConfigDialogState = {
  tokenName: string;
  tokenValue: string;
  isEnabled: boolean;
  expiresAt?: string;
};

export function APITokensPage() {
  const { t, i18n } = useTranslation();
  const { data: tokens, isLoading } = useAPITokens();
  const { data: projects } = useProjects();
  const { data: proxyStatus } = useProxyStatus();
  const { data: settings } = useSettings();
  const updateSetting = useUpdateSetting();
  const createToken = useCreateAPIToken();
  const updateToken = useUpdateAPIToken();
  const deleteToken = useDeleteAPIToken();

  const apiTokenAuthEnabled = settings?.api_token_auth_enabled === 'true';

  const handleToggleAuth = (checked: boolean) => {
    updateSetting.mutate({
      key: 'api_token_auth_enabled',
      value: checked ? 'true' : 'false',
    });
  };

  const [showForm, setShowForm] = useState(false);
  const [editingToken, setEditingToken] = useState<APIToken | null>(null);
  const [deletingToken, setDeletingToken] = useState<APIToken | null>(null);
  const [newTokenDialog, setNewTokenDialog] = useState<{
    token: string;
    name: string;
    isEnabled: boolean;
    expiresAt?: string;
  } | null>(null);
  const [copied, setCopied] = useState(false);
  const [codexConfigDialog, setCodexConfigDialog] = useState<CodexConfigDialogState | null>(null);
  const [copiedCodexSection, setCopiedCodexSection] = useState<'configToml' | 'authJson' | null>(
    null,
  );
  const devModeSwitchId = useId();

  // Form state
  const [name, setName] = useState('');
  const [description, setDescription] = useState('');
  const [projectID, setProjectID] = useState<string>('0');
  const [expiresAt, setExpiresAt] = useState('');
  const [devMode, setDevMode] = useState(false);
  const [showProjectPicker, setShowProjectPicker] = useState(false);

  const resetForm = () => {
    setName('');
    setDescription('');
    setProjectID('0');
    setExpiresAt('');
    setDevMode(false);
    setShowProjectPicker(false);
  };

  const closeEditDialog = () => {
    setEditingToken(null);
    resetForm();
  };

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    createToken.mutate(
      {
        name,
        description,
        projectID: parseInt(projectID) || 0,
        expiresAt: expiresAt ? new Date(expiresAt).toISOString() : undefined,
      },
      {
        onSuccess: (result) => {
          setShowForm(false);
          resetForm();
          // Show the new token dialog
          setNewTokenDialog({
            token: result.token,
            name: result.apiToken.name,
            isEnabled: result.apiToken.isEnabled,
            expiresAt: result.apiToken.expiresAt,
          });
        },
      },
    );
  };

  const handleUpdate = (e: React.FormEvent) => {
    e.preventDefault();
    if (!editingToken) return;

    updateToken.mutate(
      {
        id: editingToken.id,
        data: {
          name,
          description,
          projectID: parseInt(projectID) || 0,
          expiresAt: expiresAt ? new Date(expiresAt).toISOString() : undefined,
          devMode,
        },
      },
      {
        onSuccess: () => closeEditDialog(),
      },
    );
  };

  const handleToggleEnabled = (token: APIToken) => {
    updateToken.mutate({
      id: token.id,
      data: { isEnabled: !token.isEnabled },
    });
  };

  const handleDelete = () => {
    if (!deletingToken) return;
    deleteToken.mutate(deletingToken.id, {
      onSuccess: () => setDeletingToken(null),
    });
  };

  const handleEdit = (token: APIToken) => {
    setEditingToken(token);
    setName(token.name);
    setDescription(token.description);
    setProjectID(token.projectID.toString());
    setExpiresAt(token.expiresAt ? token.expiresAt.split('T')[0] : '');
    setDevMode(!!token.devMode);
  };

  const handleCopyToken = async () => {
    if (!newTokenDialog) return;
    await navigator.clipboard.writeText(newTokenDialog.token);
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  };

  const codexBaseUrl = useMemo(() => buildProxyBaseUrl(proxyStatus), [proxyStatus]);
  const codexBundle = useMemo(() => {
    if (!codexConfigDialog) return null;
    return buildCodexConfigBundle({
      token: codexConfigDialog.tokenValue,
      baseUrl: codexBaseUrl,
    });
  }, [codexConfigDialog, codexBaseUrl]);

  const isCodexTokenExpired =
    !!codexConfigDialog?.expiresAt && new Date(codexConfigDialog.expiresAt) < new Date();

  const codexPreflightChecks = useMemo(
    () => [
      {
        key: 'proxy',
        ok: !!proxyStatus?.address,
        message: t('apiTokens.codexConfigDialog.checks.proxy'),
      },
      {
        key: 'tokenEnabled',
        ok: codexConfigDialog?.isEnabled ?? false,
        message: t('apiTokens.codexConfigDialog.checks.tokenEnabled'),
      },
      {
        key: 'tokenExpiry',
        ok: !isCodexTokenExpired,
        message: t('apiTokens.codexConfigDialog.checks.tokenExpiry'),
      },
    ],
    [proxyStatus?.address, codexConfigDialog?.isEnabled, isCodexTokenExpired, t],
  );

  const openCodexConfigDialog = (token: {
    name: string;
    token: string;
    isEnabled: boolean;
    expiresAt?: string;
  }) => {
    setCopiedCodexSection(null);
    setCodexConfigDialog({
      tokenName: token.name,
      tokenValue: token.token,
      isEnabled: token.isEnabled,
      expiresAt: token.expiresAt,
    });
  };

  const handleCopyCodexSection = async (section: 'configToml' | 'authJson', content: string) => {
    if (typeof navigator === 'undefined' || !navigator.clipboard?.writeText) {
      console.error('Clipboard API is not available.');
      return;
    }

    try {
      await navigator.clipboard.writeText(content);
      setCopiedCodexSection(section);
      setTimeout(() => {
        setCopiedCodexSection((current) => (current === section ? null : current));
      }, 2000);
    } catch (error) {
      console.error('Failed to copy Codex config section.', error);
    }
  };

  const getCodexCheckHint = (checkKey: string) => {
    if (checkKey === 'proxy') return t('apiTokens.codexConfigDialog.checksHint.proxy');
    if (checkKey === 'tokenEnabled') return t('apiTokens.codexConfigDialog.checksHint.tokenEnabled');
    return t('apiTokens.codexConfigDialog.checksHint.tokenExpiry');
  };

  const getProjectName = (projectId: number) => {
    if (projectId === 0) return t('apiTokens.global');
    const project = projects?.find((p) => p.id === projectId);
    return project?.name || t('apiTokens.unknownProject', { id: projectId });
  };

  const isExpired = (token: APIToken) => {
    if (!token.expiresAt) return false;
    return new Date(token.expiresAt) < new Date();
  };

  const formatDateTime = (value?: string) => {
    if (!value) return t('apiTokens.never');
    return new Date(value).toLocaleString(i18n.resolvedLanguage ?? i18n.language, {
      month: 'short',
      day: 'numeric',
      year: 'numeric',
      hour: '2-digit',
      minute: '2-digit',
    });
  };

  return (
    <div className="flex flex-col h-full bg-background">
      <PageHeader
        icon={Key}
        iconClassName="text-purple-500"
        title={t('apiTokens.title')}
        description={t('apiTokens.description')}
      >
        {apiTokenAuthEnabled && (
          <Button
            onClick={() => {
              setShowForm(!showForm);
              if (showForm) resetForm();
            }}
            variant={showForm ? 'secondary' : 'default'}
          >
            {showForm ? <X className="mr-2 h-4 w-4" /> : <Plus className="mr-2 h-4 w-4" />}
            {showForm ? t('common.cancel') : t('apiTokens.createToken')}
          </Button>
        )}
      </PageHeader>

      <div className="flex-1 overflow-auto p-6 space-y-6">
        {!apiTokenAuthEnabled ? (
          /* Disabled State */
          <div className="flex items-center justify-center h-full">
            <Card className="border-border bg-surface-primary">
              <CardContent className="py-16">
                <div className="flex flex-col items-center text-center max-w-md mx-auto">
                  <Shield className="h-16 w-16 text-text-muted mb-6 opacity-50" />
                  <h2 className="text-xl font-semibold mb-2">{t('apiTokens.authEnabled')}</h2>
                  <p className="text-text-muted mb-6">{t('apiTokens.enableAuthPrompt')}</p>
                  <Button onClick={() => handleToggleAuth(true)} disabled={updateSetting.isPending}>
                    {updateSetting.isPending && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
                    <Shield className="mr-2 h-4 w-4" />
                    {t('apiTokens.enableAuth')}
                  </Button>
                </div>
              </CardContent>
            </Card>
          </div>
        ) : (
          /* Enabled State */
          <>
            {/* Auth Status Card */}
            <Card className="border-border bg-surface-primary">
              <CardContent className="p-4">
                <div className="flex items-center justify-between">
                  <div className="flex items-center gap-3">
                    <Shield className="h-5 w-5 text-green-500" />
                    <div>
                      <p className="font-medium">{t('apiTokens.authEnabled')}</p>
                      <p className="text-sm text-text-muted">{t('apiTokens.authEnabledDesc')}</p>
                    </div>
                  </div>
                  <div className="flex items-center gap-3">
                    <Badge
                      variant="default"
                      className="bg-green-500/10 text-green-500 border-green-500/20"
                    >
                      {t('common.enabled')}
                    </Badge>
                    <Button
                      variant="outline"
                      size="sm"
                      onClick={() => handleToggleAuth(false)}
                      disabled={updateSetting.isPending}
                    >
                      {t('apiTokens.disableAuth')}
                    </Button>
                  </div>
                </div>
              </CardContent>
            </Card>

            {/* Token List */}
            {isLoading ? (
              <div className="flex items-center justify-center p-12">
                <Loader2 className="h-8 w-8 animate-spin text-accent" />
              </div>
            ) : tokens && tokens.length > 0 ? (
              <Card className="border-border bg-surface-primary">
                <Table>
                  <TableHeader>
                    <TableRow>
                      <TableHead>{t('apiTokens.tokenName')}</TableHead>
                      <TableHead>{t('apiTokens.tokenPrefix')}</TableHead>
                      <TableHead>{t('apiTokens.project')}</TableHead>
                      <TableHead>{t('common.status')}</TableHead>
                      <TableHead>{t('apiTokens.usage')}</TableHead>
                      <TableHead>{t('apiTokens.recentIP')}</TableHead>
                      <TableHead>{t('apiTokens.lastUsed')}</TableHead>
                      <TableHead className="text-right">{t('common.actions')}</TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {tokens.map((token) => (
                      <TableRow key={token.id}>
                        <TableCell>
                          <div>
                            <div className="font-medium">{token.name}</div>
                            {token.description && (
                              <div className="text-xs text-text-muted">{token.description}</div>
                            )}
                          </div>
                        </TableCell>
                        <TableCell>
                          <div className="flex items-center gap-1">
                            <code className="text-xs bg-surface-secondary px-2 py-1 rounded font-mono">
                              {token.tokenPrefix}
                            </code>
                            <Button
                              variant="ghost"
                              size="sm"
                              className="h-6 w-6 p-0"
                              onClick={async () => {
                                await navigator.clipboard.writeText(token.token);
                              }}
                            >
                              <Copy className="h-3 w-3" />
                            </Button>
                          </div>
                        </TableCell>
                        <TableCell>
                          <Badge variant="outline" className="font-normal">
                            {getProjectName(token.projectID)}
                          </Badge>
                        </TableCell>
                        <TableCell>
                          <div className="flex items-center gap-2">
                            <Switch
                              checked={token.isEnabled}
                              onCheckedChange={() => handleToggleEnabled(token)}
                              disabled={updateToken.isPending}
                            />
                            {isExpired(token) ? (
                              <Badge variant="destructive" className="text-xs">
                                {t('apiTokens.expired')}
                              </Badge>
                            ) : token.isEnabled ? (
                              <Badge
                                variant="default"
                                className="text-xs bg-green-500/10 text-green-500 border-green-500/20"
                              >
                                {t('apiTokens.active')}
                              </Badge>
                            ) : (
                              <Badge variant="secondary" className="text-xs">
                                {t('common.disabled')}
                              </Badge>
                            )}
                          </div>
                        </TableCell>
                        <TableCell>
                          <div className="flex items-center gap-1 text-sm text-text-secondary">
                            <Hash className="h-3 w-3" />
                            {token.useCount}
                          </div>
                        </TableCell>
                        <TableCell>
                          {token.lastIP ? (
                            <div className="space-y-1 text-xs">
                              <code className="inline-flex rounded bg-surface-secondary px-2 py-1 font-mono text-text-secondary">
                                {token.lastIP}
                              </code>
                              <div className="flex items-center gap-1 text-text-muted">
                                <Clock className="h-3 w-3" />
                                {formatDateTime(token.lastIPAt)}
                              </div>
                            </div>
                          ) : (
                            <span className="text-xs text-text-muted">{t('apiTokens.never')}</span>
                          )}
                        </TableCell>
                        <TableCell>
                          {token.lastUsedAt ? (
                            <div className="flex items-center gap-1 text-xs text-text-muted">
                              <Clock className="h-3 w-3" />
                              {formatDateTime(token.lastUsedAt)}
                            </div>
                          ) : (
                            <span className="text-xs text-text-muted">{t('apiTokens.never')}</span>
                          )}
                        </TableCell>
                        <TableCell className="text-right">
                          <div className="flex justify-end gap-1">
                            <Button
                              variant="ghost"
                              size="sm"
                              aria-label={t('apiTokens.generateCodexConfig')}
                              onClick={() =>
                                openCodexConfigDialog({
                                  name: token.name,
                                  token: token.token,
                                  isEnabled: token.isEnabled,
                                  expiresAt: token.expiresAt,
                                })
                              }
                              title={t('apiTokens.generateCodexConfig')}
                            >
                              <Terminal className="h-4 w-4" />
                            </Button>
                            <Button variant="ghost" size="sm" onClick={() => handleEdit(token)}>
                              <Pencil className="h-4 w-4" />
                            </Button>
                            <Button
                              variant="ghost"
                              size="sm"
                              onClick={() => setDeletingToken(token)}
                              className="text-destructive hover:text-destructive"
                            >
                              <Trash2 className="h-4 w-4" />
                            </Button>
                          </div>
                        </TableCell>
                      </TableRow>
                    ))}
                  </TableBody>
                </Table>
              </Card>
            ) : (
              <div className="flex flex-col items-center justify-center h-64 text-text-muted border-2 border-dashed border-border rounded-lg bg-surface-primary/50">
                <Key className="h-12 w-12 opacity-20 mb-4" />
                <p className="text-lg font-medium">{t('apiTokens.noTokens')}</p>
                <p className="text-sm">{t('apiTokens.noTokensHint')}</p>
              </div>
            )}
          </>
        )}
      </div>

      {/* Create Dialog */}
      <Dialog
        open={showForm}
        onOpenChange={(open: boolean) => {
          setShowForm(open);
          if (!open) resetForm();
        }}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t('apiTokens.createDialog.title')}</DialogTitle>
            <DialogDescription>{t('apiTokens.createDialog.description')}</DialogDescription>
          </DialogHeader>
          <form onSubmit={handleSubmit} className="space-y-4">
            <div className="space-y-2">
              <label className="text-xs font-medium text-text-secondary uppercase tracking-wider">
                {t('common.name')} *
              </label>
              <Input
                value={name}
                onChange={(e) => setName(e.target.value)}
                placeholder={t('apiTokens.createDialog.namePlaceholder')}
                required
              />
            </div>
            <div className="space-y-2">
              <label className="text-xs font-medium text-text-secondary uppercase tracking-wider">
                {t('apiTokens.project')}
              </label>
              <div className="flex items-center gap-2">
                {projectID === '0' ? (
                  <Button
                    type="button"
                    variant="outline"
                    className="w-full justify-start text-muted-foreground"
                    onClick={() => setShowProjectPicker(true)}
                  >
                    <FolderKanban className="mr-2 h-4 w-4" />
                    {t('apiTokens.notSpecified')}
                  </Button>
                ) : (
                  <div className="flex items-center gap-2 w-full">
                    <Badge variant="outline" className="flex-1 justify-start py-2 px-3 font-normal">
                      <FolderKanban className="mr-2 h-4 w-4" />
                      {getProjectName(parseInt(projectID))}
                    </Badge>
                    <Button
                      type="button"
                      variant="ghost"
                      size="sm"
                      onClick={() => setProjectID('0')}
                    >
                      <X className="h-4 w-4" />
                    </Button>
                  </div>
                )}
              </div>
            </div>
            <div className="space-y-2">
              <label className="text-xs font-medium text-text-secondary uppercase tracking-wider">
                {t('common.description')}
              </label>
              <Input
                value={description}
                onChange={(e) => setDescription(e.target.value)}
                placeholder={t('apiTokens.createDialog.descriptionPlaceholder')}
              />
            </div>
            <div className="space-y-2">
              <label className="text-xs font-medium text-text-secondary uppercase tracking-wider">
                {t('apiTokens.createDialog.expiresAt')}
              </label>
              <Input
                type="date"
                value={expiresAt}
                onChange={(e) => setExpiresAt(e.target.value)}
                min={new Date().toISOString().split('T')[0]}
              />
              <p className="text-xs text-text-muted">{t('apiTokens.createDialog.expiresAtHint')}</p>
            </div>
            <DialogFooter>
              <Button type="button" variant="outline" onClick={() => setShowForm(false)}>
                {t('common.cancel')}
              </Button>
              <Button type="submit" disabled={createToken.isPending || !name}>
                {createToken.isPending && <Loader2 className="h-4 w-4 animate-spin mr-2" />}
                {t('apiTokens.createToken')}
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>

      {/* Edit Dialog */}
      <Dialog
        open={!!editingToken}
        onOpenChange={(open: boolean) => {
          if (!open) {
            closeEditDialog();
          }
        }}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t('apiTokens.editDialog.title')}</DialogTitle>
            <DialogDescription>{t('apiTokens.editDialog.description')}</DialogDescription>
          </DialogHeader>
          <form onSubmit={handleUpdate} className="space-y-4">
            <div className="space-y-2">
              <label className="text-xs font-medium text-text-secondary uppercase tracking-wider">
                {t('common.name')} *
              </label>
              <Input value={name} onChange={(e) => setName(e.target.value)} required />
            </div>
            <div className="space-y-2">
              <label className="text-xs font-medium text-text-secondary uppercase tracking-wider">
                {t('common.description')}
              </label>
              <Input value={description} onChange={(e) => setDescription(e.target.value)} />
            </div>
            <div className="space-y-2">
              <label className="text-xs font-medium text-text-secondary uppercase tracking-wider">
                {t('apiTokens.project')}
              </label>
              <div className="flex items-center gap-2">
                {projectID === '0' ? (
                  <Button
                    type="button"
                    variant="outline"
                    className="w-full justify-start text-muted-foreground"
                    onClick={() => setShowProjectPicker(true)}
                  >
                    <FolderKanban className="mr-2 h-4 w-4" />
                    {t('apiTokens.notSpecified')}
                  </Button>
                ) : (
                  <div className="flex items-center gap-2 w-full">
                    <Badge variant="outline" className="flex-1 justify-start py-2 px-3 font-normal">
                      <FolderKanban className="mr-2 h-4 w-4" />
                      {getProjectName(parseInt(projectID))}
                    </Badge>
                    <Button
                      type="button"
                      variant="ghost"
                      size="sm"
                      onClick={() => setProjectID('0')}
                    >
                      <X className="h-4 w-4" />
                    </Button>
                  </div>
                )}
              </div>
            </div>
            <div className="space-y-2">
              <label className="text-xs font-medium text-text-secondary uppercase tracking-wider">
                {t('apiTokens.createDialog.expiresAt')}
              </label>
              <Input
                type="date"
                value={expiresAt}
                onChange={(e) => setExpiresAt(e.target.value)}
                min={new Date().toISOString().split('T')[0]}
              />
            </div>
            <div className="flex items-center justify-between">
              <label
                htmlFor={devModeSwitchId}
                className="text-xs font-medium text-text-secondary uppercase tracking-wider"
              >
                {t('apiTokens.devMode')}
              </label>
              <div className="flex items-center gap-2">
                <Switch
                  id={devModeSwitchId}
                  checked={devMode}
                  onCheckedChange={setDevMode}
                  disabled={updateToken.isPending}
                />
                <span className="text-xs text-text-muted">
                  {devMode ? t('apiTokens.devModeEnabled') : t('apiTokens.devModeDisabled')}
                </span>
              </div>
            </div>
            <DialogFooter>
              <Button
                type="button"
                variant="outline"
                onClick={closeEditDialog}
              >
                {t('common.cancel')}
              </Button>
              <Button type="submit" disabled={updateToken.isPending || !name}>
                {updateToken.isPending && <Loader2 className="h-4 w-4 animate-spin mr-2" />}
                {t('common.save')}
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>

      {/* Delete Confirmation */}
      <Dialog
        open={!!deletingToken}
        onOpenChange={(open: boolean) => !open && setDeletingToken(null)}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t('apiTokens.deleteDialog.title')}</DialogTitle>
            <DialogDescription>
              {t('apiTokens.deleteDialog.description', {
                name: deletingToken?.name,
              })}
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={() => setDeletingToken(null)}>
              {t('common.cancel')}
            </Button>
            <Button variant="destructive" onClick={handleDelete} disabled={deleteToken.isPending}>
              {deleteToken.isPending && <Loader2 className="h-4 w-4 animate-spin mr-2" />}
              {t('common.delete')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* New Token Dialog */}
      <Dialog
        open={!!newTokenDialog}
        onOpenChange={(open: boolean) => !open && setNewTokenDialog(null)}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t('apiTokens.newTokenDialog.title')}</DialogTitle>
            <DialogDescription>{t('apiTokens.newTokenDialog.description')}</DialogDescription>
          </DialogHeader>
          <div className="space-y-4">
            <div className="space-y-2">
              <label className="text-xs font-medium text-text-secondary uppercase tracking-wider">
                {t('apiTokens.newTokenDialog.tokenName')}
              </label>
              <p className="font-medium">{newTokenDialog?.name}</p>
            </div>
            <div className="space-y-2">
              <label className="text-xs font-medium text-text-secondary uppercase tracking-wider">
                {t('apiTokens.newTokenDialog.apiToken')}
              </label>
              <div className="flex gap-2">
                <code className="flex-1 text-sm bg-muted p-3 rounded font-mono break-all border border-border">
                  {newTokenDialog?.token}
                </code>
                <Button
                  variant="outline"
                  size="icon"
                  onClick={handleCopyToken}
                  className="shrink-0"
                >
                  {copied ? (
                    <Check className="h-4 w-4 text-green-500" />
                  ) : (
                    <Copy className="h-4 w-4" />
                  )}
                </Button>
              </div>
            </div>
          </div>
          <DialogFooter>
            <Button
              variant="outline"
              onClick={() => {
                if (!newTokenDialog) return;
                openCodexConfigDialog({
                  name: newTokenDialog.name,
                  token: newTokenDialog.token,
                  isEnabled: newTokenDialog.isEnabled,
                  expiresAt: newTokenDialog.expiresAt,
                });
                setNewTokenDialog(null);
              }}
            >
              <Terminal className="mr-2 h-4 w-4" />
              {t('apiTokens.generateCodexConfig')}
            </Button>
            <Button onClick={() => setNewTokenDialog(null)}>
              {t('apiTokens.newTokenDialog.done')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Codex Config Generator Dialog */}
      <Dialog
        open={!!codexConfigDialog}
        onOpenChange={(open: boolean) => !open && setCodexConfigDialog(null)}
      >
        <DialogContent className="w-[min(48rem,calc(100vw-1.5rem))] max-w-[min(48rem,calc(100vw-1.5rem))] min-w-0">
          <DialogHeader>
            <DialogTitle>{t('apiTokens.codexConfigDialog.title')}</DialogTitle>
            <DialogDescription>
              {t('apiTokens.codexConfigDialog.description', {
                name: codexConfigDialog?.tokenName || '-',
              })}
            </DialogDescription>
          </DialogHeader>

          {codexConfigDialog && codexBundle && (
            <div className="space-y-4 min-w-0">
              <div className="flex min-w-0 flex-wrap items-center gap-2">
                <Badge variant="outline">{t('apiTokens.codexConfigDialog.modeGlobal')}</Badge>
                <Badge variant="secondary" className="min-w-0 max-w-full font-mono">
                  <span className="min-w-0 break-all">{codexBundle.baseUrl}</span>
                </Badge>
              </div>

              <div className="space-y-2 min-w-0">
                {codexPreflightChecks.map((check) => (
                  <div
                    key={check.key}
                    className="flex items-start justify-between gap-3 rounded-md border border-border bg-muted/30 p-3"
                  >
                    <div className="min-w-0">
                      <div className="flex items-center gap-2 text-sm font-medium">
                        {check.ok ? (
                          <CircleCheck className="h-4 w-4 shrink-0 text-green-500" />
                        ) : (
                          <CircleAlert className="h-4 w-4 shrink-0 text-amber-500" />
                        )}
                        <span>{check.message}</span>
                      </div>
                      {!check.ok && (
                        <p className="mt-1 text-xs text-muted-foreground">
                          {getCodexCheckHint(check.key)}
                        </p>
                      )}
                    </div>
                  </div>
                ))}
              </div>

              <div className="space-y-2 min-w-0">
                <label className="text-xs font-medium text-text-secondary uppercase tracking-wider">
                  {t('apiTokens.codexConfigDialog.configToml')}
                </label>
                <div className="relative">
                  <Button
                    type="button"
                    variant="outline"
                    size="sm"
                    className="absolute top-2 right-2 z-10 h-7 px-2 text-xs"
                    onClick={() => handleCopyCodexSection('configToml', codexBundle.configToml)}
                  >
                    {copiedCodexSection === 'configToml' ? (
                      <Check className="mr-1 h-3 w-3 text-green-500" />
                    ) : (
                      <Copy className="mr-1 h-3 w-3" />
                    )}
                    {copiedCodexSection === 'configToml'
                      ? t('common.copied')
                      : t('apiTokens.codexConfigDialog.copyToClipboard')}
                  </Button>
                  <pre className="max-h-64 w-full max-w-full min-w-0 overflow-x-auto overflow-y-auto rounded-md border border-border bg-muted/40 p-3 pr-24 text-xs font-mono">
                    <code className="block min-w-full whitespace-pre">{codexBundle.configToml}</code>
                  </pre>
                </div>
              </div>

              <div className="space-y-2 min-w-0">
                <label className="text-xs font-medium text-text-secondary uppercase tracking-wider">
                  {t('apiTokens.codexConfigDialog.authJson')}
                </label>
                <div className="relative">
                  <Button
                    type="button"
                    variant="outline"
                    size="sm"
                    className="absolute top-2 right-2 z-10 h-7 px-2 text-xs"
                    onClick={() => handleCopyCodexSection('authJson', codexBundle.authJson)}
                  >
                    {copiedCodexSection === 'authJson' ? (
                      <Check className="mr-1 h-3 w-3 text-green-500" />
                    ) : (
                      <Copy className="mr-1 h-3 w-3" />
                    )}
                    {copiedCodexSection === 'authJson'
                      ? t('common.copied')
                      : t('apiTokens.codexConfigDialog.copyToClipboard')}
                  </Button>
                  <pre className="max-h-56 w-full max-w-full min-w-0 overflow-x-auto overflow-y-auto rounded-md border border-border bg-muted/40 p-3 pr-24 text-xs font-mono">
                    <code className="block min-w-full whitespace-pre">{codexBundle.authJson}</code>
                  </pre>
                </div>
              </div>
            </div>
          )}

          <DialogFooter>
            <Button variant="outline" onClick={() => setCodexConfigDialog(null)}>
              {t('common.cancel')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Project Picker Dialog */}
      <Dialog open={showProjectPicker} onOpenChange={(open: boolean) => setShowProjectPicker(open)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t('apiTokens.projectDialog.title')}</DialogTitle>
            <DialogDescription>{t('apiTokens.projectDialog.description')}</DialogDescription>
          </DialogHeader>
          <div className="space-y-2 max-h-64 overflow-auto">
            {projects?.map((project) => (
              <Button
                key={project.id}
                variant="ghost"
                className="w-full justify-start"
                onClick={() => {
                  setProjectID(project.id.toString());
                  setShowProjectPicker(false);
                }}
              >
                <FolderKanban className="mr-2 h-4 w-4" />
                {project.name}
              </Button>
            ))}
            {(!projects || projects.length === 0) && (
              <p className="text-sm text-text-muted text-center py-4">
                {t('apiTokens.projectDialog.noProjects')}
              </p>
            )}
          </div>
          <DialogFooter>
            <Button
              variant="outline"
              onClick={() => {
                setProjectID('0');
                setShowProjectPicker(false);
              }}
            >
              {t('apiTokens.projectDialog.clearSelection')}
            </Button>
            <Button variant="secondary" onClick={() => setShowProjectPicker(false)}>
              {t('common.cancel')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}
