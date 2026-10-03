import { useTranslation } from 'react-i18next';
import { Textarea } from '@/components/ui/textarea';

interface OpenAISystemPromptFieldProps {
  value?: string;
  onChange: (value: string) => void;
  disabled?: boolean;
}

export function OpenAISystemPromptField({
  value = '',
  onChange,
  disabled = false,
}: OpenAISystemPromptFieldProps) {
  const { t } = useTranslation();

  return (
    <div className="rounded-xl border border-border bg-card p-4">
      <div className="mb-3">
        <div className="text-sm font-medium text-foreground">
          {t('provider.openAISystemPrompt')}
        </div>
        <p className="mt-1 text-xs text-muted-foreground">{t('provider.openAISystemPromptDesc')}</p>
      </div>
      <Textarea
        value={value}
        onChange={(event) => onChange(event.target.value)}
        placeholder={t('provider.openAISystemPromptPlaceholder')}
        className="min-h-28 font-mono text-sm"
        disabled={disabled}
      />
      <p className="mt-2 text-xs text-muted-foreground">
        {value.trim()
          ? t('provider.openAISystemPromptActiveHint')
          : t('provider.openAISystemPromptEmptyHint')}
      </p>
    </div>
  );
}
