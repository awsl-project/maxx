import { FlaskConical } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { PageHeader } from '@/components/layout/page-header';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui';

export function TestFieldPage() {
  const { t } = useTranslation();

  return (
    <div className="flex flex-1 flex-col gap-4 p-4 pt-0">
      <PageHeader title={t('testField.title')} description={t('testField.description')} />

      <Card className="border-border bg-card">
        <CardHeader className="border-b border-border py-4">
          <CardTitle className="text-base font-medium flex items-center gap-2">
            <FlaskConical className="h-4 w-4 text-muted-foreground" />
            {t('testField.readyTitle')}
          </CardTitle>
        </CardHeader>
        <CardContent className="p-6 text-sm text-muted-foreground">
          {t('testField.readyDescription')}
        </CardContent>
      </Card>
    </div>
  );
}
