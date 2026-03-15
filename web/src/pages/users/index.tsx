import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  Button,
  Card,
  CardContent,
  Input,
  Badge,
  Label,
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
  useUsers,
  useCreateUser,
  useUpdateUser,
  useDeleteUser,
  useApproveUser,
} from '@/hooks/queries';
import { Plus, Loader2, Pencil, Trash2, UserCog, Check } from 'lucide-react';
import { FieldError } from '@/components/field-error';
import { PageHeader } from '@/components/layout';
import { PasswordInput } from '@/components/password-input';
import { PasswordRulesPopover } from '@/components/auth/password-rules-popover';
import {
  getManagedPasswordError,
  getManagedPasswordRuleState,
  isPasswordPolicyViolationResponse,
} from '@/lib/managed-password';
import type { User, UserRole, UserStatus } from '@/lib/transport';
import { useDialog } from '@/contexts/dialog-context';

type CreateUserField = 'username' | 'password' | 'confirmPassword';

export function UsersPage() {
  const { t } = useTranslation();
  const { confirm } = useDialog();
  const { data: users, isLoading } = useUsers();
  const createUser = useCreateUser();
  const updateUser = useUpdateUser();
  const deleteUser = useDeleteUser();
  const approveUser = useApproveUser();

  const [showCreateDialog, setShowCreateDialog] = useState(false);
  const [editingUser, setEditingUser] = useState<User | null>(null);
  const [formData, setFormData] = useState({
    username: '',
    password: '',
    confirmPassword: '',
    role: 'member' as UserRole,
    status: 'active' as UserStatus,
  });
  const [createFieldErrors, setCreateFieldErrors] = useState<
    Partial<Record<CreateUserField, string>>
  >({});
  const [createFormError, setCreateFormError] = useState('');
  const [showCreatePasswordRules, setShowCreatePasswordRules] = useState(false);
  const [createPasswordsVisible, setCreatePasswordsVisible] = useState(false);

  const resetForm = () => {
    setFormData({
      username: '',
      password: '',
      confirmPassword: '',
      role: 'member',
      status: 'active',
    });
  };

  const resetCreateDialogState = () => {
    resetForm();
    setCreateFieldErrors({});
    setCreateFormError('');
    setShowCreatePasswordRules(false);
    setCreatePasswordsVisible(false);
    createUser.reset();
  };

  const createPasswordRuleState = getManagedPasswordRuleState(formData.password);
  const createPasswordInvalidMessage = t('login.passwordFormatInvalid');
  const createPasswordFormatError = getManagedPasswordError(
    formData.password,
    createPasswordInvalidMessage,
  );
  const createPasswordFieldError =
    createFieldErrors.password === createPasswordFormatError ? undefined : createFieldErrors.password;
  const isCreateSubmitDisabled =
    createUser.isPending ||
    !formData.username.trim() ||
    !formData.password.trim() ||
    !formData.confirmPassword.trim() ||
    !!createPasswordFormatError ||
    formData.password !== formData.confirmPassword;

  const handleCreate = async () => {
    setCreateFieldErrors({});
    setCreateFormError('');

    const nextErrors: Partial<Record<CreateUserField, string>> = {};
    if (!formData.username.trim()) {
      nextErrors.username = t('login.usernameRequired');
    }
    if (!formData.password.trim()) {
      nextErrors.password = t('login.passwordRequired');
    }
    if (createPasswordFormatError) {
      setShowCreatePasswordRules(true);
    }
    if (!formData.confirmPassword.trim()) {
      nextErrors.confirmPassword = t('login.confirmPasswordRequired');
    }
    if (
      formData.password &&
      formData.confirmPassword &&
      formData.password !== formData.confirmPassword
    ) {
      nextErrors.confirmPassword = t('users.passwordMismatch');
    }

    if (Object.keys(nextErrors).length > 0) {
      setCreateFieldErrors(nextErrors);
      return;
    }

    try {
      await createUser.mutateAsync({
        username: formData.username.trim(),
        password: formData.password,
        role: formData.role,
      });
      setShowCreateDialog(false);
      resetCreateDialogState();
    } catch (err: unknown) {
      const axiosError = err as { response?: { data?: { error?: string; code?: string } } };
      const errorData = axiosError?.response?.data;
      const errorMsg = errorData?.error;

      if (errorMsg === 'username and password are required') {
        setCreateFormError(t('login.registerFailed'));
        return;
      }
      if (errorMsg === 'user already exists or invalid data') {
        setCreateFieldErrors({ username: t('login.usernameExists') });
        return;
      }
      if (isPasswordPolicyViolationResponse(errorData)) {
        setShowCreatePasswordRules(true);
        return;
      }
      if (errorMsg) {
        setCreateFormError(errorMsg);
      }
    }
  };

  const handleUpdate = async () => {
    if (!editingUser) return;
    try {
      await updateUser.mutateAsync({
        id: editingUser.id,
        data: {
          username: formData.username,
          role: formData.role,
          status: formData.status,
        },
      });
      setEditingUser(null);
      resetForm();
    } catch {
      // Error handled by mutation
    }
  };

  const handleDelete = async (id: number) => {
    const confirmed = await confirm({
      title: t('common.confirm'),
      description: t('users.deleteConfirm'),
      confirmText: t('common.delete'),
      confirmVariant: 'destructive',
    });
    if (!confirmed) return;

    try {
      await deleteUser.mutateAsync(id);
    } catch {
      // Error handled by mutation
    }
  };

  const handleApprove = async (id: number) => {
    try {
      await approveUser.mutateAsync(id);
    } catch {
      // Error handled by mutation
    }
  };

  const openEditDialog = (user: User) => {
    setEditingUser(user);
    setFormData({
      username: user.username,
      password: '',
      confirmPassword: '',
      role: user.role,
      status: user.status || 'active',
    });
  };

  const formatDate = (dateStr?: string) => {
    if (!dateStr) return t('users.never');
    return new Date(dateStr).toLocaleString();
  };

  if (isLoading) {
    return (
      <div className="flex h-64 items-center justify-center">
        <Loader2 className="h-8 w-8 animate-spin text-muted-foreground" />
      </div>
    );
  }

  return (
    <div className="flex h-full min-h-0 flex-col">
      <PageHeader
        title={t('users.title')}
        description={t('users.description')}
        icon={UserCog}
        actions={
          <Button
            onClick={() => {
              resetCreateDialogState();
              setShowCreateDialog(true);
            }}
          >
            <Plus className="mr-2 h-4 w-4" />
            {t('users.addUser')}
          </Button>
        }
      />

      <div className="flex-1 min-h-0 overflow-y-auto">
        <Card className="m-6">
          <CardContent className="p-0">
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>{t('users.username')}</TableHead>
                  <TableHead>{t('users.role')}</TableHead>
                  <TableHead>{t('users.status')}</TableHead>
                  <TableHead>{t('users.lastLogin')}</TableHead>
                  <TableHead className="w-[140px]">
                    <span className="sr-only">{t('common.actions')}</span>
                  </TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {users?.map((user) => (
                  <TableRow key={user.id}>
                    <TableCell className="font-medium">
                      {user.username}
                      {user.isDefault && (
                        <Badge variant="outline" className="ml-2">
                          {t('users.defaultUser')}
                        </Badge>
                      )}
                    </TableCell>
                    <TableCell>
                      <Badge variant={user.role === 'admin' ? 'default' : 'secondary'}>
                        {user.role === 'admin' ? t('users.roleAdmin') : t('users.roleMember')}
                      </Badge>
                    </TableCell>
                    <TableCell>
                      <Badge
                        variant={user.status === 'active' ? 'default' : 'outline'}
                        className={
                          user.status === 'active'
                            ? 'bg-green-500/10 text-green-700 dark:text-green-400 border-green-500/20'
                            : 'bg-yellow-500/10 text-yellow-700 dark:text-yellow-400 border-yellow-500/20'
                        }
                      >
                        {user.status === 'active'
                          ? t('users.statusActive')
                          : t('users.statusPending')}
                      </Badge>
                    </TableCell>
                    <TableCell className="text-muted-foreground text-sm">
                      {formatDate(user.lastLoginAt)}
                    </TableCell>
                    <TableCell>
                      <div className="flex items-center gap-1">
                        {user.status === 'pending' && (
                          <Button
                            variant="ghost"
                            size="icon"
                            onClick={() => handleApprove(user.id)}
                            title={t('users.approve')}
                            aria-label={t('users.approve')}
                            disabled={approveUser.isPending}
                          >
                            <Check className="h-4 w-4 text-green-600" />
                          </Button>
                        )}
                        <Button
                          variant="ghost"
                          size="icon"
                          onClick={() => openEditDialog(user)}
                          title={t('users.editUser')}
                          aria-label={`${t('users.editUser')}: ${user.username}`}
                        >
                          <Pencil className="h-4 w-4" />
                        </Button>
                        {!user.isDefault && (
                          <Button
                            variant="ghost"
                            size="icon"
                            onClick={() => handleDelete(user.id)}
                            title={t('common.delete')}
                            aria-label={`${t('common.delete')}: ${user.username}`}
                            disabled={deleteUser.isPending}
                          >
                            <Trash2 className="h-4 w-4" />
                          </Button>
                        )}
                      </div>
                    </TableCell>
                  </TableRow>
                ))}
                {(!users || users.length === 0) && (
                  <TableRow>
                    <TableCell colSpan={5} className="text-center text-muted-foreground py-8">
                      {t('common.noData')}
                    </TableCell>
                  </TableRow>
                )}
              </TableBody>
            </Table>
          </CardContent>
        </Card>
      </div>

      {/* Create User Dialog */}
      <Dialog
        open={showCreateDialog}
        onOpenChange={(open) => {
          setShowCreateDialog(open);
          if (!open) {
            resetCreateDialogState();
          }
        }}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t('users.addUser')}</DialogTitle>
            <DialogDescription>{t('users.description')}</DialogDescription>
          </DialogHeader>
          <div className="space-y-4 py-4">
            {createFormError && (
              <div className="rounded-lg border border-destructive/30 bg-destructive/10 px-3 py-2 text-sm text-destructive">
                {createFormError}
              </div>
            )}
            <div className="space-y-2">
              <Label htmlFor="create-user-username">
                {t('users.username')}
              </Label>
              <Input
                id="create-user-username"
                value={formData.username}
                aria-invalid={createFieldErrors.username ? 'true' : undefined}
                onChange={(e) => {
                  setFormData({ ...formData, username: e.target.value });
                  setCreateFieldErrors((current) => ({ ...current, username: undefined }));
                  setCreateFormError('');
                }}
                placeholder={t('users.username')}
              />
              <FieldError message={createFieldErrors.username} />
            </div>
            <div className="space-y-2">
              <Label htmlFor="create-user-password">
                {t('users.password')}
              </Label>
              <div className="relative">
                <PasswordInput
                  id="create-user-password"
                  value={formData.password}
                  placeholder={t('users.password')}
                  autoComplete="new-password"
                  aria-invalid={createFieldErrors.password ? 'true' : undefined}
                  onFocus={() => setShowCreatePasswordRules(true)}
                  onBlur={() => setShowCreatePasswordRules(false)}
                  onChange={(e) => {
                    const nextPassword = e.target.value;
                    const nextPasswordError = getManagedPasswordError(
                      nextPassword,
                      createPasswordInvalidMessage,
                    );
                    setFormData({
                      ...formData,
                      password: nextPassword,
                    });
                    setShowCreatePasswordRules(true);
                    setCreateFieldErrors((current) => ({
                      ...current,
                      password: nextPasswordError,
                      confirmPassword:
                        formData.confirmPassword && nextPassword !== formData.confirmPassword
                          ? t('users.passwordMismatch')
                          : undefined,
                    }));
                    setCreateFormError('');
                  }}
                  visible={createPasswordsVisible}
                  onVisibleChange={setCreatePasswordsVisible}
                />
                <PasswordRulesPopover
                  open={showCreatePasswordRules}
                  ruleState={createPasswordRuleState}
                  title={t('login.passwordChecklistTitle')}
                  progressLabel={t('login.passwordCategoryProgress', {
                    count: createPasswordRuleState.categoryCount,
                  })}
                  minLengthLabel={t('login.passwordRuleMinLength')}
                  numberLabel={t('login.passwordRuleNumber')}
                  letterLabel={t('login.passwordRuleLetter')}
                  punctuationLabel={t('login.passwordRulePunctuation')}
                />
              </div>
              <FieldError message={createPasswordFieldError} />
            </div>
            <div className="space-y-2">
              <Label htmlFor="create-user-confirm-password">
                {t('login.confirmPasswordLabel')}
              </Label>
              <PasswordInput
                id="create-user-confirm-password"
                value={formData.confirmPassword}
                placeholder={t('login.confirmPasswordPlaceholder')}
                autoComplete="new-password"
                aria-invalid={createFieldErrors.confirmPassword ? 'true' : undefined}
                onChange={(e) => {
                  const nextConfirmPassword = e.target.value;
                  setFormData({ ...formData, confirmPassword: nextConfirmPassword });
                  setCreateFieldErrors((current) => ({
                    ...current,
                    confirmPassword:
                      nextConfirmPassword && formData.password !== nextConfirmPassword
                        ? t('users.passwordMismatch')
                        : undefined,
                  }));
                  setCreateFormError('');
                }}
                visible={createPasswordsVisible}
                onVisibleChange={setCreatePasswordsVisible}
              />
              <FieldError message={createFieldErrors.confirmPassword} />
            </div>
            <div className="space-y-2">
              <Label htmlFor="create-user-role">
                {t('users.role')}
              </Label>
              <select
                id="create-user-role"
                className="flex h-9 w-full rounded-md border border-input bg-transparent px-3 py-1 text-sm shadow-sm focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring"
                value={formData.role}
                onChange={(e) => setFormData({ ...formData, role: e.target.value as UserRole })}
              >
                <option value="admin">{t('users.roleAdmin')}</option>
                <option value="member">{t('users.roleMember')}</option>
              </select>
            </div>
          </div>
          <DialogFooter>
            <Button
              variant="outline"
              onClick={() => {
                setShowCreateDialog(false);
                resetCreateDialogState();
              }}
            >
              {t('common.cancel')}
            </Button>
            <Button onClick={handleCreate} disabled={isCreateSubmitDisabled}>
              {createUser.isPending && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
              {t('users.addUser')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Edit User Dialog */}
      <Dialog open={!!editingUser} onOpenChange={(open) => !open && setEditingUser(null)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t('users.editUser')}</DialogTitle>
            <DialogDescription>{t('users.description')}</DialogDescription>
          </DialogHeader>
          <div className="space-y-4 py-4">
            <div className="space-y-2">
              <label className="text-sm font-medium" htmlFor="edit-user-username">
                {t('users.username')}
              </label>
              <Input
                id="edit-user-username"
                value={formData.username}
                onChange={(e) => setFormData({ ...formData, username: e.target.value })}
                placeholder={t('users.username')}
              />
            </div>
            <div className="space-y-2">
              <label className="text-sm font-medium" htmlFor="edit-user-role">
                {t('users.role')}
              </label>
              <select
                id="edit-user-role"
                className="flex h-9 w-full rounded-md border border-input bg-transparent px-3 py-1 text-sm shadow-sm focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring"
                value={formData.role}
                onChange={(e) => setFormData({ ...formData, role: e.target.value as UserRole })}
              >
                <option value="admin">{t('users.roleAdmin')}</option>
                <option value="member">{t('users.roleMember')}</option>
              </select>
            </div>
            <div className="space-y-2">
              <label className="text-sm font-medium" htmlFor="edit-user-status">
                {t('users.status')}
              </label>
              <select
                id="edit-user-status"
                className="flex h-9 w-full rounded-md border border-input bg-transparent px-3 py-1 text-sm shadow-sm focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring"
                value={formData.status}
                onChange={(e) => setFormData({ ...formData, status: e.target.value as UserStatus })}
              >
                <option value="active">{t('users.statusActive')}</option>
                <option value="pending">{t('users.statusPending')}</option>
              </select>
            </div>
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setEditingUser(null)}>
              {t('common.cancel')}
            </Button>
            <Button onClick={handleUpdate} disabled={!formData.username || updateUser.isPending}>
              {updateUser.isPending && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
              {t('users.editUser')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}
