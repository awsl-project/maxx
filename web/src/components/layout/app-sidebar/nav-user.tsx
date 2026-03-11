'use client';

import { useState, useRef, useEffect } from 'react';
import { useQueryClient } from '@tanstack/react-query';
import {
  Moon,
  Sun,
  Laptop,
  Sparkles,
  Gem,
  Github,
  ChevronsUp,
  RefreshCw,
  KeyRound,
  Loader2,
  Plus,
  ShieldAlert,
  Trash2,
  CircleUserRound,
  ArrowLeftRight,
  Building2,
  BadgeCheck,
  IdCard,
  Settings2,
  ShieldCheck,
  Power,
} from 'lucide-react';
import { useNavigate } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { useTheme } from '@/components/theme-provider';
import { useTransport } from '@/lib/transport/context';
import { useAuth } from '@/lib/auth-context';
import { getAuthUserDisplay } from '@/lib/auth-user-display';
import {
  userKeys,
  useChangeMyPassword,
  useDeletePasskeyCredential,
  usePasskeyCredentials,
  useRegisterPasskey,
} from '@/hooks/queries';
import type { Theme } from '@/lib/theme';
import { Avatar, AvatarFallback, AvatarImage } from '@/components/ui/avatar';
import {
  Badge,
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { cn } from '@/lib/utils';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
  DropdownMenuGroup,
  DropdownMenuLabel,
  DropdownMenuItem,
  DropdownMenuSub,
  DropdownMenuSubTrigger,
  DropdownMenuSubContent,
  DropdownMenuPortal,
  DropdownMenuRadioGroup,
  DropdownMenuRadioItem,
} from '@/components/ui/dropdown-menu';
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip';
import { SidebarMenu, SidebarMenuItem, useSidebar } from '@/components/ui/sidebar';

