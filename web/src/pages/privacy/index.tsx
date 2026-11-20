import { useTranslation } from 'react-i18next';
import { ShieldCheck } from 'lucide-react';
import { PageHeader } from '@/components/layout';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui';
import { PrivacySettings } from './settings';

export function PrivacyPage() {
  const { t } = useTranslation();
  return (
    <div className="flex h-full flex-col overflow-hidden">
      <PageHeader
        icon={ShieldCheck}
        title={t('privacy.title')}
        description={t('privacy.description')}
      />
      <div className="flex-1 overflow-auto p-4 sm:p-6 space-y-6">
        <Card>
          <CardHeader>
            <CardTitle className="text-base">{t('privacy.archiveTitle')}</CardTitle>
          </CardHeader>
          <CardContent>
            <p className="text-sm text-text-secondary">{t('privacy.archiveDescription')}</p>
          </CardContent>
        </Card>
        <PrivacySettings />
      </div>
    </div>
  );
}
