export interface UserPanelEndpointHint {
  id: 'openai-codex' | 'claude' | 'gemini';
  url: string;
}

function trimTrailingSlash(value: string): string {
  return value.replace(/\/+$/, '');
}

export function buildUserPanelEndpointHints(origin: string): UserPanelEndpointHint[] {
  const baseUrl = trimTrailingSlash(origin);

  return [
    { id: 'openai-codex', url: `${baseUrl}/v1` },
    { id: 'claude', url: baseUrl },
    { id: 'gemini', url: `${baseUrl}/v1beta/models/{model}:generateContent` },
  ];
}

export function buildUserPanelChatCompletionsExample(params: {
  origin: string;
  tokenLabel: string;
}): string {
  const baseUrl = trimTrailingSlash(params.origin);
  return `curl ${baseUrl}/v1/chat/completions \
  -H "Authorization: Bearer <${params.tokenLabel}>" \
  -H "Content-Type: application/json" \
  -d '{"model":"gpt-4o-mini","messages":[{"role":"user","content":"hi"}]}'`;
}