export function NavUser() {
  const navigate = useNavigate();
  const { isMobile, state } = useSidebar();
  const { t, i18n } = useTranslation();
  const { transport } = useTransport();
  const { theme, setTheme } = useTheme();
  const queryClient = useQueryClient();
  const { user, authEnabled, isLoading, logout } = useAuth();
  const changePassword = useChangeMyPassword();
  const isCollapsed = !isMobile && state === 'collapsed';
  const hasAuthenticatedUser = authEnabled && Boolean(user) && !isLoading;

  const [showPasskeyDialog, setShowPasskeyDialog] = useState(false);
  const [showAccountDialog, setShowAccountDialog] = useState(false);
  const [passkeyError, setPasskeyError] = useState('');
  const [deletingPasskeyID, setDeletingPasskeyID] = useState<string | null>(null);
  const [showPasswordDialog, setShowPasswordDialog] = useState(false);
  const [passwordForm, setPasswordForm] = useState({
    oldPassword: '',
    newPassword: '',
    confirmPassword: '',
  });
  const [passwordError, setPasswordError] = useState('');
  const [passwordSuccess, setPasswordSuccess] = useState('');
  const passwordTimeoutRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const [passkeySuccess, setPasskeySuccess] = useState('');
  const securityAvailable = hasAuthenticatedUser;
  const passkeyCredentials = usePasskeyCredentials(showPasskeyDialog && securityAvailable);
  const deletePasskeyCredential = useDeletePasskeyCredential();
  const registerPasskey = useRegisterPasskey();

  useEffect(() => {
    return () => {
      if (passwordTimeoutRef.current) {
        clearTimeout(passwordTimeoutRef.current);
      }
    };
  }, []);
  useEffect(() => {
    if (securityAvailable) {
      return;
    }
    setShowPasskeyDialog(false);
    setShowPasswordDialog(false);
    setPasskeyError('');
    setPasskeySuccess('');
    setDeletingPasskeyID(null);
    setPasswordError('');
    setPasswordSuccess('');
    deletePasskeyCredential.reset();
    registerPasskey.reset();
    queryClient.cancelQueries({ queryKey: userKeys.passkeys() });
    queryClient.removeQueries({ queryKey: userKeys.passkeys() });
  }, [securityAvailable, deletePasskeyCredential, registerPasskey, queryClient]);
  const currentLanguage = (i18n.resolvedLanguage || i18n.language || 'en')
    .toLowerCase()
    .startsWith('zh')
    ? 'zh'
    : 'en';
  const currentLanguageLabel =
    currentLanguage === 'zh' ? t('settings.languages.zh') : t('settings.languages.en');
  const desktopRestartAvailable =
    typeof window !== 'undefined' &&
    !!(
      window as unknown as {
        go?: { desktop?: { LauncherApp?: { RestartServer?: () => unknown } } };
      }
    ).go?.desktop?.LauncherApp?.RestartServer;
  const desktopQuitAvailable =
    typeof window !== 'undefined' &&
    !!(
      window as unknown as {
        go?: { desktop?: { LauncherApp?: { Quit?: () => unknown } } };
      }
    ).go?.desktop?.LauncherApp?.Quit;

  const handleToggleLanguage = () => {
    i18n.changeLanguage(currentLanguage === 'zh' ? 'en' : 'zh');
  };

  const handleRestartServer = async () => {
    if (!window.confirm(t('nav.restartServerConfirm'))) return;
    try {
      if (desktopRestartAvailable) {
        const launcher = (
          window as unknown as {
            go?: { desktop?: { LauncherApp?: { RestartServer?: () => Promise<void> } } };
          }
        ).go?.desktop?.LauncherApp;
        if (!launcher?.RestartServer) {
          throw new Error('Desktop restart is unavailable.');
        }
        await launcher.RestartServer();
        return;
      }
      await transport.restartServer();
    } catch (error) {
      console.error('Restart server failed:', error);
      if (typeof window !== 'undefined') {
        window.alert(t('nav.restartServerFailed'));
      }
    }
  };

  const handleQuitApp = async () => {
    if (!desktopQuitAvailable) {
      return;
    }
    try {
      const launcher = (
        window as unknown as {
          go?: { desktop?: { LauncherApp?: { Quit?: () => Promise<void> } } };
        }
      ).go?.desktop?.LauncherApp;
      await launcher?.Quit?.();
    } catch (error) {
      console.error('Quit app failed:', error);
    }
  };

  const handleChangePassword = async () => {
    setPasswordError('');
    setPasswordSuccess('');

    if (passwordForm.newPassword !== passwordForm.confirmPassword) {
      setPasswordError(t('users.passwordMismatch'));
      return;
    }

    try {
      await changePassword.mutateAsync({
        oldPassword: passwordForm.oldPassword,
        newPassword: passwordForm.newPassword,
      });
      setPasswordSuccess(t('users.changePasswordSuccess'));
      setPasswordForm({ oldPassword: '', newPassword: '', confirmPassword: '' });
      if (passwordTimeoutRef.current) {
        clearTimeout(passwordTimeoutRef.current);
      }
      passwordTimeoutRef.current = setTimeout(() => setShowPasswordDialog(false), 1500);
    } catch (err: unknown) {
      const axiosError = err as { response?: { data?: { error?: string } } };
      setPasswordError(axiosError?.response?.data?.error || t('users.changePasswordFailed'));
    }
  };

  const handleDeletePasskey = async (credentialID: string) => {
    if (!window.confirm(t('users.passkeyDeleteConfirm'))) return;

    setPasskeyError('');
    setDeletingPasskeyID(credentialID);
    try {
      await deletePasskeyCredential.mutateAsync(credentialID);
    } catch (err: unknown) {
      const axiosError = err as { response?: { data?: { error?: string } } };
      setPasskeyError(axiosError?.response?.data?.error || t('users.passkeyDeleteFailed'));
    } finally {
      setDeletingPasskeyID(null);
    }
  };

  const handleRegisterPasskey = async () => {
    setPasskeyError('');
    setPasskeySuccess('');
    try {
      await registerPasskey.mutateAsync();
      setPasskeySuccess(t('login.passkeyRegisterSuccess'));
    } catch (err: unknown) {
      const axiosError = err as { response?: { data?: { error?: string } }; message?: string };
      const msg = axiosError?.response?.data?.error || axiosError?.message;
      if (msg === 'PASSKEY_NOT_SUPPORTED') {
        setPasskeyError(t('login.passkeyNotSupported'));
      } else {
        setPasskeyError(msg || t('login.passkeyRegisterFailed'));
      }
    }
  };

  const displayUser = getAuthUserDisplay(user);
  const menuDisplayName = displayUser.maskedIdentity;
  const roleLabel = isLoading
    ? t('common.loading')
    : user
      ? user.role === 'admin'
        ? t('users.roleAdmin')
        : t('users.roleMember')
      : t('common.unknown');
  const accountStatusLabel = isLoading
    ? t('common.loading')
    : hasAuthenticatedUser
      ? t('users.statusActive')
      : authEnabled
        ? t('common.unknown')
        : t('nav.authDisabled');
  const accountTitle = `${displayUser.maskedIdentity} · ${displayUser.tenantLabel}`;
  const detailLine = isLoading
    ? t('common.loading')
    : hasAuthenticatedUser
      ? `${displayUser.tenantLabel} · ${displayUser.userLabel}`
      : t('common.unknown');

  const openUsersPage = () => {
    navigate('/users');
    setShowAccountDialog(false);
  };

  const openSettingsPage = () => {
    navigate('/settings');
    setShowAccountDialog(false);
  };

  return (
    <SidebarMenu>
      <SidebarMenuItem>
        <div
          className="rounded-xl border border-sidebar-border/70 bg-sidebar/70 p-1.5 backdrop-blur-sm space-y-1.5"
        >
          <div className={cn('flex items-center gap-2', isCollapsed ? 'justify-center' : 'justify-between')}>
            <a
              href="https://github.com/awsl-project/maxx"
              target="_blank"
              rel="noopener noreferrer"
              className="inline-flex h-8 w-8 items-center justify-center rounded-lg text-sidebar-foreground/80 transition-colors hover:bg-sidebar-accent hover:text-sidebar-accent-foreground"
              title="GitHub"
            >
              <Github className="h-4 w-4" />
            </a>

            <button
              type="button"
              onClick={handleToggleLanguage}
              title={`${t('nav.language')}: ${currentLanguageLabel}`}
              className={cn(
                'inline-flex items-center rounded-full border border-sidebar-border/70 bg-sidebar-accent/40 p-0.5 text-sidebar-foreground transition-colors hover:bg-sidebar-accent',
                isCollapsed ? 'h-8 w-8 justify-center' : 'h-8 gap-1 px-1',
              )}
            >
              {isCollapsed ? (
                <span className="text-[11px] font-semibold uppercase">
                  {currentLanguage === 'zh' ? '中' : 'EN'}
                </span>
              ) : (
                <span className="inline-flex items-center rounded-full bg-sidebar/70 p-0.5">
                  <span
                    className={cn(
                      'rounded-full px-1.5 py-0.5 text-[10px] font-semibold uppercase transition-colors',
                      currentLanguage === 'zh'
                        ? 'bg-sidebar text-sidebar-foreground shadow-sm'
                        : 'text-sidebar-foreground/55',
                    )}
                  >
                    中
                  </span>
                  <span
                    className={cn(
                      'rounded-full px-1.5 py-0.5 text-[10px] font-semibold uppercase transition-colors',
                      currentLanguage === 'en'
                        ? 'bg-sidebar text-sidebar-foreground shadow-sm'
                        : 'text-sidebar-foreground/55',
                    )}
                  >
                    EN
                  </span>
                </span>
              )}
            </button>
          </div>

          {isCollapsed ? (
            <Tooltip>
              <TooltipTrigger
                render={(props) => (
                  <button
                    {...props}
                    type="button"
                    title={accountTitle}
                    className={cn(
                      'flex w-full items-center justify-center rounded-lg border border-sidebar-border/70 bg-sidebar-accent/25 py-2 text-sidebar-foreground transition-colors hover:bg-sidebar-accent',
                      props.className,
                    )}
                  >
                    <Avatar className="h-8 w-8 rounded-lg">
                      <AvatarImage src="/logo.png" alt={menuDisplayName} />
                      <AvatarFallback className="rounded-lg text-[10px]">
                        {displayUser.avatarFallback}
                      </AvatarFallback>
                    </Avatar>
                  </button>
                )}
              />
              <TooltipContent side={isMobile ? 'top' : 'right'} align="center">
                <div className="space-y-1">
                  <p className="text-xs font-semibold">{displayUser.maskedIdentity}</p>
                  <p className="text-[11px] text-muted-foreground">{detailLine}</p>
                  <p className="text-[11px] text-muted-foreground">
                    {roleLabel} · {accountStatusLabel}
                  </p>
                </div>
              </TooltipContent>
            </Tooltip>
          ) : (
            <div
              className="flex min-w-0 items-center gap-2 rounded-lg border border-sidebar-border/70 bg-sidebar-accent/20 px-2 py-2"
              title={accountTitle}
            >
              <Avatar className="h-9 w-9 rounded-lg">
                <AvatarImage src="/logo.png" alt={menuDisplayName} />
                <AvatarFallback className="rounded-lg text-[10px]">
                  {displayUser.avatarFallback}
                </AvatarFallback>
              </Avatar>
              <div className="min-w-0 flex-1">
                <span className="block truncate text-xs font-semibold">
                  {displayUser.maskedIdentity}
                </span>
                <span className="block truncate text-[11px] text-sidebar-foreground/65">
                  {detailLine}
                </span>
                <div className="mt-1 flex items-center gap-1">
                  <Badge
                    variant={hasAuthenticatedUser ? 'success' : 'secondary'}
                    className="h-4 px-1.5 text-[10px]"
                  >
                    {accountStatusLabel}
                  </Badge>
                  <Badge variant="outline" className="h-4 px-1.5 text-[10px]">
                    {roleLabel}
                  </Badge>
                </div>
              </div>
            </div>
          )}

          <DropdownMenu>
            <DropdownMenuTrigger
              render={(props) => (
                <button
                  {...props}
                  type="button"
                  title="Menu"
                  className={cn(
                    'inline-flex h-8 w-8 items-center justify-center rounded-lg text-sidebar-foreground/80 transition-colors hover:bg-sidebar-accent hover:text-sidebar-accent-foreground',
                    !isCollapsed && 'w-full border border-sidebar-border/70 bg-sidebar-accent/15',
                    props.className,
                  )}
                >
                  <ChevronsUp className="h-4 w-4" />
                </button>
              )}
            />
            <DropdownMenuContent
              className="!w-64 rounded-lg !min-w-0"
              side={isMobile ? 'bottom' : 'right'}
              align="end"
              sideOffset={4}
            >
              <DropdownMenuGroup>
                <DropdownMenuLabel>
                  <div className="flex items-center gap-2 w-full">
                    <Avatar className="h-8 w-8 rounded-lg">
                      <AvatarImage src="/logo.png" alt={menuDisplayName} />
                      <AvatarFallback className="rounded-lg">{displayUser.avatarFallback}</AvatarFallback>
                    </Avatar>
                    <div className="grid flex-1 text-left leading-tight">
                      <span className="truncate font-medium">{menuDisplayName}</span>
                      <span className="truncate text-xs text-muted-foreground">
                        {roleLabel} · {accountStatusLabel}
                      </span>
                      <span className="truncate text-xs text-muted-foreground">{detailLine}</span>
                    </div>
                  </div>
                </DropdownMenuLabel>
                <DropdownMenuSeparator />
              </DropdownMenuGroup>
              <DropdownMenuGroup>
                <DropdownMenuItem onClick={() => setShowAccountDialog(true)}>
                  <CircleUserRound />
                  <span>{t('nav.accountOverview')}</span>
                </DropdownMenuItem>
                <DropdownMenuSub>
                  <DropdownMenuSubTrigger>
                    <ShieldCheck />
                    <span>{t('nav.securityCenter')}</span>
                  </DropdownMenuSubTrigger>
                  <DropdownMenuPortal>
                    <DropdownMenuSubContent>
                      {securityAvailable && (
                        <DropdownMenuItem
                          onClick={() => {
                            setPasskeyError('');
                            setPasskeySuccess('');
                            setShowPasskeyDialog(true);
                          }}
                        >
                          <ShieldAlert />
                          <span>{t('nav.managePasskeys')}</span>
                        </DropdownMenuItem>
                      )}
                      {securityAvailable && (
                        <DropdownMenuItem
                          onClick={() => {
                            setPasswordForm({
                              oldPassword: '',
                              newPassword: '',
                              confirmPassword: '',
                            });
                            setPasswordError('');
                            setPasswordSuccess('');
                            setShowPasswordDialog(true);
                          }}
                        >
                          <KeyRound />
                          <span>{t('nav.changePassword')}</span>
                        </DropdownMenuItem>
                      )}
                      <DropdownMenuItem onClick={openSettingsPage}>
                        <Settings2 />
                        <span>{t('nav.settings')}</span>
                      </DropdownMenuItem>
                    </DropdownMenuSubContent>
                  </DropdownMenuPortal>
                </DropdownMenuSub>
              </DropdownMenuGroup>
              <DropdownMenuSeparator />
              <DropdownMenuGroup>
                <DropdownMenuSub>
                  <DropdownMenuSubTrigger>
                    {theme === 'light' ? (
                      <Sun />
                    ) : theme === 'dark' ? (
                      <Moon />
                    ) : theme === 'hermes' || theme === 'tiffany' ? (
                      <Sparkles />
                    ) : (
                      <Laptop />
                    )}
                    <span>{t('nav.theme')}</span>
                  </DropdownMenuSubTrigger>
                  <DropdownMenuPortal>
                    <DropdownMenuSubContent>
                      <DropdownMenuRadioGroup
                        value={theme}
                        onValueChange={(v) => setTheme(v as Theme)}
                      >
                        <DropdownMenuLabel className="text-xs text-muted-foreground">
                          {t('settings.themeDefault')}
                        </DropdownMenuLabel>
                        <DropdownMenuRadioItem value="light" closeOnClick>
                          <Sun />
                          <span>{t('settings.theme.light')}</span>
                        </DropdownMenuRadioItem>
                        <DropdownMenuRadioItem value="dark" closeOnClick>
                          <Moon />
                          <span>{t('settings.theme.dark')}</span>
                        </DropdownMenuRadioItem>
                        <DropdownMenuRadioItem value="system" closeOnClick>
                          <Laptop />
                          <span>{t('settings.theme.system')}</span>
                        </DropdownMenuRadioItem>
                        <DropdownMenuSeparator />
                        <DropdownMenuLabel className="text-xs text-muted-foreground">
                          {t('settings.themeLuxury')}
                        </DropdownMenuLabel>
                        <DropdownMenuRadioItem value="hermes" closeOnClick>
                          <Sparkles className="text-orange-500" />
                          <span>{t('settings.theme.hermes')}</span>
                        </DropdownMenuRadioItem>
                        <DropdownMenuRadioItem value="tiffany" closeOnClick>
                          <Gem className="text-cyan-500" />
                          <span>{t('settings.theme.tiffany')}</span>
                        </DropdownMenuRadioItem>
                      </DropdownMenuRadioGroup>
                    </DropdownMenuSubContent>
                  </DropdownMenuPortal>
                </DropdownMenuSub>
              </DropdownMenuGroup>
              <DropdownMenuSeparator />
              <DropdownMenuItem onClick={handleRestartServer}>
                <RefreshCw />
                <span>{t('nav.restartServer')}</span>
              </DropdownMenuItem>
              {authEnabled && (
                <DropdownMenuItem
                  onClick={() => {
                    logout();
                  }}
                >
                  <ArrowLeftRight />
                  <span>{t('nav.switchAccount')}</span>
                </DropdownMenuItem>
              )}
              {desktopQuitAvailable && (
                <DropdownMenuItem onClick={handleQuitApp} variant="destructive">
                  <Power />
                  <span>{t('nav.exitApp')}</span>
                </DropdownMenuItem>
              )}
            </DropdownMenuContent>
          </DropdownMenu>
        </div>
      </SidebarMenuItem>

      <Dialog open={showAccountDialog} onOpenChange={setShowAccountDialog}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t('nav.accountOverview')}</DialogTitle>
            <DialogDescription>{t('nav.sensitiveMasked')}</DialogDescription>
          </DialogHeader>
          <div className="space-y-3 py-4">
            <div className="grid gap-3 sm:grid-cols-2">
              <div className="rounded-lg border bg-muted/30 p-3">
                <div className="mb-2 flex items-center gap-2 text-xs text-muted-foreground">
                  <CircleUserRound className="h-3.5 w-3.5" />
                  <span>{t('nav.currentIdentity')}</span>
                </div>
                <p className="text-sm font-semibold">{displayUser.maskedIdentity}</p>
              </div>
              <div className="rounded-lg border bg-muted/30 p-3">
                <div className="mb-2 flex items-center gap-2 text-xs text-muted-foreground">
                  <BadgeCheck className="h-3.5 w-3.5" />
                  <span>{t('common.status')}</span>
                </div>
                <p className="text-sm font-semibold">{accountStatusLabel}</p>
              </div>
              <div className="rounded-lg border bg-muted/30 p-3">
                <div className="mb-2 flex items-center gap-2 text-xs text-muted-foreground">
                  <Building2 className="h-3.5 w-3.5" />
                  <span>{t('nav.tenant')}</span>
                </div>
                <p className="text-sm font-semibold">{displayUser.tenantLabel}</p>
                <p className="text-xs text-muted-foreground">{displayUser.tenantIDLabel}</p>
              </div>
              <div className="rounded-lg border bg-muted/30 p-3">
                <div className="mb-2 flex items-center gap-2 text-xs text-muted-foreground">
                  <IdCard className="h-3.5 w-3.5" />
                  <span>{t('nav.role')}</span>
                </div>
                <p className="text-sm font-semibold">{roleLabel}</p>
                <p className="text-xs text-muted-foreground">{displayUser.userLabel}</p>
              </div>
            </div>
            <div className="flex flex-wrap gap-2">
              <Button variant="outline" onClick={openSettingsPage}>
                <Settings2 className="mr-2 h-4 w-4" />
                {t('nav.openSettings')}
              </Button>
              {user?.role === 'admin' && (
                <Button variant="outline" onClick={openUsersPage}>
                  <CircleUserRound className="mr-2 h-4 w-4" />
                  {t('nav.manageUsers')}
                </Button>
              )}
            </div>
          </div>
        </DialogContent>
      </Dialog>

      {/* Change Password Dialog */}
      <Dialog open={showPasswordDialog} onOpenChange={setShowPasswordDialog}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t('users.changePassword')}</DialogTitle>
            <DialogDescription>{t('users.changePasswordDescription')}</DialogDescription>
          </DialogHeader>
          <div className="space-y-4 py-4">
            <div className="space-y-2">
              <label htmlFor="old-password" className="text-sm font-medium">
                {t('users.oldPassword')}
              </label>
              <Input
                id="old-password"
                type="password"
                value={passwordForm.oldPassword}
                onChange={(e) => setPasswordForm({ ...passwordForm, oldPassword: e.target.value })}
                placeholder={t('users.oldPassword')}
              />
            </div>
            <div className="space-y-2">
              <label htmlFor="new-password" className="text-sm font-medium">
                {t('users.newPassword')}
              </label>
              <Input
                id="new-password"
                type="password"
                value={passwordForm.newPassword}
                onChange={(e) => setPasswordForm({ ...passwordForm, newPassword: e.target.value })}
                placeholder={t('users.newPassword')}
              />
            </div>
            <div className="space-y-2">
              <label htmlFor="confirm-new-password" className="text-sm font-medium">
                {t('users.confirmNewPassword')}
              </label>
              <Input
                id="confirm-new-password"
                type="password"
                value={passwordForm.confirmPassword}
                onChange={(e) =>
                  setPasswordForm({ ...passwordForm, confirmPassword: e.target.value })
                }
                placeholder={t('users.confirmNewPassword')}
              />
            </div>
            {passwordError && <p className="text-destructive text-sm">{passwordError}</p>}
            {passwordSuccess && (
              <p className="text-green-600 dark:text-green-400 text-sm">{passwordSuccess}</p>
            )}
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setShowPasswordDialog(false)}>
              {t('common.cancel')}
            </Button>
            <Button
              onClick={handleChangePassword}
              disabled={
                !passwordForm.oldPassword ||
                !passwordForm.newPassword ||
                !passwordForm.confirmPassword ||
                changePassword.isPending
              }
            >
              {changePassword.isPending && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
              {t('common.confirm')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog open={showPasskeyDialog} onOpenChange={setShowPasskeyDialog}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t('users.passkeyManagement')}</DialogTitle>
            <DialogDescription>{t('users.passkeyManagementDescription')}</DialogDescription>
          </DialogHeader>
          <div className="space-y-4 py-4">
            <p className="text-xs text-muted-foreground">{t('users.passkeyFallbackHint')}</p>
            {passkeyCredentials.isLoading ? (
              <div className="flex items-center gap-2 text-sm text-muted-foreground">
                <Loader2 className="h-4 w-4 animate-spin" />
                <span>{t('common.loading')}</span>
              </div>
            ) : (passkeyCredentials.data?.length ?? 0) === 0 ? (
              <p className="text-sm text-muted-foreground">{t('users.passkeyListEmpty')}</p>
            ) : (
              <div className="space-y-2 max-h-80 overflow-y-auto pr-1">
                {(passkeyCredentials.data ?? []).map((credential) => (
                  <div key={credential.id} className="rounded-md border p-3 space-y-1">
                    <p className="text-sm font-medium">{credential.label}</p>
                    <p className="text-xs text-muted-foreground break-all">{credential.id}</p>
                    <p className="text-xs text-muted-foreground">
                      {[
                        credential.attachment
                          ? `${t('users.passkeyAttachment')}: ${credential.attachment}`
                          : null,
                        credential.transports?.length
                          ? `${t('users.passkeyTransport')}: ${credential.transports.join(', ')}`
                          : null,
                        `${t('users.passkeySignCount')}: ${credential.signCount}`,
                        credential.backupState
                          ? t('users.passkeyBackedUp')
                          : t('users.passkeyNotBackedUp'),
                      ]
                        .filter(Boolean)
                        .join(' · ')}
                    </p>
                    {credential.cloneWarning && (
                      <p className="text-xs text-amber-600">{t('users.passkeyCloneWarning')}</p>
                    )}
                    <Button
                      variant="destructive"
                      size="sm"
                      onClick={() => handleDeletePasskey(credential.id)}
                      disabled={deletePasskeyCredential.isPending}
                    >
                      {deletePasskeyCredential.isPending && deletingPasskeyID === credential.id ? (
                        <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                      ) : (
                        <Trash2 className="mr-2 h-4 w-4" />
                      )}
                      {t('users.passkeyDelete')}
                    </Button>
                  </div>
                ))}
              </div>
            )}
            {passkeyError && <p className="text-destructive text-sm">{passkeyError}</p>}
            {passkeySuccess && (
              <p className="text-green-600 dark:text-green-400 text-sm">{passkeySuccess}</p>
            )}
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setShowPasskeyDialog(false)}>
              {t('common.close')}
            </Button>
            <Button onClick={handleRegisterPasskey} disabled={registerPasskey.isPending}>
              {registerPasskey.isPending ? (
                <Loader2 className="mr-2 h-4 w-4 animate-spin" />
              ) : (
                <Plus className="mr-2 h-4 w-4" />
              )}
              {t('login.passkeyRegister')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </SidebarMenu>
  );
}

