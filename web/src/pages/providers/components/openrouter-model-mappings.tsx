import { useMemo, useState } from 'react';
import { ArrowRight, Plus, Trash2, Zap } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { ModelInput } from '@/components/ui/model-input';

/**
 * Inline editor for OpenRouter request-model mappings (RequestModel →
 * OpenRouter model id, e.g. "claude-3-5-sonnet" → "anthropic/claude-3.5-sonnet").
 * Edits a plain Record<string,string> stored on config.openrouter.modelMapping.
 * Shared by the create step and the edit view so both produce identical shapes.
 */
export function OpenRouterModelMappings({
  mappings,
  onChange,
  disabled,
}: {
  mappings: Record<string, string>;
  onChange: (mappings: Record<string, string>) => void;
  disabled?: boolean;
}) {
  const { t } = useTranslation();
  const [newPattern, setNewPattern] = useState('');
  const [newTarget, setNewTarget] = useState('');
  const entries = useMemo(() => Object.entries(mappings), [mappings]);

  const handleAdd = () => {
    const pattern = newPattern.trim();
    const target = newTarget.trim();
    if (!pattern || !target) return;
    onChange({ ...mappings, [pattern]: target });
    setNewPattern('');
    setNewTarget('');
  };

  const handleUpdate = (oldPattern: string, pattern: string, target: string) => {
    const next: Record<string, string> = {};
    for (const [key, value] of entries) {
      if (key === oldPattern) continue;
      next[key] = value;
    }
    const nextPattern = pattern.trim();
    const nextTarget = target.trim();
    if (nextPattern && nextTarget) {
      next[nextPattern] = nextTarget;
    }
    onChange(next);
  };

  const handleDelete = (pattern: string) => {
    const next: Record<string, string> = {};
    for (const [key, value] of entries) {
      if (key === pattern) continue;
      next[key] = value;
    }
    onChange(next);
  };

  return (
    <div>
      <div className="flex items-center gap-2 mb-4 border-b border-border pb-2">
        <Zap size={18} className="text-yellow-500" />
        <h4 className="text-lg font-semibold text-foreground">
          {t('addProvider.openrouter.modelMappingTitle')}
        </h4>
        <span className="text-sm text-muted-foreground">({entries.length})</span>
      </div>

      <div className="bg-card border border-border rounded-xl p-4">
        <p className="text-xs text-muted-foreground mb-4">
          {t('addProvider.openrouter.modelMappingDesc')}
        </p>

        {entries.length > 0 && (
          <div className="space-y-2 mb-4">
            {entries.map(([pattern, target], index) => (
              <div key={pattern} className="flex items-center gap-2">
                <span className="text-xs text-muted-foreground w-6 shrink-0">{index + 1}.</span>
                <ModelInput
                  value={pattern}
                  onChange={(value) => handleUpdate(pattern, value, target)}
                  placeholder={t('addProvider.openrouter.modelMappingFrom')}
                  disabled={disabled}
                  className="flex-1 min-w-0 h-8 text-sm"
                />
                <ArrowRight className="h-4 w-4 text-muted-foreground shrink-0" />
                <Input
                  value={target}
                  onChange={(event) => handleUpdate(pattern, pattern, event.target.value)}
                  placeholder={t('addProvider.openrouter.modelMappingTo')}
                  disabled={disabled}
                  className="flex-1 min-w-0 h-8 text-sm font-mono"
                />
                <Button
                  variant="ghost"
                  size="sm"
                  onClick={() => handleDelete(pattern)}
                  disabled={disabled}
                >
                  <Trash2 className="h-4 w-4 text-destructive" />
                </Button>
              </div>
            ))}
          </div>
        )}

        {entries.length === 0 && (
          <div className="text-center py-6 mb-4">
            <p className="text-muted-foreground text-sm">
              {t('addProvider.openrouter.modelMappingEmpty')}
            </p>
          </div>
        )}

        <div className="flex items-center gap-2 pt-4 border-t border-border">
          <ModelInput
            value={newPattern}
            onChange={setNewPattern}
            placeholder={t('addProvider.openrouter.modelMappingFrom')}
            disabled={disabled}
            className="flex-1 min-w-0 h-8 text-sm"
          />
          <ArrowRight className="h-4 w-4 text-muted-foreground shrink-0" />
          <Input
            value={newTarget}
            onChange={(event) => setNewTarget(event.target.value)}
            placeholder={t('addProvider.openrouter.modelMappingTo')}
            disabled={disabled}
            className="flex-1 min-w-0 h-8 text-sm font-mono"
          />
          <Button
            variant="outline"
            size="sm"
            onClick={handleAdd}
            disabled={!newPattern.trim() || !newTarget.trim() || disabled}
          >
            <Plus className="h-4 w-4 mr-1" />
            {t('common.add')}
          </Button>
        </div>
      </div>
    </div>
  );
}

/**
 * Two-switch client selector (Claude + OpenAI) for OpenRouter, both natively
 * supported. At least one must stay enabled — enforced by the caller's isValid.
 */
export const OPENROUTER_CLIENT_TYPES = ['claude', 'openai'] as const;
