import { useEffect, useMemo, useState } from 'react';
import { useQueryClient } from '@tanstack/react-query';
import { Eye, Save } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { PageHeader } from '@/components/layout/page-header';
import { Badge, Button, Card, CardContent, CardHeader, CardTitle } from '@/components/ui';
import { Textarea } from '@/components/ui/textarea';
import { settingsKeys, useSettings, useUpdateSetting } from '@/hooks/queries';

const EXTERNAL_MODEL_LIST_SETTING_KEY = 'external_model_list';

function parseModelList(value: string) {
  return Array.from(
    new Set(
      value
        .split(/[\n,]+/)
        .map((model) => model.trim())
        .filter(Boolean),
    ),
  ).sort((a, b) => a.localeCompare(b));
}

export function ExternalModelsPage() {
  const { t } = useTranslation();
  const queryClient = useQueryClient();
  const { data: settings, isLoading } = useSettings();
  const updateSetting = useUpdateSetting();
  const storedList = settings?.[EXTERNAL_MODEL_LIST_SETTING_KEY] ?? '';
  const [value, setValue] = useState(storedList);
  const [saved, setSaved] = useState(false);
  const models = useMemo(() => parseModelList(value), [value]);
  const hasChanges = value !== storedList;

  useEffect(() => {
    setValue(storedList);
  }, [storedList]);

  const handleSave = async () => {
    const normalized = models.join('\n');
    await updateSetting.mutateAsync({ key: EXTERNAL_MODEL_LIST_SETTING_KEY, value: normalized });
    queryClient.setQueryData<Record<string, string>>(settingsKeys.all, {
      ...(settings ?? {}),
      [EXTERNAL_MODEL_LIST_SETTING_KEY]: normalized,
    });
    setValue(normalized);
    setSaved(true);
    window.setTimeout(() => setSaved(false), 1600);
  };

  return (
    <div className="flex h-full flex-col bg-background">
      <PageHeader
        icon={Eye}
        iconClassName="text-zinc-500"
        title={t('externalModels.title')}
        description={t('externalModels.description')}
      />

      <div className="flex-1 overflow-y-auto p-4 md:p-6">
        <Card className="border-border bg-card">
          <CardHeader className="border-b border-border">
            <CardTitle className="flex items-center justify-between gap-3 text-base font-medium">
              <span>{t('externalModels.editorTitle')}</span>
              <Badge variant="secondary">
                {t('externalModels.modelCount', { count: models.length })}
              </Badge>
            </CardTitle>
          </CardHeader>
          <CardContent className="space-y-4 p-5">
            <p className="text-sm text-muted-foreground">{t('externalModels.editorDesc')}</p>
            <Textarea
              className="min-h-72 font-mono text-sm"
              value={value}
              onChange={(event) => setValue(event.target.value)}
              disabled={isLoading || updateSetting.isPending}
              placeholder={t('externalModels.placeholder')}
            />
            <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
              <p className="text-xs text-muted-foreground">{t('externalModels.saveHint')}</p>
              <Button
                className="gap-2"
                onClick={handleSave}
                disabled={isLoading || updateSetting.isPending || !hasChanges}
              >
                <Save className="size-4" />
                {updateSetting.isPending
                  ? t('common.loading')
                  : saved
                    ? t('common.saved')
                    : t('common.save')}
              </Button>
            </div>
            {models.length > 0 ? (
              <div className="flex flex-wrap gap-2 rounded-xl border border-border bg-muted/25 p-4">
                {models.map((model) => (
                  <Badge key={model} variant="outline" className="max-w-full truncate font-mono">
                    {model}
                  </Badge>
                ))}
              </div>
            ) : (
              <div className="rounded-xl border border-dashed border-border bg-muted/20 p-4 text-sm text-muted-foreground">
                {t('externalModels.empty')}
              </div>
            )}
          </CardContent>
        </Card>
      </div>
    </div>
  );
}
