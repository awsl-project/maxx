import { Textarea } from '@/components/ui/textarea';
import { useState } from 'react';
import { useMutation } from '@tanstack/react-query';
import { useTranslation } from 'react-i18next';
import { Button } from '@/components/ui';
import { getTransport } from '@/lib/transport';
import type { PrivacyConfig } from '@/lib/transport/privacy-types';
import { Toggle } from './controls';

export function PrivacyPreview({ config }: { config: PrivacyConfig }) {
  const { t } = useTranslation();
  const [text, setText] = useState('');
  const [json, setJSON] = useState(false);
  const preview = useMutation({
    mutationFn: () => getTransport().previewPrivacy(config, text, json),
  });
  return (
    <div className="space-y-3">
      <p className="text-sm text-text-secondary">{t('privacy.previewHelp')}</p>
      <Toggle label={t('privacy.jsonBody')} checked={json} onChange={setJSON} />
      <Textarea
        aria-label={t('privacy.previewInput')}
        value={text}
        onChange={(e) => {
          setText(e.target.value);
          preview.reset();
        }}
        rows={5}
        maxLength={250000}
      />
      <Button type="button" disabled={preview.isPending || !text} onClick={() => preview.mutate()}>
        {t('privacy.runPreview')}
      </Button>
      {preview.error && (
        <p role="alert" className="text-sm text-destructive">
          {t('privacy.previewFailed')}
        </p>
      )}
      {preview.data && (
        <div className="space-y-3">
          <p className="text-sm">
            {t(preview.data.blocked ? 'privacy.previewBlocked' : 'privacy.previewAllowed')}
          </p>
          <pre className="overflow-auto rounded-lg bg-bg-secondary p-3 text-xs whitespace-pre-wrap break-all">
            {preview.data.body}
          </pre>
          <ul className="space-y-1 text-xs">
            {preview.data.hits.map((hit, index) => (
              <li key={index} className="break-all">
                {hit.Kind} · {hit.By} · {hit.Action || 'mask'} · {hit.Placeholder} · {hit.Len}
              </li>
            ))}
          </ul>
        </div>
      )}
    </div>
  );
}
