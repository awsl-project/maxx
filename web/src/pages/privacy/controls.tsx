import type { ReactNode, SelectHTMLAttributes } from 'react';
import { Switch } from '@/components/ui';

export function Toggle({
  label,
  checked,
  onChange,
  description,
}: {
  label: string;
  checked: boolean;
  onChange: (value: boolean) => void;
  description?: string;
}) {
  return (
    <label className="flex items-center justify-between gap-4 py-2">
      <span className="space-y-1">
        <span className="block text-sm font-medium">{label}</span>
        {description && <span className="block text-xs text-text-secondary">{description}</span>}
      </span>
      <Switch aria-label={label} checked={checked} onCheckedChange={onChange} />
    </label>
  );
}
export function Choice(props: SelectHTMLAttributes<HTMLSelectElement>) {
  return (
    <select
      {...props}
      className={`min-w-0 rounded-md border border-border bg-bg-primary px-3 py-2 text-sm ${props.className ?? ''}`}
    />
  );
}
export function Field({
  label,
  children,
  description,
}: {
  label: string;
  children: ReactNode;
  description?: string;
}) {
  return (
    <label className="flex min-w-0 flex-col gap-2 text-sm">
      <span className="font-medium">{label}</span>
      {children}
      {description && <span className="text-xs text-text-secondary">{description}</span>}
    </label>
  );
}
