import { useState, useEffect } from 'react';
import { Switch } from '@/components/ui';
import { Input } from '@/components/ui/input';
import { ClientIcon } from '@/components/icons/client-icons';
import type { ClientType } from '@/lib/transport';
import type { ClientConfig } from '../types';
import { useTranslation } from 'react-i18next';

interface ClientsConfigSectionProps {
  clients: ClientConfig[];
  onUpdateClient: (clientId: ClientType, updates: Partial<ClientConfig>) => void;
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

export function ClientsConfigSection({ clients, onUpdateClient }: ClientsConfigSectionProps) {
  const { t } = useTranslation();
  return (
    <div>
      <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-6">
        {clients.map((client) => (
          <div
            key={client.id}
            className={`rounded-xl border transition-all duration-200 flex flex-col ${
              client.enabled
                ? 'bg-card border-border shadow-sm'
                : 'bg-muted/30 border-transparent opacity-80 hover:opacity-100 hover:bg-muted/50'
            }`}
          >
            <div className="flex items-center justify-between p-4 border-b border-transparent">
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
              className={`px-4 pb-4 transition-all duration-200 ${client.enabled ? 'opacity-100' : 'opacity-50 grayscale pointer-events-none'}`}
            >
              <div className="space-y-3">
                <div className="bg-muted/50 rounded-lg p-3 border border-border/50">
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
                <div className="bg-muted/50 rounded-lg p-3 border border-border/50">
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
            </div>
          </div>
        ))}
      </div>
    </div>
  );
}
