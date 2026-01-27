import { useState } from 'react';
import { Button } from '@/components/ui';
import { Terminal, Check } from 'lucide-react';
import type { RequestInfo } from '@/lib/transport';
import { useSetting } from '@/hooks/queries';
import { useTranslation } from 'react-i18next';

interface CopyAsCurlButtonProps {
  requestInfo: RequestInfo;
}

function generateCurlCommand(requestInfo: RequestInfo, proxyPort: string): string {
  const parts: string[] = ['curl'];

  // Method (default is GET, so only add if different)
  if (requestInfo.method && requestInfo.method !== 'GET') {
    parts.push(`-X ${requestInfo.method}`);
  }

  // Build full URL using proxy server address
  const port = proxyPort || '9880';
  const baseUrl = `http://localhost:${port}`;
  let fullUrl = requestInfo.url;
  if (fullUrl && !fullUrl.startsWith('http://') && !fullUrl.startsWith('https://')) {
    fullUrl = `${baseUrl}${fullUrl}`;
  }

  parts.push(`'${fullUrl}'`);

  // Headers
  if (requestInfo.headers) {
    for (const [key, value] of Object.entries(requestInfo.headers)) {
      // Skip some headers that curl handles automatically or are not useful
      // Also skip Host header since we're using proxy server address
      const skipHeaders = ['content-length', 'connection', 'accept-encoding', 'host'];
      if (skipHeaders.includes(key.toLowerCase())) continue;

      // Escape single quotes in header values
      const escapedValue = value.replace(/'/g, "'\\''");
      parts.push(`-H '${key}: ${escapedValue}'`);
    }
  }

  // Body
  if (requestInfo.body) {
    // Escape single quotes in body
    const escapedBody = requestInfo.body.replace(/'/g, "'\\''");
    parts.push(`-d '${escapedBody}'`);
  }

  return parts.join(' \\\n  ');
}

export function CopyAsCurlButton({ requestInfo }: CopyAsCurlButtonProps) {
  const { t } = useTranslation();
  const [copied, setCopied] = useState(false);
  const { data: settingData } = useSetting('proxy_port');
  const proxyPort = settingData?.value || '9880';

  const handleCopy = async () => {
    try {
      const curlCommand = generateCurlCommand(requestInfo, proxyPort);
      await navigator.clipboard.writeText(curlCommand);
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    } catch (err) {
      console.error('Failed to copy:', err);
    }
  };

  return (
    <Button variant="outline" size="sm" onClick={handleCopy} className="h-6 px-2 text-[10px] gap-1">
      {copied ? (
        <>
          <Check className="h-3 w-3" />
          {t('common.copied')}
        </>
      ) : (
        <>
          <Terminal className="h-3 w-3" />
          cURL
        </>
      )}
    </Button>
  );
}
