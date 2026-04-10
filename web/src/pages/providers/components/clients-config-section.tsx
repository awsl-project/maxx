import { useState, useEffect } from 'react';
import { Switch } from '@/components/ui';
import { Input } from '@/components/ui/input';
import { Textarea } from '@/components/ui/textarea';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import { ClientIcon } from '@/components/icons/client-icons';
import type { ClientType } from '@/lib/transport';
import type { ClientConfig } from '../types';
import { useTranslation } from 'react-i18next';

export type DisguiseTypeValue = 'none' | 'claude-code' | 'bedrock';

interface DisguiseProp {
  type: DisguiseTypeValue;
  // Sub-options only meaningful when type === 'claude-code'
  claudeCodeMode: 'auto' | 'always' | 'never';
  claudeCodeStrictMode: boolean;
  claudeCodeSensitiveWords: string;
}

interface ClientsConfigSectionProps {
  clients: ClientConfig[];
  onUpdateClient: (clientId: ClientType, updates: Partial<ClientConfig>) => void;
  disguise?: DisguiseProp;
  onUpdateDisguise?: (updates: Partial<DisguiseProp>) => void;
}

// Separate component for multiplier input to manage local state
function MultiplierInput({
  value,
  onChange,
  disabled,
}: {
  value: number;
  onChange: (value: number) => void;
  disabled: boolean;
}) {
  const [localValue, setLocalValue] = useState(() => (value / 10000).toFixed(2));

  // Sync with external value when it changes (e.g., from parent reset)
  useEffect(() => {
    setLocalValue((value / 10000).toFixed(2));
  }, [value]);

  const handleChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    setLocalValue(e.target.value);
  };

  const handleBlur = () => {
    const parsed = parseFloat(localValue);
    if (!isNaN(parsed) && parsed >= 0) {
      onChange(Math.round(parsed * 10000));
      setLocalValue(parsed.toFixed(2));
    } else {
      // Reset to current value if invalid
      setLocalValue((value / 10000).toFixed(2));
    }
  };

  return (
    <Input
      type="number"
      step="0.01"
      min="0"
      value={localValue}
      onChange={handleChange}
      onBlur={handleBlur}
      disabled={disabled}
      className="text-sm w-24 bg-card h-9 font-mono"
    />
  );
}

