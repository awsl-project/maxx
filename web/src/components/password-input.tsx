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
  const resolvedVisible = visible ?? internalVisible;

  return (
    <div className="group/password-input relative">
      <Input
        type={resolvedVisible ? 'text' : 'password'}
        className={cn('pr-10', className)}
        onFocus={onFocus}
        onBlur={onBlur}
        {...props}
      />
      <button
        type="button"
        aria-label={resolvedVisible ? t('common.hide') : t('common.show')}
        title={resolvedVisible ? t('common.hide') : t('common.show')}
        className="pointer-events-none absolute right-2 top-1/2 -translate-y-1/2 rounded-sm p-1 text-muted-foreground opacity-0 transition-[opacity,color] hover:text-foreground focus-visible:pointer-events-auto focus-visible:opacity-100 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring group-focus-within/password-input:pointer-events-auto group-focus-within/password-input:opacity-100"
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
    </div>
  );
}
