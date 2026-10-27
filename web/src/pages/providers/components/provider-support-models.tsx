import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Filter, Plus, Trash2 } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { ModelInput } from '@/components/ui/model-input';

export function ProviderSupportModels({
  supportModels,
  onChange,
}: {
  supportModels: string[];
  onChange: (models: string[]) => void;
}) {
  const { t } = useTranslation();
  const [newModel, setNewModel] = useState('');

  const handleAddModel = () => {
    if (!newModel.trim()) return;
    const trimmedModel = newModel.trim();
    if (!supportModels.includes(trimmedModel)) {
      onChange([...supportModels, trimmedModel]);
    }
    setNewModel('');
  };

  const handleRemoveModel = (model: string) => {
    onChange(supportModels.filter((m) => m !== model));
  };

  return (
    <div>
      <div className="flex items-center gap-2 mb-4 border-b border-border pb-2">
        <Filter size={18} className="text-blue-500" />
        <h4 className="text-lg font-semibold text-foreground">
          {t('providers.supportModels.title')}
        </h4>
        <span className="text-sm text-muted-foreground">({supportModels.length})</span>
      </div>

      <div className="bg-card border border-border rounded-xl p-4">
        <p className="text-xs text-muted-foreground mb-4">{t('providers.supportModels.desc')}</p>

        {supportModels.length > 0 && (
          <div className="flex flex-wrap gap-2 mb-4">
            {supportModels.map((model) => (
              <div
                key={model}
                className="flex items-center gap-1 bg-muted/50 border border-border rounded-lg px-3 py-1.5"
              >
                <span className="text-sm">{model}</span>
                <button
                  type="button"
                  onClick={() => handleRemoveModel(model)}
                  className="text-muted-foreground hover:text-destructive ml-1"
                >
                  <Trash2 className="h-3.5 w-3.5" />
                </button>
              </div>
            ))}
          </div>
        )}

        {supportModels.length === 0 && (
          <div className="text-center py-6 mb-4">
            <p className="text-muted-foreground text-sm">{t('providers.supportModels.empty')}</p>
          </div>
        )}

        <div className="flex items-center gap-2 pt-4 border-t border-border">
          <ModelInput
            value={newModel}
            onChange={setNewModel}
            placeholder={t('providers.supportModels.placeholder')}
            className="flex-1 min-w-0 h-8 text-sm"
          />
          <Button variant="outline" size="sm" onClick={handleAddModel} disabled={!newModel.trim()}>
            <Plus className="h-4 w-4 mr-1" />
            {t('common.add')}
          </Button>
        </div>
      </div>
    </div>
  );
}
