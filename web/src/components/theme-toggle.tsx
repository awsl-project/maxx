import * as React from 'react';
import { Check, Moon, Sun, Laptop, Sparkles } from 'lucide-react';
import { useTheme } from '@/components/theme-provider';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
import {
  getDefaultThemes,
  getLuxuryThemes,
  type Theme,
  type ThemeMetadata,
  getThemeMetadata,
} from '@/lib/theme';
import { cn } from '@/lib/utils';
import { Button } from './ui';

export function ThemeToggle() {
  const { theme, setTheme } = useTheme();
  const defaultThemes = getDefaultThemes();
  const luxuryThemes = getLuxuryThemes();
  const currentTheme = getThemeMetadata(theme);
  const [hoveredTheme, setHoveredTheme] = React.useState<ThemeMetadata | null>(null);
  const [focusedIndex, setFocusedIndex] = React.useState<number>(-1);
  const swatchRefs = React.useRef<(HTMLButtonElement | null)[]>([]);

  // Display hovered theme or current theme as fallback
  const displayTheme = hoveredTheme || currentTheme;

  // Keyboard navigation handler
  const handleKeyDown = React.useCallback(
    (e: React.KeyboardEvent<HTMLDivElement>) => {
      const allThemes = [...defaultThemes, ...luxuryThemes];
      const currentIndex = focusedIndex >= 0 ? focusedIndex : allThemes.findIndex((t) => t.id === theme);

      switch (e.key) {
        case 'ArrowRight':
        case 'ArrowDown': {
          e.preventDefault();
          const nextIndex = (currentIndex + 1) % allThemes.length;
          setFocusedIndex(nextIndex);
          swatchRefs.current[nextIndex]?.focus();
          break;
        }

        case 'ArrowLeft':
        case 'ArrowUp': {
          e.preventDefault();
          const prevIndex = currentIndex <= 0 ? allThemes.length - 1 : currentIndex - 1;
          setFocusedIndex(prevIndex);
          swatchRefs.current[prevIndex]?.focus();
          break;
        }

        case 'Enter':
        case ' ': {
          // Find the active element index - either from state or from DOM focus
          const activeIndex =
            focusedIndex >= 0
              ? focusedIndex
              : swatchRefs.current.findIndex((el) => el === document.activeElement);

          if (activeIndex < 0) return;

          e.preventDefault();
          setFocusedIndex(activeIndex);
          setTheme(allThemes[activeIndex].id);
          break;
        }

        case 'Escape':
          e.preventDefault();
          setFocusedIndex(-1);
          break;

        case 'Home':
          e.preventDefault();
          setFocusedIndex(0);
          swatchRefs.current[0]?.focus();
          break;

        case 'End': {
          e.preventDefault();
          const lastIndex = allThemes.length - 1;
          setFocusedIndex(lastIndex);
          swatchRefs.current[lastIndex]?.focus();
          break;
        }
      }
    },
    [focusedIndex, theme, defaultThemes, luxuryThemes, setTheme],
  );

  // Get icon based on current theme - memoized for performance
  const getThemeIcon = React.useMemo(() => {
    const iconClassName = 'transition-transform duration-200 hover:rotate-12 hover:scale-110';

    // System theme
    if (theme === 'system') {
      return <Laptop className={iconClassName} />;
    }

    // Luxury themes - use sparkles icon
    if (currentTheme.category === 'luxury') {
      return <Sparkles className={iconClassName} />;
    }

    // Default light/dark themes
    if (theme === 'light' || currentTheme.baseMode === 'light') {
      return <Sun className={iconClassName} />;
    }

    return <Moon className={iconClassName} />;
  }, [theme, currentTheme.category, currentTheme.baseMode]);

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
            {getThemeIcon}
            <span className="sr-only">Select theme - Current: {currentTheme.name}</span>
          </Button>
        )}
      />
      <DropdownMenuContent align="end" className="w-80 p-0 overflow-hidden" onKeyDown={handleKeyDown}>
        <div className="p-4 space-y-4">
          {/* Default Themes Section */}
          <div>
            <h3 className="mb-3 text-sm font-medium text-muted-foreground">Default Themes</h3>
            <div className="grid grid-cols-3 gap-2">
              {defaultThemes.map((themeOption, index) => (
                <ThemeSwatch
                  key={themeOption.id}
                  theme={themeOption.id}
                  name={themeOption.name}
                  accentColor={themeOption.accentColor}
                  isActive={theme === themeOption.id}
                  onClick={() => setTheme(themeOption.id)}
                  onHover={() => setHoveredTheme(themeOption)}
                  onLeave={() => setHoveredTheme(null)}
                  swatchRef={(el) => (swatchRefs.current[index] = el)}
                />
              ))}
            </div>
          </div>

          {/* Luxury Themes Section */}
          <div>
            <h3 className="mb-3 text-sm font-medium text-muted-foreground">Luxury Themes</h3>
            <div className="grid grid-cols-3 gap-2">
              {luxuryThemes.map((themeOption, index) => (
                <ThemeSwatch
                  key={themeOption.id}
                  theme={themeOption.id}
                  name={themeOption.name}
                  accentColor={themeOption.accentColor}
                  isActive={theme === themeOption.id}
                  onClick={() => setTheme(themeOption.id)}
                  onHover={() => setHoveredTheme(themeOption)}
                  onLeave={() => setHoveredTheme(null)}
                  swatchRef={(el) => (swatchRefs.current[defaultThemes.length + index] = el)}
                />
              ))}
            </div>
          </div>
        </div>

        {/* Preview Area - Fixed at bottom */}
        <div className="border-t border-border bg-muted/30 p-4 h-[180px] flex flex-col transition-all duration-200">
          <div className="space-y-2 animate-in fade-in-0 duration-200">
            <div className="flex items-center justify-between">
              <div className="font-semibold text-sm text-foreground">{displayTheme.name}</div>
              {hoveredTheme && hoveredTheme.id !== theme && (
                <span className="text-xs text-muted-foreground animate-in fade-in-0 duration-150">
                  Preview
                </span>
              )}
            </div>
            <div className="text-xs text-muted-foreground">{displayTheme.description}</div>
            {displayTheme.brandInspiration && (
              <div className="text-xs text-muted-foreground italic border-l-2 border-accent pl-2">
                Inspired by {displayTheme.brandInspiration}
              </div>
            )}
            {/* Color Preview Swatches */}
            <div className="grid grid-cols-3 gap-2 pt-2">
              <div className="space-y-1">
                <div
                  className="h-8 rounded-md border border-border shadow-sm"
                  style={{ background: displayTheme.accentColor }}
                />
                <div className="text-[10px] text-center text-muted-foreground">Accent</div>
              </div>
              <div className="space-y-1">
                <div
                  className="h-8 rounded-md border border-border shadow-sm"
                  style={{ background: displayTheme.primaryColor }}
                />
                <div className="text-[10px] text-center text-muted-foreground">Primary</div>
              </div>
              <div className="space-y-1">
                <div
                  className="h-8 rounded-md border border-border shadow-sm"
                  style={{ background: displayTheme.secondaryColor }}
                />
                <div className="text-[10px] text-center text-muted-foreground">Secondary</div>
              </div>
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
  accentColor: string;
  isActive: boolean;
  onClick: () => void;
  onHover: () => void;
  onLeave: () => void;
  swatchRef?: (el: HTMLButtonElement | null) => void;
}

