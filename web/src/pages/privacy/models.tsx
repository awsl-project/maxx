import { useMutation, useQuery } from '@tanstack/react-query';
import { useTranslation } from 'react-i18next';
import { Button, Input } from '@/components/ui';
import { getTransport } from '@/lib/transport';
import type { PrivacyConfig } from '@/lib/transport/privacy-types';
import { Choice, Field, Toggle } from './controls';

export function PrivacyModels({
  id,
  model,
  setModel,
}: {
  id: number;
  model: PrivacyConfig['model'];
  setModel: (model: PrivacyConfig['model']) => void;
}) {
  const { t } = useTranslation();
  const status = useQuery({
    queryKey: ['privacy', id, 'models'],
    queryFn: () => getTransport().getPrivacyModels(id),
    refetchInterval: 3000,
  });
  const action = useMutation({
    mutationFn: ({ name, verb }: { name: string; verb: string }) =>
      getTransport().privacyModelAction(id, name, verb),
    onSuccess: () => status.refetch(),
  });
  return (
    <div className="space-y-4">
      <Toggle
        label={t('privacy.modelEnabled')}
        checked={model.enabled}
        onChange={(enabled) => setModel({ ...model, enabled })}
        description={t('privacy.modelStrict')}
      />
      <div className="grid gap-4 sm:grid-cols-2">
        <Field label={t('privacy.model')}>
          <Choice
            value={model.name}
            onChange={(e) =>
              setModel({ ...model, name: e.target.value as PrivacyConfig['model']['name'] })
            }
          >
            <option value="astrlink-guard">AstrLink Guard (~39 MB)</option>
            <option value="privacy-filter">Privacy Filter (~1.6 GB)</option>
          </Choice>
        </Field>
        <Field label={t('privacy.threshold')} description={t('privacy.thresholdHelp')}>
          <Input
            type="number"
            min="0"
            max="0.99"
            step="0.01"
            value={model.score}
            onChange={(e) => setModel({ ...model, score: Number(e.target.value) })}
          />
        </Field>
      </div>
      <p className="text-xs text-text-secondary">{t('privacy.sharedModels')}</p>
      {(status.error || action.error) && (
        <p role="alert" className="text-sm text-destructive">
          {t('privacy.modelFailed')}
        </p>
      )}
      {status.data?.items.map((item) => (
        <div key={item.model} className="space-y-2 rounded-lg border border-border p-3">
          <div className="flex flex-wrap justify-between gap-2 text-sm">
            <strong>{item.model}</strong>
            <span>
              {t(
                item.loading
                  ? 'privacy.loadingModel'
                  : item.running
                    ? 'privacy.modelRunning'
                    : item.installed
                      ? 'privacy.installed'
                      : 'privacy.notInstalled',
              )}
              {item.device && ` · ${item.device}`}
            </span>
          </div>
          {item.downloading && (
            <div>
              <progress
                aria-label={t('privacy.downloadProgress')}
                className="w-full"
                value={item.done ?? 0}
                max={item.total || item.size}
              />
              <p className="text-xs">
                {((item.done ?? 0) / 1048576).toFixed(1)} /{' '}
                {((item.total || item.size) / 1048576).toFixed(1)} MB
              </p>
            </div>
          )}
          {(item.error || item.failed) && (
            <p className="text-xs text-destructive">{item.error || item.failed}</p>
          )}
          {status.data.canManage && (
            <div className="flex flex-wrap gap-2">
              {(item.downloading
                ? ['cancel']
                : item.installed
                  ? ['warm', 'stop', 'remove']
                  : ['download']
              ).map((verb) => (
                <Button
                  key={verb}
                  type="button"
                  variant="outline"
                  size="sm"
                  disabled={action.isPending || !item.supported}
                  onClick={() => {
                    if (verb !== 'remove' || window.confirm(t('privacy.removeModelConfirm')))
                      action.mutate({ name: item.model, verb });
                  }}
                >
                  {t(`privacy.modelAction.${verb}`)}
                </Button>
              ))}
            </div>
          )}
        </div>
      ))}
    </div>
  );
}