export function ClientsConfigSection({
  clients,
  onUpdateClient,
  disguise,
  onUpdateDisguise,
}: ClientsConfigSectionProps) {
  const { t } = useTranslation();
  return (
    <div>
      <div className="rounded-xl border border-border overflow-hidden bg-card">
        {clients.map((client, index) => (
          <div
            key={client.id}
            className={`px-4 py-4 transition-colors duration-200 ${
              client.enabled ? 'bg-card' : 'bg-muted/30'
            } ${index > 0 ? 'border-t border-border' : ''}`}
          >
            <div className="flex items-center justify-between">
              <div className="flex items-center gap-3">
                <ClientIcon type={client.id} size={32} />
                <span
                  className={`text-base font-semibold ${client.enabled ? 'text-foreground' : 'text-muted-foreground'}`}
                >
                  {client.name}
                </span>
              </div>
              <div onClick={(e) => e.stopPropagation()}>
                <Switch
                  checked={client.enabled}
                  onCheckedChange={(checked) => onUpdateClient(client.id, { enabled: checked })}
                />
              </div>
            </div>

            {/* Expandable/Visible Content */}
            <div
              className={`pt-4 transition-all duration-200 ${
                client.enabled ? 'opacity-100' : 'opacity-50 grayscale pointer-events-none'
              }`}
            >
              <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
                <div>
                  <label className="text-xs font-medium text-muted-foreground block mb-1.5 uppercase tracking-wide">
                    {t('provider.endpointOverride')}
                  </label>
                  <Input
                    type="text"
                    value={client.urlOverride}
                    onChange={(e) => onUpdateClient(client.id, { urlOverride: e.target.value })}
                    placeholder={t('common.default')}
                    disabled={!client.enabled}
                    className="text-sm w-full bg-card h-9"
                  />
                </div>
                <div>
                  <label className="text-xs font-medium text-muted-foreground block mb-1.5 uppercase tracking-wide">
                    {t('provider.multiplier', 'Price Multiplier')}
                  </label>
                  <div className="flex items-center gap-2">
                    <MultiplierInput
                      value={client.multiplier}
                      onChange={(value) => onUpdateClient(client.id, { multiplier: value })}
                      disabled={!client.enabled}
                    />
                    <span className="text-xs text-muted-foreground">×</span>
                    <span className="text-xs text-muted-foreground">
                      {t('provider.multiplierHint', '(1.00 = 100%)')}
                    </span>
                  </div>
                </div>
              </div>

              {client.id === 'claude' && disguise && onUpdateDisguise && (
                <div className="mt-5 space-y-4">
                  <div className="border-t border-border/60" />
                  <div>
                    <label className="text-xs font-medium text-muted-foreground block mb-1.5 uppercase tracking-wide">
                      {t('provider.disguiseType')}
                    </label>
                    <Select
                      value={disguise.type}
                      onValueChange={(value) =>
                        onUpdateDisguise({ type: value as DisguiseTypeValue })
                      }
                    >
                      <SelectTrigger className="w-full">
                        <SelectValue />
                      </SelectTrigger>
                      <SelectContent>
                        <SelectItem value="none">{t('provider.disguiseTypeNone')}</SelectItem>
                        <SelectItem value="claude-code">
                          {t('provider.disguiseTypeClaudeCode')}
                        </SelectItem>
                        <SelectItem value="bedrock">
                          {t('provider.disguiseTypeBedrock')}
                        </SelectItem>
                      </SelectContent>
                    </Select>
                    <p className="text-xs text-muted-foreground mt-1">
                      {disguise.type === 'bedrock'
                        ? t('provider.disguiseTypeBedrockDesc')
                        : disguise.type === 'claude-code'
                          ? t('provider.disguiseTypeClaudeCodeDesc')
                          : t('provider.disguiseTypeNoneDesc')}
                    </p>
                  </div>

                  {disguise.type === 'claude-code' && (
                    <>
                      <div>
                        <label className="text-xs font-medium text-muted-foreground block mb-1.5 uppercase tracking-wide">
                          {t('provider.cloakMode')}
                        </label>
                        <Select
                          value={disguise.claudeCodeMode}
                          onValueChange={(value) =>
                            onUpdateDisguise({
                              claudeCodeMode: value as 'auto' | 'always' | 'never',
                            })
                          }
                        >
                          <SelectTrigger className="w-full">
                            <SelectValue />
                          </SelectTrigger>
                          <SelectContent>
                            <SelectItem value="auto">{t('provider.cloakModeAuto')}</SelectItem>
                            <SelectItem value="always">{t('provider.cloakModeAlways')}</SelectItem>
                            <SelectItem value="never">{t('provider.cloakModeNever')}</SelectItem>
                          </SelectContent>
                        </Select>
                        <p className="text-xs text-muted-foreground mt-1">
                          {t('provider.cloakModeDesc')}
                        </p>
                      </div>

                      <div className="flex items-center justify-between">
                        <div>
                          <label className="text-xs font-medium text-muted-foreground uppercase tracking-wide block">
                            {t('provider.cloakStrictMode')}
                          </label>
                          <p className="text-xs text-muted-foreground mt-1">
                            {t('provider.cloakStrictModeDesc')}
                          </p>
                        </div>
                        <Switch
                          checked={disguise.claudeCodeStrictMode}
                          onCheckedChange={(checked) =>
                            onUpdateDisguise({ claudeCodeStrictMode: checked })
                          }
                        />
                      </div>

                      <div>
                        <label className="text-xs font-medium text-muted-foreground block mb-1.5 uppercase tracking-wide">
                          {t('provider.cloakSensitiveWords')}
                        </label>
                        <Textarea
                          value={disguise.claudeCodeSensitiveWords}
                          onChange={(e) =>
                            onUpdateDisguise({ claudeCodeSensitiveWords: e.target.value })
                          }
                          placeholder={t('provider.cloakSensitiveWordsPlaceholder')}
                          className="min-h-[100px] bg-card"
                        />
                        <p className="text-xs text-muted-foreground mt-1">
                          {t('provider.cloakSensitiveWordsDesc')}
                        </p>
                      </div>
                    </>
                  )}
                </div>
              )}
            </div>
          </div>
        ))}
      </div>
    </div>
  );
}
