import { useState } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useTranslation } from 'react-i18next';
import { Button, Input } from '@/components/ui';
import { getTransport } from '@/lib/transport';
import type { PrivacyRecord } from '@/lib/transport/privacy-types';
import { Choice } from './controls';

function RecordDetail({ id, record }: { id: number; record: PrivacyRecord }) {
  const { t } = useTranslation();
  const original = useMutation({
    mutationFn: (placeholder: string) =>
      getTransport().getPrivacyOriginal(id, record.id, placeholder),
  });
  const sent = useMutation({ mutationFn: () => getTransport().getPrivacySent(id, record.id) });
  return (
    <div className="space-y-3 rounded-lg border border-border p-4">
      <p className="break-all text-sm font-medium">
        {record.id} · {record.status}
      </p>
      <ul className="space-y-3">
        {record.values.map((value, i) => (
          <li key={i} className="space-y-1 text-xs">
            <p className="break-all">
              {value.kind} · {value.by} · {value.action} · {value.placeholder}
            </p>
            <p>
              {t('privacy.valueCounts', {
                length: value.length,
                count: value.count,
                restored: value.restored,
              })}
            </p>
            <Button
              type="button"
              size="sm"
              variant="outline"
              disabled={original.isPending}
              onClick={() => original.mutate(value.placeholder)}
            >
              {t('privacy.reveal')}
            </Button>
            {original.variables === value.placeholder && original.data && (
              <pre className="whitespace-pre-wrap break-all rounded bg-bg-secondary p-2">
                {original.data.value}
              </pre>
            )}
          </li>
        ))}
      </ul>
      {original.error && (
        <p role="alert" className="text-sm text-text-secondary">
          {t('privacy.originalExpired')}
        </p>
      )}
      <Button
        type="button"
        variant="outline"
        disabled={sent.isPending}
        onClick={() => sent.mutate()}
      >
        {t('privacy.sentBody')}
      </Button>
      {sent.error && (
        <p role="alert" className="text-sm text-text-secondary">
          {t('privacy.originalExpired')}
        </p>
      )}
      {sent.data && (
        <pre className="max-h-96 overflow-auto whitespace-pre-wrap break-all rounded bg-bg-secondary p-3 text-xs">
          {sent.data.body}
        </pre>
      )}
      <p className="text-xs text-text-secondary">{t('privacy.memoryOnly')}</p>
    </div>
  );
}

