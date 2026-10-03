import { useId } from 'react';
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
  const textareaId = useId();

  return (
    <div className="rounded-xl border border-border bg-card p-4">
      <label htmlFor={textareaId} className="mb-3 block text-sm font-medium text-foreground">
        {t('provider.openAISystemPrompt')}
      </label>
      <Textarea
        id={textareaId}
        value={value}
        onChange={(event) => onChange(event.target.value)}
        placeholder={t('provider.openAISystemPromptPlaceholder')}
        className="min-h-28 font-mono text-sm"
        disabled={disabled}
      />
    </div>
  );
}
