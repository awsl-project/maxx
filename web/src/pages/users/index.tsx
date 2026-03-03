import { useState } from 'react';
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
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui';
import {
  useUsers,
  useCreateUser,
  useUpdateUser,
  useDeleteUser,
} from '@/hooks/queries';
import {
  Plus,
  Loader2,
  Pencil,
  Trash2,
  UserCog,
} from 'lucide-react';
import { PageHeader } from '@/components/layout';
import type { User, UserRole } from '@/lib/transport';

export function UsersPage() {
  const { t } = useTranslation();
  const { data: users, isLoading } = useUsers();
  const createUser = useCreateUser();
  const updateUser = useUpdateUser();
  const deleteUser = useDeleteUser();

  const [showCreateDialog, setShowCreateDialog] = useState(false);
  const [editingUser, setEditingUser] = useState<User | null>(null);
  const [formData, setFormData] = useState({
    username: '',
    password: '',
    role: 'member' as UserRole,
  });

  const resetForm = () => {
    setFormData({ username: '', password: '', role: 'member' });
  };

  const handleCreate = async () => {
    try {
      await createUser.mutateAsync({
        username: formData.username,
        password: formData.password,
        role: formData.role,
      });
      setShowCreateDialog(false);
      resetForm();
    } catch {
      // Error handled by mutation
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
        },
      });
      setEditingUser(null);
      resetForm();
    } catch {
      // Error handled by mutation
    }
  };

  const handleDelete = async (id: number) => {
    if (!window.confirm(t('users.deleteConfirm'))) return;
    try {
      await deleteUser.mutateAsync(id);
    } catch {
      // Error handled by mutation
    }
  };

  const openEditDialog = (user: User) => {
    setEditingUser(user);
    setFormData({
      username: user.username,
      password: '',
      role: user.role,
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
    <div className="space-y-6">
      <PageHeader
        title={t('users.title')}
        description={t('users.description')}
        icon={UserCog}
        actions={
          <Button onClick={() => { resetForm(); setShowCreateDialog(true); }}>
            <Plus className="mr-2 h-4 w-4" />
            {t('users.addUser')}
          </Button>
        }
      />

      <Card>
        <CardContent className="p-0">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>{t('users.username')}</TableHead>
                <TableHead>{t('users.role')}</TableHead>
                <TableHead>{t('users.lastLogin')}</TableHead>
                <TableHead className="w-[100px]" />
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
                  <TableCell className="text-muted-foreground text-sm">
                    {formatDate(user.lastLoginAt)}
                  </TableCell>
                  <TableCell>
                    <div className="flex items-center gap-1">
                      <Button
                        variant="ghost"
                        size="icon"
                        onClick={() => openEditDialog(user)}
                      >
                        <Pencil className="h-4 w-4" />
                      </Button>
                      {!user.isDefault && (
                        <Button
                          variant="ghost"
                          size="icon"
                          onClick={() => handleDelete(user.id)}
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
                  <TableCell colSpan={4} className="text-center text-muted-foreground py-8">
                    No users found
                  </TableCell>
                </TableRow>
              )}
            </TableBody>
          </Table>
        </CardContent>
      </Card>

      {/* Create User Dialog */}
      <Dialog open={showCreateDialog} onOpenChange={setShowCreateDialog}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t('users.addUser')}</DialogTitle>
            <DialogDescription>{t('users.description')}</DialogDescription>
          </DialogHeader>
          <div className="space-y-4 py-4">
            <div className="space-y-2">
              <label className="text-sm font-medium">{t('users.username')}</label>
              <Input
                value={formData.username}
                onChange={(e) => setFormData({ ...formData, username: e.target.value })}
                placeholder={t('users.username')}
              />
            </div>
            <div className="space-y-2">
              <label className="text-sm font-medium">{t('users.password')}</label>
              <Input
                type="password"
                value={formData.password}
                onChange={(e) => setFormData({ ...formData, password: e.target.value })}
                placeholder={t('users.password')}
              />
            </div>
            <div className="space-y-2">
              <label className="text-sm font-medium">{t('users.role')}</label>
              <select
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
            <Button variant="outline" onClick={() => setShowCreateDialog(false)}>
              Cancel
            </Button>
            <Button
              onClick={handleCreate}
              disabled={!formData.username || !formData.password || createUser.isPending}
            >
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
              <label className="text-sm font-medium">{t('users.username')}</label>
              <Input
                value={formData.username}
                onChange={(e) => setFormData({ ...formData, username: e.target.value })}
                placeholder={t('users.username')}
              />
            </div>
            <div className="space-y-2">
              <label className="text-sm font-medium">{t('users.role')}</label>
              <select
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
            <Button variant="outline" onClick={() => setEditingUser(null)}>
              Cancel
            </Button>
            <Button
              onClick={handleUpdate}
              disabled={!formData.username || updateUser.isPending}
            >
              {updateUser.isPending && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
              {t('users.editUser')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}
