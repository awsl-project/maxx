import type { ReasoningPolicy } from '@/lib/transport';
import { useTranslation } from 'react-i18next';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';

const EFFORTS = ['none', 'minimal', 'low', 'medium', 'high'] as const;
const UNSET_VALUE = '__unset__';

type ReasoningEffort = (typeof EFFORTS)[number];

interface ReasoningPolicySettingsProps {
  value?: ReasoningPolicy;
  onChange: (value: ReasoningPolicy | undefined) => void;
}

function normalizePolicy(policy?: ReasoningPolicy): ReasoningPolicy {
  return {
    defaultEffort: policy?.defaultEffort || undefined,
    maxEffort: policy?.maxEffort || undefined,
  };
}

function compactPolicy(policy: ReasoningPolicy): ReasoningPolicy | undefined {
  const next = normalizePolicy(policy);
  return next.defaultEffort || next.maxEffort ? next : undefined;
}

export function ReasoningPolicySettings({ value, onChange }: ReasoningPolicySettingsProps) {
  const { t } = useTranslation();
  const policy = normalizePolicy(value);

  const update = (patch: Partial<ReasoningPolicy>) => {
    onChange(compactPolicy({ ...policy, ...patch }));
  };

  const effortLabel = (effort: ReasoningEffort) => t(`provider.reasoningEffort.${effort}`);

  return (
    <div className="space-y-4 rounded-xl border border-border bg-card p-4">
      <div>
        <div className="text-sm font-medium text-foreground">{t('provider.reasoningPolicy')}</div>
        <p className="mt-1 text-xs text-muted-foreground">{t('provider.reasoningPolicyDesc')}</p>
      </div>

      <div className="grid grid-cols-1 gap-4 md:grid-cols-2">
        <div>
          <label className="mb-2 block text-xs font-medium text-muted-foreground">
            {t('provider.reasoningDefaultEffort')}
          </label>
          <Select
            value={policy.defaultEffort || UNSET_VALUE}
            onValueChange={(selected) =>
              update({
                defaultEffort: selected === UNSET_VALUE ? undefined : (selected as ReasoningEffort),
              })
            }
          >
            <SelectTrigger className="w-full">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value={UNSET_VALUE}>{t('provider.reasoningUnset')}</SelectItem>
              {EFFORTS.map((effort) => (
                <SelectItem key={effort} value={effort}>
                  {effortLabel(effort)}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
          <p className="mt-1 text-xs text-muted-foreground">
            {t('provider.reasoningDefaultEffortDesc')}
          </p>
        </div>

        <div>
          <label className="mb-2 block text-xs font-medium text-muted-foreground">
            {t('provider.reasoningMaxEffort')}
          </label>
          <Select
            value={policy.maxEffort || UNSET_VALUE}
            onValueChange={(selected) =>
              update({
                maxEffort: selected === UNSET_VALUE ? undefined : (selected as ReasoningEffort),
              })
            }
          >
            <SelectTrigger className="w-full">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value={UNSET_VALUE}>{t('provider.reasoningUnset')}</SelectItem>
              {EFFORTS.map((effort) => (
                <SelectItem key={effort} value={effort}>
                  {effortLabel(effort)}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
          <p className="mt-1 text-xs text-muted-foreground">
            {t('provider.reasoningMaxEffortDesc')}
          </p>
        </div>
      </div>
    </div>
  );
}
