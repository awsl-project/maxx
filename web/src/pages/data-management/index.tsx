import { Database } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { PageHeader } from '@/components/layout/page-header';
import { BackupSection, DataRetentionSection } from '@/pages/settings';

export function DataManagementPage() {
  const { t } = useTranslation();

  return (
    <div className="flex flex-col h-full bg-background">
      <PageHeader
        icon={Database}
        iconClassName="text-zinc-500"
        title={t('dataManagement.title')}
        description={t('dataManagement.description')}
      />

      <div className="flex-1 overflow-y-auto p-4 md:p-6">
        <div className="space-y-6">
          <DataRetentionSection />
          <BackupSection />
        </div>
      </div>
    </div>
  );
}

export default DataManagementPage;
