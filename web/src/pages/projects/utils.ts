import type { TFunction } from 'i18next';
import type { ClientType } from '@/lib/transport';

const clientTypeKeyMap: Record<ClientType, string> = {
  claude: 'claude',
  openai: 'openai',
  codex: 'codex',
  gemini: 'gemini',
};

export function getClientTypeLabel(t: TFunction, clientType: ClientType): string {
  const key = clientTypeKeyMap[clientType];
  if (!key) {
    return clientType;
  }

  return t(`clientTypes.${key}`);
}