export function PrivacyHistory({ id }: { id: number }) {
  const { t } = useTranslation();
  const client = useQueryClient();
  const [page, setPage] = useState(1);
  const [kind, setKind] = useState('');
  const [action, setAction] = useState('');
  const [by, setBy] = useState('');
  const [providerId, setProvider] = useState('');
  const [since, setSince] = useState('');
  const [selected, setSelected] = useState<PrivacyRecord>();
  const filters = {
    page,
    limit: 20,
    kind,
    action,
    by,
    providerId,
    ...(since ? { since: new Date(since).toISOString() } : {}),
  };
  const records = useQuery({
    queryKey: ['privacy', id, 'records', filters],
    queryFn: () => getTransport().getPrivacyRecords(id, filters),
    refetchInterval: 5000,
  });
  const summary = useQuery({
    queryKey: ['privacy', id, 'summary'],
    queryFn: () => getTransport().getPrivacySummary(id),
    refetchInterval: 5000,
  });
  const clear = useMutation({
    mutationFn: () => getTransport().clearPrivacyRecords(id),
    onSuccess: async () => {
      setSelected(undefined);
      setPage(1);
      await Promise.all([
        client.invalidateQueries({ queryKey: ['privacy', id, 'records'] }),
        client.invalidateQueries({ queryKey: ['privacy', id, 'summary'] }),
      ]);
    },
  });
  return (
    <div className="space-y-4">
      <p className="text-sm text-text-secondary">{t('privacy.historyHelp')}</p>
      {summary.data && (
        <div className="grid grid-cols-2 gap-3 sm:grid-cols-4">
          {(['Requests', 'Masked', 'Warned', 'Blocked', 'Restored', 'Bypassed'] as const).map(
            (name) => (
              <div key={name} className="rounded-lg border border-border p-3">
                <p className="text-xs text-text-secondary">{t(`privacy.stat.${name}`)}</p>
                <p className="text-xl font-semibold">{summary.data[name]}</p>
              </div>
            ),
          )}
        </div>
      )}
      <div className="flex flex-wrap gap-2">
        <Input
          className="w-36"
          aria-label={t('privacy.kind')}
          placeholder={t('privacy.kind')}
          value={kind}
          onChange={(e) => {
            setKind(e.target.value.toUpperCase());
            setPage(1);
          }}
        />
        <Choice
          aria-label={t('privacy.action')}
          value={action}
          onChange={(e) => {
            setAction(e.target.value);
            setPage(1);
          }}
        >
          <option value="">{t('privacy.allActions')}</option>
          {['mask', 'warn', 'block'].map((a) => (
            <option key={a} value={a}>
              {t(`privacy.actionName.${a}`)}
            </option>
          ))}
        </Choice>
        <Choice
          aria-label={t('privacy.finder')}
          value={by}
          onChange={(e) => {
            setBy(e.target.value);
            setPage(1);
          }}
        >
          <option value="">{t('privacy.allFinders')}</option>
          {['rule', 'yours', 'word', 'model', 'known'].map((b) => (
            <option key={b} value={b}>
              {b}
            </option>
          ))}
        </Choice>
        <Input
          className="w-36"
          type="number"
          min="1"
          aria-label={t('privacy.providerId')}
          placeholder={t('privacy.providerId')}
          value={providerId}
          onChange={(e) => {
            setProvider(e.target.value);
            setPage(1);
          }}
        />
        <Input
          className="w-auto"
          type="datetime-local"
          aria-label={t('privacy.since')}
          value={since}
          onChange={(e) => {
            setSince(e.target.value);
            setPage(1);
          }}
        />
      </div>
      {(records.error || summary.error || clear.error) && (
        <p role="alert" className="text-sm text-destructive">
          {t('privacy.historyFailed')}
        </p>
      )}
      <div className="overflow-x-auto">
        <table className="w-full text-left text-sm">
          <thead>
            <tr className="border-b border-border">
              <th className="p-2">{t('privacy.time')}</th>
              <th className="p-2">{t('privacy.providerId')}</th>
              <th className="p-2">{t('privacy.hits')}</th>
              <th className="p-2">{t('privacy.status')}</th>
              <th />
            </tr>
          </thead>
          <tbody>
            {records.data?.items.map((record) => (
              <tr key={record.id} className="border-b border-border">
                <td className="p-2 whitespace-nowrap">{new Date(record.time).toLocaleString()}</td>
                <td className="p-2">{record.providerId || '—'}</td>
                <td className="p-2">{record.values.reduce((n, v) => n + v.count, 0)}</td>
                <td className="p-2">{record.status}</td>
                <td className="p-2">
                  <Button
                    type="button"
                    variant="outline"
                    size="sm"
                    onClick={() => setSelected(record)}
                  >
                    {t('privacy.detail')}
                  </Button>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      {records.data?.total === 0 && (
        <p className="text-sm text-text-secondary">{t('privacy.noRecords')}</p>
      )}
      <div className="flex flex-wrap items-center gap-3">
        <Button
          type="button"
          variant="outline"
          disabled={page <= 1}
          onClick={() => setPage(page - 1)}
        >
          {t('privacy.previous')}
        </Button>
        <span className="text-sm">
          {page} / {Math.max(1, Math.ceil((records.data?.total ?? 0) / 20))}
        </span>
        <Button
          type="button"
          variant="outline"
          disabled={page * 20 >= (records.data?.total ?? 0)}
          onClick={() => setPage(page + 1)}
        >
          {t('privacy.next')}
        </Button>
        <Button
          type="button"
          variant="outline"
          disabled={clear.isPending}
          onClick={() => {
            if (window.confirm(t('privacy.clearConfirm'))) clear.mutate();
          }}
        >
          {t('privacy.clearHistory')}
        </Button>
      </div>
      {selected && <RecordDetail key={selected.id} id={id} record={selected} />}
    </div>
  );
}
