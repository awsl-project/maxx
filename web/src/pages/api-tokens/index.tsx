import { useState } from 'react'
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
    if (projectId === 0) return 'Global'
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
        title="API Tokens"
        description="Manage API tokens for proxy authentication"
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
            {showForm ? 'Cancel' : 'Create Token'}
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
                  <h2 className="text-xl font-semibold mb-2">
                    API Token Authentication
                  </h2>
                  <p className="text-text-muted mb-6">
                    Enable API Token Authentication to require valid tokens for
                    proxy requests. This adds an extra layer of security by
                    ensuring only authorized clients can access the proxy.
                  </p>
                  <Button
                    onClick={() => handleToggleAuth(true)}
                    disabled={updateSetting.isPending}
                  >
                    {updateSetting.isPending && (
                      <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                    )}
                    <Shield className="mr-2 h-4 w-4" />
                    Enable Authentication
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
                      <p className="font-medium">API Token Authentication</p>
                      <p className="text-sm text-text-muted">
                        Proxy requests require a valid API token
                      </p>
                    </div>
                  </div>
                  <div className="flex items-center gap-3">
                    <Badge
                      variant="default"
                      className="bg-green-500/10 text-green-500 border-green-500/20"
                    >
                      Enabled
                    </Badge>
                    <Button
                      variant="outline"
                      size="sm"
                      onClick={() => handleToggleAuth(false)}
                      disabled={updateSetting.isPending}
                    >
                      Disable
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
                      <TableHead>Name</TableHead>
                      <TableHead>Token Prefix</TableHead>
                      <TableHead>Project</TableHead>
                      <TableHead>Status</TableHead>
                      <TableHead>Usage</TableHead>
                      <TableHead>Last Used</TableHead>
                      <TableHead className="text-right">Actions</TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {tokens.map(token => (
                      <TableRow key={token.id}>
                        <TableCell>
                          <div>
                            <div className="font-medium">{token.name}</div>
                            {token.description && (
                              <div className="text-xs text-text-muted">
                                {token.description}
                              </div>
                            )}
                          </div>
                        </TableCell>
                        <TableCell>
                          <div className="flex items-center gap-1">
                            <code className="text-xs bg-muted px-2 py-1 rounded font-mono">
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
                              <Badge variant="destructive" className="text-xs">
                                Expired
                              </Badge>
                            ) : token.isEnabled ? (
                              <Badge
                                variant="default"
                                className="text-xs bg-green-500/10 text-green-500 border-green-500/20"
                              >
                                Active
                              </Badge>
                            ) : (
                              <Badge variant="secondary" className="text-xs">
                                Disabled
                              </Badge>
                            )}
                          </div>
                        </TableCell>
                        <TableCell>
                          <div className="flex items-center gap-1 text-sm text-muted-foreground">
                            <Hash className="h-3 w-3" />
                            {token.useCount}
                          </div>
                        </TableCell>
                        <TableCell>
                          {token.lastUsedAt ? (
                            <div className="flex items-center gap-1 text-xs text-text-muted">
                              <Clock className="h-3 w-3" />
                              {new Date(token.lastUsedAt).toLocaleDateString()}
                            </div>
                          ) : (
                            <span className="text-xs text-text-muted">
                              Never
                            </span>
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
                <p className="text-lg font-medium">No API tokens</p>
                <p className="text-sm">
                  Create a token to authenticate proxy requests
                </p>
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
            <DialogTitle>Create New API Token</DialogTitle>
            <DialogDescription>
              Create a token to authenticate proxy requests.
            </DialogDescription>
          </DialogHeader>
          <form onSubmit={handleSubmit} className="space-y-4">
            <div className="space-y-2">
              <label className="text-xs font-medium text-muted-foreground uppercase tracking-wider">
                Name *
              </label>
              <Input
                value={name}
                onChange={e => setName(e.target.value)}
                placeholder="e.g., Claude Code Token"
                required
              />
            </div>
            <div className="space-y-2">
              <label className="text-xs font-medium text-muted-foreground uppercase tracking-wider">
                Project
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
                    Not Specified
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
              <label className="text-xs font-medium text-muted-foreground uppercase tracking-wider">
                Description
              </label>
              <Input
                value={description}
                onChange={e => setDescription(e.target.value)}
                placeholder="Optional description"
              />
            </div>
            <div className="space-y-2">
              <label className="text-xs font-medium text-muted-foreground uppercase tracking-wider">
                Expires At
              </label>
              <Input
                type="date"
                value={expiresAt}
                onChange={e => setExpiresAt(e.target.value)}
                min={new Date().toISOString().split('T')[0]}
              />
              <p className="text-xs text-text-muted">
                Leave empty for no expiration
              </p>
            </div>
            <DialogFooter>
              <Button
                type="button"
                variant="outline"
                onClick={() => setShowForm(false)}
              >
                Cancel
              </Button>
              <Button type="submit" disabled={createToken.isPending || !name}>
                {createToken.isPending && (
                  <Loader2 className="h-4 w-4 animate-spin mr-2" />
                )}
                Create Token
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
            <DialogTitle>Edit API Token</DialogTitle>
            <DialogDescription>
              Update the token settings. The token value cannot be changed.
            </DialogDescription>
          </DialogHeader>
          <form onSubmit={handleUpdate} className="space-y-4">
            <div className="space-y-2">
              <label className="text-xs font-medium text-muted-foreground uppercase tracking-wider">
                Name *
              </label>
              <Input
                value={name}
                onChange={e => setName(e.target.value)}
                required
              />
            </div>
            <div className="space-y-2">
              <label className="text-xs font-medium text-muted-foreground uppercase tracking-wider">
                Description
              </label>
              <Input
                value={description}
                onChange={e => setDescription(e.target.value)}
              />
            </div>
            <div className="space-y-2">
              <label className="text-xs font-medium text-muted-foreground uppercase tracking-wider">
                Project
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
                    Not Specified
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
              <label className="text-xs font-medium text-muted-foreground uppercase tracking-wider">
                Expires At
              </label>
              <Input
                type="date"
                value={expiresAt}
                onChange={e => setExpiresAt(e.target.value)}
                min={new Date().toISOString().split('T')[0]}
              />
            </div>
            <DialogFooter>
              <Button
                type="button"
                variant="outline"
                onClick={() => setEditingToken(null)}
              >
                Cancel
              </Button>
              <Button type="submit" disabled={updateToken.isPending || !name}>
                {updateToken.isPending && (
                  <Loader2 className="h-4 w-4 animate-spin mr-2" />
                )}
                Save Changes
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
            <DialogTitle>Delete API Token</DialogTitle>
            <DialogDescription>
              Are you sure you want to delete "{deletingToken?.name}"? This
              action cannot be undone. Any applications using this token will no
              longer be able to authenticate.
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={() => setDeletingToken(null)}>
              Cancel
            </Button>
            <Button
              variant="destructive"
              onClick={handleDelete}
              disabled={deleteToken.isPending}
            >
              {deleteToken.isPending && (
                <Loader2 className="h-4 w-4 animate-spin mr-2" />
              )}
              Delete
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
            <DialogTitle>Token Created Successfully</DialogTitle>
            <DialogDescription>
              Copy your new API token now. You won't be able to see it again!
            </DialogDescription>
          </DialogHeader>
          <div className="space-y-4">
            <div className="space-y-2">
              <label className="text-xs font-medium text-muted-foreground uppercase tracking-wider">
                Token Name
              </label>
              <p className="font-medium">{newTokenDialog?.name}</p>
            </div>
            <div className="space-y-2">
              <label className="text-xs font-medium text-muted-foreground uppercase tracking-wider">
                API Token
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
                <strong>Important:</strong> This token will only be shown once.
                Make sure to copy it now.
              </p>
            </div>
          </div>
          <DialogFooter>
            <Button onClick={() => setNewTokenDialog(null)}>Done</Button>
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
            <DialogTitle>Select Project</DialogTitle>
            <DialogDescription>
              Choose a project to limit this token's access.
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
              <p className="text-sm text-text-muted text-center py-4">
                No projects available
              </p>
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
              Clear Selection
            </Button>
            <Button
              variant="secondary"
              onClick={() => setShowProjectPicker(false)}
            >
              Cancel
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  )
}
