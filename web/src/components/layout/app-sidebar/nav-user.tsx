'use client';

import { Moon, Sun, Laptop, Sparkles, Gem, Github, ChevronsUp, RefreshCw, LogOut } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { useTheme } from '@/components/theme-provider';
import { useTransport } from '@/lib/transport/context';
import { useAuth } from '@/lib/auth-context';
import type { Theme } from '@/lib/theme';
import { Avatar, AvatarFallback, AvatarImage } from '@/components/ui/avatar';
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
import {
  SidebarMenu,
  SidebarMenuItem,
  useSidebar,
} from '@/components/ui/sidebar';

export function NavUser() {
  const { isMobile, state } = useSidebar();
  const { t, i18n } = useTranslation();
  const { transport } = useTransport();
  const { theme, setTheme } = useTheme();
  const { user, authEnabled, multiTenancyEnabled, logout } = useAuth();
  const isCollapsed = !isMobile && state === 'collapsed';
  const currentLanguage = (i18n.resolvedLanguage || i18n.language || 'en').toLowerCase().startsWith('zh')
    ? 'zh'
    : 'en';
  const currentLanguageLabel =
    currentLanguage === 'zh' ? t('settings.languages.zh') : t('settings.languages.en');
  const desktopRestartAvailable =
    typeof window !== 'undefined' &&
    !!(window as unknown as { go?: { desktop?: { LauncherApp?: { RestartServer?: () => unknown } } } })
      .go?.desktop?.LauncherApp?.RestartServer;

  const handleToggleLanguage = () => {
    i18n.changeLanguage(currentLanguage === 'zh' ? 'en' : 'zh');
  };

  const handleRestartServer = async () => {
    if (!window.confirm(t('nav.restartServerConfirm'))) return;
    try {
      if (desktopRestartAvailable) {
        const launcher = (window as unknown as {
          go?: { desktop?: { LauncherApp?: { RestartServer?: () => Promise<void> } } };
        }).go?.desktop?.LauncherApp;
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

  const displayUser = {
    name: user?.username || 'Maxx',
    avatar: '/logo.png',
  };

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
                      <AvatarImage src={displayUser.avatar} alt={displayUser.name} />
                      <AvatarFallback className="rounded-lg">
                        {displayUser.name.substring(0, 2).toUpperCase()}
                      </AvatarFallback>
                    </Avatar>
                    <div className="grid flex-1 text-left text-sm leading-tight">
                      <span className="truncate font-medium">{displayUser.name}</span>
                      {multiTenancyEnabled && user && (
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
                      <DropdownMenuRadioGroup value={theme} onValueChange={(v) => setTheme(v as Theme)}>
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
    </SidebarMenu>
  );
}


