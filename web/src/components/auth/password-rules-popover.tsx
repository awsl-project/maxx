import { CheckIcon, CircleIcon } from 'lucide-react';
import { cn } from '@/lib/utils';
import type { ManagedPasswordRuleState } from '@/lib/managed-password';

interface PasswordRulesPopoverProps {
  open: boolean;
  ruleState: ManagedPasswordRuleState;
  title: string;
  progressLabel: string;
  minLengthLabel: string;
  numberLabel: string;
  letterLabel: string;
  punctuationLabel: string;
  className?: string;
}

function PasswordRequirementItem({ met, label }: { met: boolean; label: string }) {
  return (
    <div className="flex items-center gap-2 text-xs whitespace-nowrap">
      <span
        className={cn(
          'flex size-4 shrink-0 items-center justify-center rounded-full',
          met
            ? 'bg-emerald-500/15 text-emerald-600 dark:text-emerald-400'
            : 'text-muted-foreground',
        )}
      >
        {met ? <CheckIcon className="size-3" /> : <CircleIcon className="size-3" />}
      </span>
      <span className={met ? 'text-foreground' : 'text-muted-foreground'}>{label}</span>
    </div>
  );
}

export function PasswordRulesPopover({
  open,
  ruleState,
  title,
  progressLabel,
  minLengthLabel,
  numberLabel,
  letterLabel,
  punctuationLabel,
  className,
}: PasswordRulesPopoverProps) {
  if (!open) {
    return null;
  }

  return (
    <div
      className={cn(
        'bg-popover text-popover-foreground absolute left-0 top-[calc(100%+0.5rem)] z-30 w-max max-w-none rounded-xl border border-border/70 p-3 shadow-2xl',
        className,
      )}
    >
      <div className="mb-3 flex items-center justify-between gap-3 whitespace-nowrap">
        <p className="whitespace-nowrap text-xs font-medium text-foreground">{title}</p>
        <span className="text-muted-foreground whitespace-nowrap text-xs">{progressLabel}</span>
      </div>
      <div className="flex flex-col gap-2">
        <PasswordRequirementItem met={ruleState.minLength} label={minLengthLabel} />
        <PasswordRequirementItem met={ruleState.hasNumber} label={numberLabel} />
        <PasswordRequirementItem met={ruleState.hasLetter} label={letterLabel} />
        <PasswordRequirementItem met={ruleState.hasPunctuation} label={punctuationLabel} />
      </div>
    </div>
  );
}
