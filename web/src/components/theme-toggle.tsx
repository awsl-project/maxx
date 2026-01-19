import { Check, Moon, Sun, Laptop } from 'lucide-react';
import { useTheme } from '@/components/theme-provider';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
import { Tooltip, TooltipTrigger, TooltipContent } from '@/components/ui/tooltip';
import { getDefaultThemes, getLuxuryThemes, type Theme, getThemeMetadata } from '@/lib/theme';
import { cn } from '@/lib/utils';
import { Button } from './ui';

export function ThemeToggle() {
  const { theme, setTheme } = useTheme();
  const defaultThemes = getDefaultThemes();
  const luxuryThemes = getLuxuryThemes();
  const currentTheme = getThemeMetadata(theme);

  // Get icon based on current theme
  const getThemeIcon = () => {
    if (theme === 'system') return <Laptop className="transition-transform hover:rotate-12" />;
    if (theme === 'light' || currentTheme.baseMode === 'light')
      return <Sun className="transition-transform hover:rotate-12" />;
    return <Moon className="transition-transform hover:rotate-12" />;
  };

  return (
    <DropdownMenu>
      <DropdownMenuTrigger
        render={(props) => (
          <Button
            {...props}
            title={`Current theme: ${currentTheme.name}`}
            variant="ghost"
            size="icon-sm"
          >
            {getThemeIcon()}
            <span className="sr-only">Select theme - Current: {currentTheme.name}</span>
          </Button>
        )}
      />
      <DropdownMenuContent align="end" className="w-80 p-4 overflow-visible">
        <div className="space-y-4">
          {/* Default Themes Section */}
          <div>
            <h3 className="mb-3 text-sm font-medium text-muted-foreground">Default Themes</h3>
            <div className="grid grid-cols-3 gap-2">
              {defaultThemes.map((themeOption) => (
                <ThemeSwatch
                  key={themeOption.id}
                  theme={themeOption.id}
                  name={themeOption.name}
                  description={themeOption.description}
                  accentColor={themeOption.accentColor}
                  primaryColor={themeOption.primaryColor}
                  secondaryColor={themeOption.secondaryColor}
                  isActive={theme === themeOption.id}
                  onClick={() => setTheme(themeOption.id)}
                />
              ))}
            </div>
          </div>

          {/* Luxury Themes Section */}
          <div>
            <h3 className="mb-3 text-sm font-medium text-muted-foreground">Luxury Themes</h3>
            <div className="grid grid-cols-3 gap-2">
              {luxuryThemes.map((themeOption) => (
                <ThemeSwatch
                  key={themeOption.id}
                  theme={themeOption.id}
                  name={themeOption.name}
                  description={themeOption.description}
                  brandInspiration={themeOption.brandInspiration}
                  accentColor={themeOption.accentColor}
                  primaryColor={themeOption.primaryColor}
                  secondaryColor={themeOption.secondaryColor}
                  isActive={theme === themeOption.id}
                  onClick={() => setTheme(themeOption.id)}
                />
              ))}
            </div>
          </div>
        </div>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}

interface ThemeSwatchProps {
  theme: Theme;
  name: string;
  description: string;
  brandInspiration?: string;
  accentColor: string;
  primaryColor: string;
  secondaryColor: string;
  isActive: boolean;
  onClick: () => void;
}

function ThemeSwatch({
  theme,
  name,
  description,
  brandInspiration,
  accentColor,
  primaryColor,
  secondaryColor,
  isActive,
  onClick,
}: ThemeSwatchProps) {
  const tooltipText = brandInspiration
    ? `${description} • Inspired by ${brandInspiration}`
    : description;

  return (
    <Tooltip>
      <TooltipTrigger
        render={(props) => (
          <button
            {...props}
            type="button"
            onClick={onClick}
            className={cn(
              'group relative flex flex-col items-center gap-2 rounded-lg p-3 transition-colors',
              'hover:bg-accent/50',
              isActive && 'bg-accent/30',
            )}
            title={tooltipText}
            aria-label={`Select ${name} theme`}
          >
            {/* Color Swatch */}
            <div className="relative">
              <div
                className={cn(
                  'h-10 w-10 rounded-full border-2 transition-all',
                  isActive ? 'border-primary scale-110' : 'border-border',
                )}
                style={{
                  background:
                    theme === 'system'
                      ? 'linear-gradient(135deg, oklch(0.3261 0 0) 50%, oklch(0.9848 0 0) 50%)'
                      : accentColor,
                }}
              />
              {/* Checkmark for active theme */}
              {isActive && (
                <div className="absolute inset-0 flex items-center justify-center">
                  <div className="rounded-full bg-background p-0.5">
                    <Check className="h-4 w-4 text-primary" />
                  </div>
                </div>
              )}
            </div>

            {/* Theme Name */}
            <span
              className={cn(
                'text-xs font-medium text-center leading-tight',
                isActive ? 'text-foreground' : 'text-muted-foreground',
              )}
            >
              {name}
            </span>
          </button>
        )}
      />
      <TooltipContent className="w-56 p-4 bg-card border border-border">
        <div className="space-y-2">
          <div className="font-semibold text-sm text-foreground">{name}</div>
          <div className="text-xs text-muted-foreground">{description}</div>
          {brandInspiration && (
            <div className="text-xs text-muted-foreground italic border-l-2 border-accent pl-2">
              Inspired by {brandInspiration}
            </div>
          )}
          {/* Color Preview Swatches */}
          <div className="space-y-1.5 pt-2 border-t border-border">
            <div className="text-xs font-medium text-muted-foreground">Color Preview</div>
            <div className="grid grid-cols-3 gap-2">
              <div className="space-y-1">
                <div
                  className="h-10 rounded-md border-2 border-border shadow-sm"
                  style={{ background: accentColor }}
                />
                <div className="text-[10px] text-center text-muted-foreground">Accent</div>
              </div>
              <div className="space-y-1">
                <div
                  className="h-10 rounded-md border-2 border-border shadow-sm"
                  style={{ background: primaryColor }}
                />
                <div className="text-[10px] text-center text-muted-foreground">Primary</div>
              </div>
              <div className="space-y-1">
                <div
                  className="h-10 rounded-md border-2 border-border shadow-sm"
                  style={{ background: secondaryColor }}
                />
                <div className="text-[10px] text-center text-muted-foreground">Secondary</div>
              </div>
            </div>
          </div>
        </div>
      </TooltipContent>
    </Tooltip>
  );
}
