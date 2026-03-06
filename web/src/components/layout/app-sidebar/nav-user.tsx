'use client';

import { useState, useRef, useEffect } from 'react';
import {
  Moon,
  Sun,
  Laptop,
  Sparkles,
  Gem,
  Github,
  ChevronsUp,
  RefreshCw,
  LogOut,
  KeyRound,
  Loader2,
  Plus,
  ShieldAlert,
  Trash2,
} from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { useTheme } from '@/components/theme-provider';
import { useTransport } from '@/lib/transport/context';
import { useAuth } from '@/lib/auth-context';
import {
  useChangeMyPassword,
  useDeletePasskeyCredential,
  usePasskeyCredentials,
  useRegisterPasskey,
} from '@/hooks/queries';
import type { Theme } from '@/lib/theme';
import { Avatar, AvatarFallback, AvatarImage } from '@/components/ui/avatar';
import {
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
  const { isMobile, state } = useSidebar();
  const { t, i18n } = useTranslation();
  const { transport } = useTransport();
  const { theme, setTheme } = useTheme();
  const { user, authEnabled, logout } = useAuth();
  const changePassword = useChangeMyPassword();
  const isCollapsed = !isMobile && state === 'collapsed';

  const [showPasskeyDialog, setShowPasskeyDialog] = useState(false);
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
  const passkeyCredentials = usePasskeyCredentials(showPasskeyDialog && authEnabled);
  const deletePasskeyCredential = useDeletePasskeyCredential();
  const registerPasskey = useRegisterPasskey();

  useEffect(() => {
    return () => {
      if (passwordTimeoutRef.current) {
        clearTimeout(passwordTimeoutRef.current);
      }
    };
  }, []);
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

  const username = user?.username?.trim() || '';
  const hasUsername = username.length > 0;
  const displayUser = {
    name: username,
    avatar: '/logo.png',
  };
  const displayUserFallback = (displayUser.name || 'U').slice(0, 2).toUpperCase();
  const menuDisplayName = displayUser.name || 'Maxx';
  const menuDisplayFallback = menuDisplayName.slice(0, 2).toUpperCase();
  const accountTitle = hasUsername ? displayUser.name : undefined;

  return (
    <SidebarMenu>
      <SidebarMenuItem>
        <div
          className={cn(
            'flex items-center gap-2 rounded-xl border border-sidebar-border/70 bg-sidebar/70 p-1.5 backdrop-blur-sm',
            isCollapsed ? 'flex-col' : 'justify-between',
          )}
        >
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
              isCollapsed ? 'h-8 w-8 justify-center' : 'h-8 px-1 gap-1',
            )}
          >
            {isCollapsed ? (
              <span className="text-[11px] font-semibold uppercase">
                {currentLanguage === 'zh' ? '中' : 'EN'}
              </span>
            ) : (
              <>
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
              </>
            )}
          </button>

          {hasUsername &&
            (isCollapsed ? (
              <Tooltip>
                <TooltipTrigger
                  render={(props) => (
                    <button
                      {...props}
                      type="button"
                      className={cn(
                        'inline-flex h-8 w-8 items-center justify-center rounded-lg border border-sidebar-border/70 bg-sidebar-accent/40 text-sidebar-foreground transition-colors hover:bg-sidebar-accent',
                        props.className,
                      )}
                    >
                      <Avatar className="h-6 w-6 rounded-lg">
                        <AvatarImage src={displayUser.avatar} alt={displayUser.name} />
                        <AvatarFallback className="rounded-lg text-[10px]">
                          {displayUserFallback}
                        </AvatarFallback>
                      </Avatar>
                    </button>
                  )}
                />
                <TooltipContent side={isMobile ? 'top' : 'right'} align="center">
                  <span className="text-xs font-medium">{displayUser.name}</span>
                </TooltipContent>
              </Tooltip>
            ) : (
              <div
                className="flex h-8 min-w-0 flex-1 items-center gap-2 rounded-lg border border-sidebar-border/70 bg-sidebar-accent/20 px-2"
                title={accountTitle}
              >
                <Avatar className="h-6 w-6 rounded-lg">
                  <AvatarImage src={displayUser.avatar} alt={displayUser.name} />
                  <AvatarFallback className="rounded-lg text-[10px]">
                    {displayUserFallback}
                  </AvatarFallback>
                </Avatar>
                <div className="min-w-0">
                  <span className="block truncate text-xs font-medium">{displayUser.name}</span>
                </div>
              </div>
            ))}

          <DropdownMenu>
            <DropdownMenuTrigger
              render={(props) => (
                <button
                  {...props}
                  type="button"
                  title="Menu"
                  className={cn(
                    'inline-flex h-8 w-8 items-center justify-center rounded-lg text-sidebar-foreground/80 transition-colors hover:bg-sidebar-accent hover:text-sidebar-accent-foreground',
                    props.className,
                  )}
                >
                  <ChevronsUp className="h-4 w-4" />
                </button>
              )}
            />
            <DropdownMenuContent
              className="!w-32 rounded-lg max-w-xs !min-w-0"
              style={{ width: '8rem' }}
              side={isMobile ? 'bottom' : 'right'}
              align="end"
              sideOffset={4}
            >
              <DropdownMenuGroup>
                <DropdownMenuLabel>
                  <div className="flex items-center gap-2 w-full">
                    <Avatar className="h-8 w-8 rounded-lg">
                      <AvatarImage src={displayUser.avatar} alt={menuDisplayName} />
                      <AvatarFallback className="rounded-lg">{menuDisplayFallback}</AvatarFallback>
                    </Avatar>
                    <div className="grid flex-1 text-left text-sm leading-tight">
                      <span className="truncate font-medium">{menuDisplayName}</span>
                      {user && (
                        <span className="truncate text-xs text-muted-foreground">
                          {user.role === 'admin' ? t('users.roleAdmin') : t('users.roleMember')}
                          {user.tenantName && ` · ${user.tenantName}`}
                        </span>
                      )}
                    </div>
                  </div>
                </DropdownMenuLabel>
                <DropdownMenuSeparator />
              </DropdownMenuGroup>
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
              <>
                <DropdownMenuSeparator />
                <DropdownMenuItem onClick={handleRestartServer}>
                  <RefreshCw />
                  <span>{t('nav.restartServer')}</span>
                </DropdownMenuItem>
              </>
              {authEnabled && (
                <>
                  <DropdownMenuSeparator />
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
                </>
              )}
              {authEnabled && (
                <>
                  <DropdownMenuSeparator />
                  <DropdownMenuItem
                    onClick={() => {
                      setPasswordForm({ oldPassword: '', newPassword: '', confirmPassword: '' });
                      setPasswordError('');
                      setPasswordSuccess('');
                      setShowPasswordDialog(true);
                    }}
                  >
                    <KeyRound />
                    <span>{t('nav.changePassword')}</span>
                  </DropdownMenuItem>
                </>
              )}
              {authEnabled && (
                <>
                  <DropdownMenuSeparator />
                  <DropdownMenuItem onClick={logout}>
                    <LogOut />
                    <span>{t('nav.logout')}</span>
                  </DropdownMenuItem>
                </>
              )}
            </DropdownMenuContent>
          </DropdownMenu>
        </div>
      </SidebarMenuItem>

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
