import { ShieldCheck } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { PageHeader } from '@/components/layout/page-header';
import {
  AntigravitySection,
  ForceProjectSection,
  OpenAIChatStreamTimeoutSection,
  ProxyKillSwitchSection,
  ProxyRouteExposureSection,
} from '@/pages/settings';

export function ProxyAccessPage() {
  const { t } = useTranslation();

  return (
    <div className="flex flex-col h-full bg-background">
      <PageHeader
        icon={ShieldCheck}
        iconClassName="text-zinc-500"
        title={t('proxyAccess.title')}
        description={t('proxyAccess.description')}
      />

      <div className="flex-1 overflow-y-auto p-4 md:p-6">
        <div className="space-y-6">
          <ProxyKillSwitchSection />
          <ForceProjectSection />
          <ProxyRouteExposureSection />
          <OpenAIChatStreamTimeoutSection />
          <AntigravitySection />
        </div>
      </div>
    </div>
  );
}

export default ProxyAccessPage;
