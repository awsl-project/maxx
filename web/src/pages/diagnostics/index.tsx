import { Activity } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { PageHeader } from '@/components/layout/page-header';
import { PprofSection, RequestDiagnosticsSection } from '@/pages/settings';

export function DiagnosticsPage() {
  const { t } = useTranslation();

  return (
    <div className="flex flex-col h-full bg-background">
      <PageHeader
        icon={Activity}
        iconClassName="text-zinc-500"
        title={t('diagnostics.title')}
        description={t('diagnostics.description')}
      />

      <div className="flex-1 overflow-y-auto p-4 md:p-6">
        <div className="space-y-6">
          <RequestDiagnosticsSection />
          <PprofSection />
        </div>
      </div>
    </div>
  );
}

export default DiagnosticsPage;
