import { useState } from 'react';
import { Eye, EyeOff } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { Input } from '@/components/ui/input';
import { cn } from '@/lib/utils';

export function PasswordInput({
  className,
  onBlur,
  onFocus,
  onVisibleChange,
  visible,
  ...props
}: Omit<React.ComponentProps<typeof Input>, 'type'> & {
  onVisibleChange?: (visible: boolean) => void;
  visible?: boolean;
}) {
  const { t } = useTranslation();
  const [internalVisible, setInternalVisible] = useState(false);
  const [focused, setFocused] = useState(false);
  const resolvedVisible = visible ?? internalVisible;

  return (
    <div className="relative">
      <Input
        type={resolvedVisible ? 'text' : 'password'}
        className={cn('pr-10', className)}
        onFocus={(event) => {
          setFocused(true);
          onFocus?.(event);
        }}
        onBlur={(event) => {
          setFocused(false);
          onBlur?.(event);
        }}
        {...props}
      />
      {focused && (
        <button
          type="button"
          tabIndex={-1}
          aria-label={resolvedVisible ? t('common.hide') : t('common.show')}
          title={resolvedVisible ? t('common.hide') : t('common.show')}
          className="absolute right-2 top-1/2 -translate-y-1/2 text-muted-foreground transition-colors hover:text-foreground"
          onMouseDown={(event) => event.preventDefault()}
          onClick={() => {
            const nextVisible = !resolvedVisible;
            if (visible === undefined) {
              setInternalVisible(nextVisible);
            }
            onVisibleChange?.(nextVisible);
          }}
        >
          {resolvedVisible ? <EyeOff className="h-4 w-4" /> : <Eye className="h-4 w-4" />}
        </button>
      )}
    </div>
  );
}