function ThemeSwatch({
  theme,
  name,
  accentColor,
  isActive,
  onClick,
  onHover,
  onLeave,
  swatchRef,
}: ThemeSwatchProps) {
  return (
    <button
      ref={swatchRef}
      type="button"
      onClick={onClick}
      onMouseEnter={onHover}
      onMouseLeave={onLeave}
      className={cn(
        'group relative flex flex-col items-center gap-2 rounded-lg p-3',
        'transition-all duration-200',
        'hover:bg-accent/50 hover:scale-[1.02]',
        'active:scale-[0.98]',
        'focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring/50 focus-visible:ring-offset-2',
        isActive && 'bg-accent/30',
      )}
      aria-label={`Select ${name} theme${isActive ? ' (currently selected)' : ''}`}
      tabIndex={0}
    >
      {/* Color Swatch */}
      <div className="relative">
        <div
          className={cn(
            'h-10 w-10 rounded-full transition-all duration-300',
            'hover:scale-105 active:scale-95',
            isActive ? 'scale-110 ring-2 ring-primary ring-offset-2 ring-offset-background' : '',
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
          <div className="absolute inset-0 flex items-center justify-center animate-in zoom-in-95 fade-in-0 duration-200">
            <div className="rounded-full bg-background/90 backdrop-blur-sm p-0.5 shadow-sm">
              <Check className="h-4 w-4 text-primary animate-in zoom-in-50 duration-300" />
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
  );
}
