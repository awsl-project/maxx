import { useEffect, useMemo, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  Button,
  Card,
  CardContent,
  Input,
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
  DialogHeader,
  DialogTitle,
} from '@/components/ui';
import {
  useInviteCodes,
  useCreateInviteCodes,
  useUpdateInviteCode,
  useDeleteInviteCode,
  useInviteCodeUsages,
} from '@/hooks/queries';
import { PageHeader } from '@/components/layout';
import { Plus, Loader2, Copy, Eye, Ban, Check, Trash2, Ticket } from 'lucide-react';
import type { InviteCode, InviteCodeCreateItem } from '@/lib/transport';

export function InviteCodesPage() {
  const { t } = useTranslation();
  const { data: codes, isLoading } = useInviteCodes();
  const createInviteCodes = useCreateInviteCodes();
  const updateInviteCode = useUpdateInviteCode();
  const deleteInviteCode = useDeleteInviteCode();

  const [showCreateDialog, setShowCreateDialog] = useState(false);
  const [newCodesDialog, setNewCodesDialog] = useState<InviteCodeCreateItem[] | null>(null);
  const [usageDialogCode, setUsageDialogCode] = useState<InviteCode | null>(null);
  const [copiedAll, setCopiedAll] = useState(false);
  const copiedTimerRef = useRef<number | null>(null);

  const { data: usages, isLoading: usagesLoading } = useInviteCodeUsages(usageDialogCode?.id ?? 0);

  const [formData, setFormData] = useState({
    count: '1',
    maxUses: '1',
    expiresAt: '',
    note: '',
  });

  useEffect(
    () => () => {
      if (copiedTimerRef.current) {
        window.clearTimeout(copiedTimerRef.current);
      }
    },
    [],
  );

  const resetForm = () => {
    setFormData({ count: '1', maxUses: '1', expiresAt: '', note: '' });
  };

  const handleCreate = async () => {
    const count = parseInt(formData.count || '1', 10);
    const maxUses = formData.maxUses === '' ? undefined : parseInt(formData.maxUses, 10);
    createInviteCodes.mutate(
      {
        count: Number.isNaN(count) ? 1 : count,
        maxUses: Number.isNaN(maxUses ?? 1) ? 1 : maxUses,
        expiresAt: formData.expiresAt ? new Date(formData.expiresAt).toISOString() : undefined,
        note: formData.note.trim() || undefined,
      },
      {
        onSuccess: (result) => {
          setShowCreateDialog(false);
          setNewCodesDialog(result.items);
          resetForm();
        },
      },
    );
  };

  const handleToggleStatus = (code: InviteCode) => {
    const nextStatus = code.status === 'active' ? 'disabled' : 'active';
    updateInviteCode.mutate({ id: code.id, data: { status: nextStatus } });
  };

  const handleDelete = (code: InviteCode) => {
    if (!window.confirm(t('inviteCodes.deleteConfirm'))) return;
    deleteInviteCode.mutate(code.id);
  };

  const copyCodes = async () => {
    if (!newCodesDialog) return;
    const text = newCodesDialog.map((item) => item.code).join('\n');
    if (typeof navigator === 'undefined' || !navigator.clipboard?.writeText) {
      console.error('Clipboard API is not available.');
      return;
    }
    try {
      await navigator.clipboard.writeText(text);
      setCopiedAll(true);
      if (copiedTimerRef.current) {
        window.clearTimeout(copiedTimerRef.current);
      }
      copiedTimerRef.current = window.setTimeout(() => {
        setCopiedAll(false);
        copiedTimerRef.current = null;
      }, 2000);
    } catch (error) {
      console.error('Failed to copy invite codes.', error);
    }
  };

  const formatDate = (value?: string) => {
    if (!value) return t('inviteCodes.never');
    return new Date(value).toLocaleString();
  };

  const maxUsesLabel = (code: InviteCode) => {
    if (!code.maxUses) return t('inviteCodes.unlimited');
    return `${code.usedCount}/${code.maxUses}`;
  };

  const isBusy = useMemo(
    () => createInviteCodes.isPending || updateInviteCode.isPending || deleteInviteCode.isPending,
    [createInviteCodes.isPending, updateInviteCode.isPending, deleteInviteCode.isPending],
  );

  if (isLoading) {
    return (
      <div className="flex h-64 items-center justify-center">
        <Loader2 className="h-8 w-8 animate-spin text-muted-foreground" />
      </div>
    );
  }

  return (
    <div className="flex h-full flex-col">
      <PageHeader
        title={t('inviteCodes.title')}
        description={t('inviteCodes.description')}
        icon={Ticket}
        iconClassName="text-amber-500"
        actions={(
          <Button onClick={() => setShowCreateDialog(true)}>
            <Plus className="mr-2 h-4 w-4" />
            {t('inviteCodes.create')}
          </Button>
        )}
      />

      <div className="flex-1 overflow-y-auto">
        <div className="space-y-6">
          <Card className="m-6">
            <CardContent className="p-0">
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>{t('inviteCodes.codePrefix')}</TableHead>
                    <TableHead>{t('inviteCodes.status')}</TableHead>
                    <TableHead>{t('inviteCodes.usage')}</TableHead>
                    <TableHead>{t('inviteCodes.expiresAt')}</TableHead>
                    <TableHead>{t('inviteCodes.note')}</TableHead>
                    <TableHead className="w-[140px]" />
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {codes?.map((code) => (
                    <TableRow key={code.id}>
                      <TableCell className="font-medium">{code.codePrefix}</TableCell>
                      <TableCell>
                        <Badge variant={code.status === 'active' ? 'default' : 'outline'}>
                          {code.status === 'active' ? t('inviteCodes.statusActive') : t('inviteCodes.statusDisabled')}
                        </Badge>
                      </TableCell>
                      <TableCell>{maxUsesLabel(code)}</TableCell>
                      <TableCell className="text-muted-foreground text-sm">
                        {formatDate(code.expiresAt)}
                      </TableCell>
                      <TableCell className="text-muted-foreground text-sm">
                        {code.note || '-'}
                      </TableCell>
                      <TableCell>
                        <div className="flex items-center gap-1">
                          <Button
                            variant="ghost"
                            size="icon"
                            onClick={() => setUsageDialogCode(code)}
                            title={t('inviteCodes.viewUsages')}
                            aria-label={t('inviteCodes.viewUsages')}
                            disabled={isBusy}
                          >
                            <Eye className="h-4 w-4" />
                          </Button>
                          <Button
                            variant="ghost"
                            size="icon"
                            onClick={() => handleToggleStatus(code)}
                            title={t('inviteCodes.toggleStatus')}
                            aria-label={t('inviteCodes.toggleStatus')}
                            disabled={isBusy}
                          >
                            {code.status === 'active' ? <Ban className="h-4 w-4" /> : <Check className="h-4 w-4" />}
                          </Button>
                          <Button
                            variant="ghost"
                            size="icon"
                            onClick={() => handleDelete(code)}
                            title={`Delete invite code ${code.codePrefix}`}
                            aria-label={`Delete invite code ${code.codePrefix}`}
                            disabled={isBusy}
                          >
                            <Trash2 className="h-4 w-4" />
                          </Button>
                        </div>
                      </TableCell>
                    </TableRow>
                  ))}
                  {(!codes || codes.length === 0) && (
                    <TableRow>
                      <TableCell colSpan={6} className="text-center text-muted-foreground py-8">
                        {t('common.noData')}
                      </TableCell>
                    </TableRow>
                  )}
                </TableBody>
              </Table>
            </CardContent>
          </Card>
        </div>
      </div>

      <Dialog open={showCreateDialog} onOpenChange={setShowCreateDialog}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t('inviteCodes.create')}</DialogTitle>
            <DialogDescription>{t('inviteCodes.createDescription')}</DialogDescription>
          </DialogHeader>
          <div className="space-y-4 py-4">
            <div className="space-y-2">
              <label className="text-sm font-medium">{t('inviteCodes.count')}</label>
              <Input
                type="number"
                min={1}
                max={100}
                value={formData.count}
                onChange={(e) => setFormData({ ...formData, count: e.target.value })}
              />
            </div>
            <div className="space-y-2">
              <label className="text-sm font-medium">{t('inviteCodes.maxUses')}</label>
              <Input
                type="number"
                min={0}
                value={formData.maxUses}
                onChange={(e) => setFormData({ ...formData, maxUses: e.target.value })}
              />
              <p className="text-xs text-muted-foreground">{t('inviteCodes.maxUsesHint')}</p>
            </div>
            <div className="space-y-2">
              <label className="text-sm font-medium">{t('inviteCodes.expiresAt')}</label>
              <Input
                type="datetime-local"
                value={formData.expiresAt}
                onChange={(e) => setFormData({ ...formData, expiresAt: e.target.value })}
              />
            </div>
            <div className="space-y-2">
              <label className="text-sm font-medium">{t('inviteCodes.note')}</label>
              <Input
                value={formData.note}
                onChange={(e) => setFormData({ ...formData, note: e.target.value })}
              />
            </div>
          </div>
          <div className="flex justify-end gap-2">
            <Button variant="ghost" onClick={() => { setShowCreateDialog(false); resetForm(); }}>
              {t('common.cancel')}
            </Button>
            <Button onClick={handleCreate} disabled={createInviteCodes.isPending}>
              {createInviteCodes.isPending ? t('common.loading') : t('inviteCodes.create')}
            </Button>
          </div>
        </DialogContent>
      </Dialog>

      <Dialog open={!!newCodesDialog} onOpenChange={() => setNewCodesDialog(null)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t('inviteCodes.created')}</DialogTitle>
            <DialogDescription>{t('inviteCodes.createdDescription')}</DialogDescription>
          </DialogHeader>
          <div className="space-y-2">
            <Button variant="secondary" onClick={copyCodes} className="w-full">
              {copiedAll ? (
                <Check className="mr-2 h-4 w-4 text-green-500" />
              ) : (
                <Copy className="mr-2 h-4 w-4" />
              )}
              {t('inviteCodes.copyAll')}
            </Button>
            <div className="max-h-64 overflow-auto rounded border p-3 text-sm">
              {newCodesDialog?.map((item) => (
                <div key={item.inviteCode.id} className="font-mono">
                  {item.code}
                </div>
              ))}
            </div>
          </div>
        </DialogContent>
      </Dialog>

      <Dialog open={!!usageDialogCode} onOpenChange={() => setUsageDialogCode(null)}>
        <DialogContent className="max-w-2xl">
          <DialogHeader>
            <DialogTitle>{t('inviteCodes.usagesTitle')}</DialogTitle>
            <DialogDescription>
              {usageDialogCode ? `${t('inviteCodes.codePrefix')}: ${usageDialogCode.codePrefix}` : ''}
            </DialogDescription>
          </DialogHeader>
          <div className="max-h-80 overflow-auto">
            {usagesLoading ? (
              <div className="flex h-24 items-center justify-center">
                <Loader2 className="h-6 w-6 animate-spin text-muted-foreground" />
              </div>
            ) : (
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>{t('inviteCodes.usageUser')}</TableHead>
                    <TableHead>{t('inviteCodes.usageTime')}</TableHead>
                    <TableHead>{t('inviteCodes.usageIP')}</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {usages?.map((usage) => (
                    <TableRow key={usage.id}>
                      <TableCell>{usage.username || '-'}</TableCell>
                      <TableCell>{formatDate(usage.usedAt)}</TableCell>
                      <TableCell className="text-muted-foreground text-sm">{usage.ip || '-'}</TableCell>
                    </TableRow>
                  ))}
                  {(!usages || usages.length === 0) && (
                    <TableRow>
                      <TableCell colSpan={3} className="text-center text-muted-foreground py-6">
                        {t('common.noData')}
                      </TableCell>
                    </TableRow>
                  )}
                </TableBody>
              </Table>
            )}
          </div>
        </DialogContent>
      </Dialog>

    </div>
  );
}
