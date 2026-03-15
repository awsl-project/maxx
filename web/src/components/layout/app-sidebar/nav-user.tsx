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
import { PasswordRulesPopover } from '@/components/auth/password-rules-popover';
import { FieldError } from '@/components/field-error';
import { PasswordInput } from '@/components/password-input';
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
  Label,
} from '@/components/ui';
import { Button } from '@/components/ui/button';
import {
  getManagedPasswordError,
  getManagedPasswordRuleState,
  isPasswordPolicyViolationResponse,
} from '@/lib/managed-password';
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
import { useDialog } from '@/contexts/dialog-context';

type PasswordField = 'oldPassword' | 'newPassword' | 'confirmPassword';

export function NavUser() {
  const { isMobile, state } = useSidebar();
  const { t, i18n } = useTranslation();
  const { alert, confirm } = useDialog();
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
  const [passwordFieldErrors, setPasswordFieldErrors] = useState<
    Partial<Record<PasswordField, string>>
  >({});
  const [showPasswordRules, setShowPasswordRules] = useState(false);
  const [newPasswordsVisible, setNewPasswordsVisible] = useState(false);
  const passwordTimeoutRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const passwordRequestIdRef = useRef(0);
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
  const passwordRuleState = getManagedPasswordRuleState(passwordForm.newPassword);
  const passwordInvalidMessage = t('login.passwordFormatInvalid');
  const passwordFormatError = getManagedPasswordError(
    passwordForm.newPassword,
    passwordInvalidMessage,
  );
  const passwordFieldError =
    passwordFieldErrors.newPassword === passwordFormatError
      ? undefined
      : passwordFieldErrors.newPassword;
  const isChangePasswordDisabled =
    !passwordForm.oldPassword.trim() ||
    !passwordForm.newPassword.trim() ||
    !passwordForm.confirmPassword.trim() ||
    !!passwordFormatError ||
    passwordForm.newPassword !== passwordForm.confirmPassword ||
    changePassword.isPending;

  const resetPasswordDialogState = () => {
    passwordRequestIdRef.current += 1;
    setPasswordForm({ oldPassword: '', newPassword: '', confirmPassword: '' });
    setPasswordFieldErrors({});
    setPasswordError('');
    setPasswordSuccess('');
    setShowPasswordRules(false);
    setNewPasswordsVisible(false);
    if (passwordTimeoutRef.current) {
      clearTimeout(passwordTimeoutRef.current);
      passwordTimeoutRef.current = null;
    }
  };

  const handleToggleLanguage = () => {
    i18n.changeLanguage(currentLanguage === 'zh' ? 'en' : 'zh');
  };

  const handleRestartServer = async () => {
    const confirmed = await confirm({
      title: t('common.confirm'),
      description: t('nav.restartServerConfirm'),
      confirmText: t('nav.restartServer'),
    });
    if (!confirmed) return;

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
      await alert({
        title: t('nav.notifications'),
        description: t('nav.restartServerFailed'),
      });
    }
  };

  const handleChangePassword = async () => {
    setPasswordFieldErrors({});
    setPasswordError('');
    setPasswordSuccess('');

    const nextErrors: Partial<Record<PasswordField, string>> = {};
    if (!passwordForm.oldPassword.trim()) {
      nextErrors.oldPassword = t('users.oldPassword');
    }
    if (!passwordForm.newPassword.trim()) {
      nextErrors.newPassword = t('login.passwordRequired');
    }
    if (passwordFormatError) {
      setShowPasswordRules(true);
    }
    if (!passwordForm.confirmPassword.trim()) {
      nextErrors.confirmPassword = t('login.confirmPasswordRequired');
    }
    if (
      passwordForm.confirmPassword.trim() &&
      passwordForm.newPassword !== passwordForm.confirmPassword
    ) {
      nextErrors.confirmPassword = t('users.passwordMismatch');
    }

    if (Object.keys(nextErrors).length > 0) {
      setPasswordFieldErrors(nextErrors);
      if (nextErrors.newPassword) {
        setShowPasswordRules(true);
      }
      return;
    }

    const requestId = passwordRequestIdRef.current + 1;
    passwordRequestIdRef.current = requestId;

    try {
      await changePassword.mutateAsync({
        oldPassword: passwordForm.oldPassword,
        newPassword: passwordForm.newPassword,
      });
      if (requestId !== passwordRequestIdRef.current) {
        return;
      }
      setPasswordSuccess(t('users.changePasswordSuccess'));
      setPasswordForm({ oldPassword: '', newPassword: '', confirmPassword: '' });
      setPasswordFieldErrors({});
      setShowPasswordRules(false);
      if (passwordTimeoutRef.current) {
        clearTimeout(passwordTimeoutRef.current);
      }
      passwordTimeoutRef.current = setTimeout(() => {
        if (requestId !== passwordRequestIdRef.current) {
          return;
        }
        setShowPasswordDialog(false);
        resetPasswordDialogState();
      }, 1500);
    } catch (err: unknown) {
      if (requestId !== passwordRequestIdRef.current) {
        return;
      }
      const axiosError = err as { response?: { data?: { error?: string; code?: string } } };
      const errorData = axiosError?.response?.data;
      const errorMsg = errorData?.error;
      if (isPasswordPolicyViolationResponse(errorData)) {
        setShowPasswordRules(true);
        return;
      }
      setPasswordError(errorMsg || t('users.changePasswordFailed'));
    }
  };

  const handleDeletePasskey = async (credentialID: string) => {
    const confirmed = await confirm({
      title: t('common.confirm'),
      description: t('users.passkeyDeleteConfirm'),
      confirmText: t('common.delete'),
      confirmVariant: 'destructive',
    });
    if (!confirmed) return;

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
                      resetPasswordDialogState();
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
      <Dialog
        open={showPasswordDialog}
        onOpenChange={(open) => {
          setShowPasswordDialog(open);
          if (!open) {
            resetPasswordDialogState();
          }
        }}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t('users.changePassword')}</DialogTitle>
            <DialogDescription>{t('users.changePasswordDescription')}</DialogDescription>
          </DialogHeader>
          <div className="space-y-4 py-4">
            <div className="space-y-2">
              <Label htmlFor="old-password">
                {t('users.oldPassword')}
              </Label>
              <PasswordInput
                id="old-password"
                value={passwordForm.oldPassword}
                aria-invalid={passwordFieldErrors.oldPassword ? 'true' : undefined}
                onChange={(e) => {
                  setPasswordForm({ ...passwordForm, oldPassword: e.target.value });
                  setPasswordFieldErrors((current) => ({ ...current, oldPassword: undefined }));
                  setPasswordError('');
                }}
                placeholder={t('users.oldPassword')}
              />
              <FieldError message={passwordFieldErrors.oldPassword} />
            </div>
            <div className="space-y-2">
              <Label htmlFor="new-password">
                {t('users.newPassword')}
              </Label>
              <div className="relative">
                <PasswordInput
                  id="new-password"
                  value={passwordForm.newPassword}
                  aria-invalid={passwordFieldErrors.newPassword ? 'true' : undefined}
                  onFocus={() => setShowPasswordRules(true)}
                  onBlur={() => setShowPasswordRules(false)}
                  onChange={(e) => {
                    const nextPassword = e.target.value;
                    const nextPasswordError = getManagedPasswordError(
                      nextPassword,
                      passwordInvalidMessage,
                    );
                    setPasswordForm({ ...passwordForm, newPassword: nextPassword });
                    setShowPasswordRules(true);
                    setPasswordFieldErrors((current) => ({
                      ...current,
                      newPassword: nextPasswordError,
                      confirmPassword:
                        passwordForm.confirmPassword &&
                        nextPassword !== passwordForm.confirmPassword
                          ? t('users.passwordMismatch')
                          : undefined,
                    }));
                    setPasswordError('');
                  }}
                  placeholder={t('users.newPassword')}
                  visible={newPasswordsVisible}
                  onVisibleChange={setNewPasswordsVisible}
                />
                <PasswordRulesPopover
                  open={showPasswordRules}
                  ruleState={passwordRuleState}
                  title={t('login.passwordChecklistTitle')}
                  progressLabel={t('login.passwordCategoryProgress', {
                    count: passwordRuleState.categoryCount,
                  })}
                  minLengthLabel={t('login.passwordRuleMinLength')}
                  numberLabel={t('login.passwordRuleNumber')}
                  letterLabel={t('login.passwordRuleLetter')}
                  punctuationLabel={t('login.passwordRulePunctuation')}
                />
              </div>
              <FieldError message={passwordFieldError} />
            </div>
            <div className="space-y-2">
              <Label htmlFor="confirm-new-password">
                {t('users.confirmNewPassword')}
              </Label>
              <PasswordInput
                id="confirm-new-password"
                value={passwordForm.confirmPassword}
                aria-invalid={passwordFieldErrors.confirmPassword ? 'true' : undefined}
                onChange={(e) => {
                  const nextConfirmPassword = e.target.value;
                setPasswordForm({ ...passwordForm, confirmPassword: nextConfirmPassword });
                setPasswordFieldErrors((current) => ({
                  ...current,
                  confirmPassword:
                      nextConfirmPassword.trim() &&
                      passwordForm.newPassword !== nextConfirmPassword
                        ? t('users.passwordMismatch')
                        : undefined,
                }));
                  setPasswordError('');
                }}
                placeholder={t('users.confirmNewPassword')}
                visible={newPasswordsVisible}
                onVisibleChange={setNewPasswordsVisible}
              />
              <FieldError message={passwordFieldErrors.confirmPassword} />
            </div>
            {passwordError && <p className="text-destructive text-sm">{passwordError}</p>}
            {passwordSuccess && (
              <p className="text-green-600 dark:text-green-400 text-sm">{passwordSuccess}</p>
            )}
          </div>
          <DialogFooter>
            <Button
              variant="outline"
              onClick={() => {
                setShowPasswordDialog(false);
                resetPasswordDialogState();
              }}
            >
              {t('common.cancel')}
            </Button>
            <Button onClick={handleChangePassword} disabled={isChangePasswordDisabled}>
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
