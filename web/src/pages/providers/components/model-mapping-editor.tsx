import { useState } from 'react';
import { Plus, Trash2, ArrowRight } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { Button } from '@/components/ui/button';
import { ModelInput } from '@/components/ui/model-input';

interface ModelMappingEditorProps {
  value: Record<string, string>;
  onChange: (value: Record<string, string>) => void;
  disabled?: boolean;
  /** Only show Antigravity-supported models for target selection */
  targetOnlyAntigravity?: boolean;
}

export function ModelMappingEditor({
  value,
  onChange,
  disabled = false,
  targetOnlyAntigravity = false,
}: ModelMappingEditorProps) {
  const { t } = useTranslation();
  const [newFrom, setNewFrom] = useState('');
  const [newTo, setNewTo] = useState('');

  const entries = Object.entries(value);

  // Target model providers filter
  const targetProviders = targetOnlyAntigravity ? ['Antigravity' as const] : undefined;

  const handleAdd = () => {
    if (!newFrom.trim() || !newTo.trim()) return;
    if (value[newFrom.trim()]) return; // Already exists

    onChange({
      ...value,
      [newFrom.trim()]: newTo.trim(),
    });
    setNewFrom('');
    setNewTo('');
  };

  const handleRemove = (key: string) => {
    const newValue = { ...value };
    delete newValue[key];
    onChange(newValue);
  };

  const handleUpdate = (oldKey: string, newKey: string, newVal: string) => {
    const normalizedKey = newKey.trim();
    const normalizedVal = newVal.trim();
    if (!normalizedKey || !normalizedVal) return;
    if (oldKey !== normalizedKey && value[normalizedKey]) return; // prevent overwrite

    const newValue = { ...value };
    if (oldKey !== normalizedKey) {
      delete newValue[oldKey];
    }
    newValue[normalizedKey] = normalizedVal;
    onChange(newValue);
  };

  return (
    <div className="space-y-3">
      {entries.length > 0 && (
        <div className="space-y-2">
          {entries.map(([from, to]) => (
            <div key={from} className="flex items-center gap-2">
              <ModelInput
                value={from}
                onChange={(newKey) => handleUpdate(from, newKey, to)}
                placeholder={t('modelMapping.requestModel')}
                className="flex-1"
                disabled={disabled}
              />
              <ArrowRight size={16} className="text-muted-foreground shrink-0" />
              <ModelInput
                value={to}
                onChange={(newVal) => handleUpdate(from, from, newVal)}
                placeholder={t('modelMapping.mappedModel')}
                className="flex-1"
                disabled={disabled}
                providers={targetProviders}
              />
              <Button
                type="button"
                variant="ghost"
                size="icon"
                onClick={() => handleRemove(from)}
                disabled={disabled}
                className="shrink-0 text-muted-foreground hover:text-error"
              >
                <Trash2 size={16} />
              </Button>
            </div>
          ))}
        </div>
      )}

      {/* Add new mapping */}
      <div className="flex items-center gap-2">
        <ModelInput
          value={newFrom}
          onChange={setNewFrom}
          placeholder={t('modelMapping.requestModel')}
          className="flex-1"
          disabled={disabled}
        />
        <ArrowRight size={16} className="text-muted-foreground shrink-0" />
        <ModelInput
          value={newTo}
          onChange={setNewTo}
          placeholder={t('modelMapping.mappedModel')}
          className="flex-1"
          disabled={disabled}
          providers={targetProviders}
        />
        <Button
          type="button"
          variant="secondary"
          size="icon"
          onClick={handleAdd}
          disabled={disabled || !newFrom.trim() || !newTo.trim()}
          className="shrink-0"
        >
          <Plus size={16} />
        </Button>
      </div>

      {entries.length === 0 && (
        <p className="text-xs text-muted-foreground">
          {t('modelMapping.emptyHint')}
        </p>
      )}
    </div>
  );
}
