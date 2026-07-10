import { useNavigate, useParams } from 'react-router-dom';
import { useProviders } from '@/hooks/queries';
import { ProviderEditFlow } from './components/provider-edit-flow';
import { useTranslation } from 'react-i18next';
import { Loader2, Lock } from 'lucide-react';

export function ProviderEditPage() {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const { id } = useParams<{ id: string }>();
  const { data: providers, isLoading } = useProviders();

  const provider = providers?.find((p) => p.id + '' === id + '');

  if (isLoading) {
    return (
      <div className="flex items-center justify-center h-full bg-background">
        <Loader2 className="h-8 w-8 animate-spin text-accent" />
      </div>
    );
  }

  if (!provider) {
    return (
      <div className="flex items-center justify-center h-full bg-background">
        <div className="text-muted-foreground">{t('providers.notFound')}</div>
      </div>
    );
  }

  if (provider.blackBox) {
    return (
      <div className="flex items-center justify-center h-full bg-background">
        <div className="max-w-md rounded-2xl border border-border bg-card p-8 text-center shadow-sm">
          <div className="mx-auto mb-4 flex h-12 w-12 items-center justify-center rounded-full bg-zinc-500/10 text-zinc-500">
            <Lock className="h-6 w-6" />
          </div>
          <div className="text-lg font-semibold text-foreground">
            {t('provider.blackBoxLockedTitle')}
          </div>
          <p className="mt-2 text-sm text-muted-foreground">{t('provider.blackBoxLockedDesc')}</p>
          <button
            type="button"
            onClick={() => navigate('/providers', { replace: true })}
            className="mt-6 rounded-lg bg-accent px-4 py-2 text-sm font-semibold text-accent-foreground hover:bg-accent/90"
          >
            {t('provider.backToProviders')}
          </button>
        </div>
      </div>
    );
  }

  return (
    <ProviderEditFlow
      key={provider.id}
      provider={provider}
      onClose={() => navigate('/providers')}
    />
  );
}
