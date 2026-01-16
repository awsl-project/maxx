import { useState } from 'react'
import { useTranslation } from 'react-i18next'
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
} from '@/components/ui'
import {
  useAPITokens,
  useCreateAPIToken,
  useUpdateAPIToken,
  useDeleteAPIToken,
  useProjects,
  useSettings,
  useUpdateSetting,
} from '@/hooks/queries'
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
} from 'lucide-react'
import { PageHeader } from '@/components/layout'
import type { APIToken } from '@/lib/transport'

export function APITokensPage() {
  const { t, i18n } = useTranslation()
  const { data: tokens, isLoading } = useAPITokens()
  const { data: projects } = useProjects()
  const { data: settings } = useSettings()
  const updateSetting = useUpdateSetting()
  const createToken = useCreateAPIToken()
  const updateToken = useUpdateAPIToken()
  const deleteToken = useDeleteAPIToken()

  const apiTokenAuthEnabled = settings?.api_token_auth_enabled === 'true'

  const handleToggleAuth = (checked: boolean) => {
    updateSetting.mutate({
      key: 'api_token_auth_enabled',
      value: checked ? 'true' : 'false',
    })
  }

  const [showForm, setShowForm] = useState(false)
  const [editingToken, setEditingToken] = useState<APIToken | null>(null)
  const [deletingToken, setDeletingToken] = useState<APIToken | null>(null)
  const [newTokenDialog, setNewTokenDialog] = useState<{
    token: string
    name: string
  } | null>(null)
  const [copied, setCopied] = useState(false)

  // Form state
  const [name, setName] = useState('')
  const [description, setDescription] = useState('')
  const [projectID, setProjectID] = useState<string>('0')
  const [expiresAt, setExpiresAt] = useState('')
  const [showProjectPicker, setShowProjectPicker] = useState(false)

  const resetForm = () => {
    setName('')
    setDescription('')
    setProjectID('0')
    setExpiresAt('')
    setShowProjectPicker(false)
  }

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault()
    createToken.mutate(
      {
        name,
        description,
        projectID: parseInt(projectID) || 0,
        expiresAt: expiresAt ? new Date(expiresAt).toISOString() : undefined,
      },
      {
        onSuccess: result => {
          setShowForm(false)
          resetForm()
          // Show the new token dialog
          setNewTokenDialog({ token: result.token, name: result.apiToken.name })
        },
      }
    )
  }

  const handleUpdate = (e: React.FormEvent) => {
    e.preventDefault()
    if (!editingToken) return

    updateToken.mutate(
      {
        id: editingToken.id,
        data: {
          name,
          description,
          projectID: parseInt(projectID) || 0,
          expiresAt: expiresAt ? new Date(expiresAt).toISOString() : undefined,
        },
      },
      {
        onSuccess: () => {
          setEditingToken(null)
          resetForm()
        },
      }
    )
  }

  const handleToggleEnabled = (token: APIToken) => {
    updateToken.mutate({
      id: token.id,
      data: { isEnabled: !token.isEnabled },
    })
  }

  const handleDelete = () => {
    if (!deletingToken) return
    deleteToken.mutate(deletingToken.id, {
      onSuccess: () => setDeletingToken(null),
    })
  }

  const handleEdit = (token: APIToken) => {
    setEditingToken(token)
    setName(token.name)
    setDescription(token.description)
    setProjectID(token.projectID.toString())
    setExpiresAt(token.expiresAt ? token.expiresAt.split('T')[0] : '')
  }

  const handleCopyToken = async () => {
    if (!newTokenDialog) return
    await navigator.clipboard.writeText(newTokenDialog.token)
    setCopied(true)
    setTimeout(() => setCopied(false), 2000)
  }

  const getProjectName = (projectId: number) => {
    if (projectId === 0) return t('apiTokens.global')
    const project = projects?.find(p => p.id === projectId)
    return project?.name || `Project #${projectId}`
  }

  const isExpired = (token: APIToken) => {
    if (!token.expiresAt) return false
    return new Date(token.expiresAt) < new Date()
  }

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
              setShowForm(!showForm)
              if (showForm) resetForm()
            }}
            variant={showForm ? 'secondary' : 'default'}
          >
            {showForm ? (
              <X className="mr-2 h-4 w-4" />
            ) : (
              <Plus className="mr-2 h-4 w-4" />
            )}
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
                  <p className="text-text-muted mb-6">
                    {t('apiTokens.enableAuthPrompt')}
                  </p>
                  <Button
                    onClick={() => handleToggleAuth(true)}
                    disabled={updateSetting.isPending}
                  >
                    {updateSetting.isPending && (
                      <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                    )}
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
                    <Badge variant="default" className="bg-green-500/10 text-green-500 border-green-500/20">
                      {t('common.enabled')}
                    </Badge>
                    <Button variant="outline" size="sm" onClick={() => handleToggleAuth(false)} disabled={updateSetting.isPending}>
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
                  <TableHead>{t('apiTokens.lastUsed')}</TableHead>
                  <TableHead className="text-right">{t('common.actions')}</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {tokens.map(token => (
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
                            await navigator.clipboard.writeText(token.token)
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
                          <Badge variant="destructive" className="text-xs">{t('apiTokens.expired')}</Badge>
                        ) : token.isEnabled ? (
                          <Badge variant="default" className="text-xs bg-green-500/10 text-green-500 border-green-500/20">{t('apiTokens.active')}</Badge>
                        ) : (
                          <Badge variant="secondary" className="text-xs">{t('common.disabled')}</Badge>
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
                      {token.lastUsedAt ? (
                        <div className="flex items-center gap-1 text-xs text-text-muted">
                          <Clock className="h-3 w-3" />
                          {new Date(token.lastUsedAt).toLocaleDateString(
                            i18n.resolvedLanguage ?? i18n.language,
                            {
                              month: 'short',
                              day: 'numeric',
                              year: 'numeric',
                            }
                          )}
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
                          onClick={() => handleEdit(token)}
                        >
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
          setShowForm(open)
          if (!open) resetForm()
        }}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t('apiTokens.createDialog.title')}</DialogTitle>
            <DialogDescription>
              {t('apiTokens.createDialog.description')}
            </DialogDescription>
          </DialogHeader>
          <form onSubmit={handleSubmit} className="space-y-4">
            <div className="space-y-2">
              <label className="text-xs font-medium text-text-secondary uppercase tracking-wider">
                {t('common.name')} *
              </label>
              <Input
                value={name}
                onChange={e => setName(e.target.value)}
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
                    <Badge
                      variant="outline"
                      className="flex-1 justify-start py-2 px-3 font-normal"
                    >
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
                onChange={e => setDescription(e.target.value)}
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
                onChange={e => setExpiresAt(e.target.value)}
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
        onOpenChange={(open: boolean) => !open && setEditingToken(null)}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t('apiTokens.editDialog.title')}</DialogTitle>
            <DialogDescription>
              {t('apiTokens.editDialog.description')}
            </DialogDescription>
          </DialogHeader>
          <form onSubmit={handleUpdate} className="space-y-4">
            <div className="space-y-2">
              <label className="text-xs font-medium text-text-secondary uppercase tracking-wider">
                {t('common.name')} *
              </label>
              <Input
                value={name}
                onChange={e => setName(e.target.value)}
                required
              />
            </div>
            <div className="space-y-2">
              <label className="text-xs font-medium text-text-secondary uppercase tracking-wider">
                {t('common.description')}
              </label>
              <Input
                value={description}
                onChange={e => setDescription(e.target.value)}
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
                    <Badge
                      variant="outline"
                      className="flex-1 justify-start py-2 px-3 font-normal"
                    >
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
                onChange={e => setExpiresAt(e.target.value)}
                min={new Date().toISOString().split('T')[0]}
              />
            </div>
            <DialogFooter>
              <Button type="button" variant="outline" onClick={() => setEditingToken(null)}>
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
              {t('apiTokens.deleteDialog.description', { name: deletingToken?.name })}
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={() => setDeletingToken(null)}>
              {t('common.cancel')}
            </Button>
            <Button
              variant="destructive"
              onClick={handleDelete}
              disabled={deleteToken.isPending}
            >
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
            <DialogDescription>
              {t('apiTokens.newTokenDialog.description')}
            </DialogDescription>
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
            <div className="bg-amber-500/10 border border-amber-500/20 rounded-lg p-3">
              <p className="text-sm text-amber-600 dark:text-amber-400">
                <strong>{t('common.confirm')}:</strong> {t('apiTokens.newTokenDialog.warning')}
              </p>
            </div>
          </div>
          <DialogFooter>
            <Button onClick={() => setNewTokenDialog(null)}>
              {t('apiTokens.newTokenDialog.done')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Project Picker Dialog */}
      <Dialog
        open={showProjectPicker}
        onOpenChange={(open: boolean) => setShowProjectPicker(open)}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t('apiTokens.projectDialog.title')}</DialogTitle>
            <DialogDescription>
              {t('apiTokens.projectDialog.description')}
            </DialogDescription>
          </DialogHeader>
          <div className="space-y-2 max-h-64 overflow-auto">
            {projects?.map(project => (
              <Button
                key={project.id}
                variant="ghost"
                className="w-full justify-start"
                onClick={() => {
                  setProjectID(project.id.toString())
                  setShowProjectPicker(false)
                }}
              >
                <FolderKanban className="mr-2 h-4 w-4" />
                {project.name}
              </Button>
            ))}
            {(!projects || projects.length === 0) && (
              <p className="text-sm text-text-muted text-center py-4">{t('apiTokens.projectDialog.noProjects')}</p>
            )}
          </div>
          <DialogFooter>
            <Button
              variant="outline"
              onClick={() => {
                setProjectID('0')
                setShowProjectPicker(false)
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
  )
}
