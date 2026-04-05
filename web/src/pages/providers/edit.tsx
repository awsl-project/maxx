import { useNavigate, useParams } from 'react-router-dom';
import { useProviders } from '@/hooks/queries';
import { ProviderEditFlow } from './components/provider-edit-flow';
import { useTranslation } from 'react-i18next';
import { Loader2 } from 'lucide-react';

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

  return (
    <ProviderEditFlow
      key={provider.id}
      provider={provider}
      onClose={() => navigate('/providers')}
    />
  );
}
