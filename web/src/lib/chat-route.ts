import type { ClientType, UserPanelAvailableModelRouteGroup } from '@/lib/transport';

export type ChatRouteType = Extract<ClientType, 'openai' | 'claude' | 'gemini' | 'codex'>;

export interface ChatRouteConfig {
  type: ChatRouteType;
  label: string;
  defaultModel: string;
}

export const CHAT_ROUTE_CONFIGS: ChatRouteConfig[] = [
  { type: 'openai', label: 'OpenAI', defaultModel: 'gpt-5' },
  { type: 'claude', label: 'Claude', defaultModel: 'claude-sonnet-4-5' },
  { type: 'gemini', label: 'Gemini', defaultModel: 'gemini-2.5-pro' },
  { type: 'codex', label: 'Codex', defaultModel: 'gpt-5' },
];

const CHAT_ROUTE_TYPE_SET = new Set<ChatRouteType>(CHAT_ROUTE_CONFIGS.map((config) => config.type));

export function getChatRouteConfig(type: ChatRouteType): ChatRouteConfig {
  return CHAT_ROUTE_CONFIGS.find((config) => config.type === type) ?? CHAT_ROUTE_CONFIGS[0];
}

export function getChatEndpoint(
  routeType: ChatRouteType,
  model: string,
  options: { stream?: boolean } = {},
): string {
  if (routeType === 'gemini') {
    const cleanModel = model.replace(/^models\//, '') || getChatRouteConfig(routeType).defaultModel;
    const action = options.stream ? 'streamGenerateContent?alt=sse' : 'generateContent';
    return `/v1beta/models/${encodeURIComponent(cleanModel)}:${action}`;
  }
  if (routeType === 'claude') return '/v1/messages';
  if (routeType === 'codex') return '/v1/responses';
  return '/v1/chat/completions';
}

export function getChatModelRoutesEndpoint(): string {
  return '/api/chat/model-routes';
}

export function parseChatHashToken(hash: string): string {
  const value = hash.startsWith('#') ? hash.slice(1) : hash;
  if (!value) return '';

  try {
    if (value.startsWith('token=') || value.startsWith('key=') || value.startsWith('api_key=')) {
      const params = new URLSearchParams(value);
      return params.get('token') ?? params.get('key') ?? params.get('api_key') ?? '';
    }

    return decodeURIComponent(value.trim());
  } catch {
    return value.trim();
  }
}

export function isChatRouteType(value: unknown): value is ChatRouteType {
  return typeof value === 'string' && CHAT_ROUTE_TYPE_SET.has(value as ChatRouteType);
}

export function normalizeChatModelRouteGroups(
  payload: unknown,
): UserPanelAvailableModelRouteGroup[] {
  if (!Array.isArray(payload)) return [];

  return payload.filter((item): item is UserPanelAvailableModelRouteGroup => {
    if (!item || typeof item !== 'object') return false;
    const group = item as Partial<UserPanelAvailableModelRouteGroup>;
    return (
      isChatRouteType(group.clientType) && Array.isArray(group.models) && group.models.length > 0
    );
  });
}

export function getExposedChatRouteTypes(
  groups: UserPanelAvailableModelRouteGroup[],
): ChatRouteType[] {
  const exposed = new Set(groups.map((group) => group.clientType).filter(isChatRouteType));
  return CHAT_ROUTE_CONFIGS.map((config) => config.type).filter((type) => exposed.has(type));
}

export function normalizeChatModelList(
  groups: UserPanelAvailableModelRouteGroup[],
  routeType: ChatRouteType,
): string[] {
  const models = groups
    .filter((group) => group.clientType === routeType)
    .flatMap((group) => group.models)
    .map((model) => (typeof model === 'string' ? model.trim().replace(/^models\//, '') : ''))
    .filter(Boolean);

  return Array.from(new Set(models)).sort((a, b) => a.localeCompare(b));
}

export function buildChatRequestBody(
  routeType: ChatRouteType,
  model: string,
  messages: { role: 'user' | 'assistant'; content: string }[],
  options: { stream?: boolean } = {},
) {
  const fallbackModel = getChatRouteConfig(routeType).defaultModel;
  const selectedModel = model.trim() || fallbackModel;
  const userMessages = messages.map((message) => ({
    role: message.role,
    content: message.content,
  }));
  switch (routeType) {
    case 'claude':
      return {
        model: selectedModel,
        max_tokens: 1024,
        messages: userMessages,
        stream: options.stream ?? false,
      };
    case 'gemini':
      return {
        contents: messages.map((message) => ({
          role: message.role === 'assistant' ? 'model' : 'user',
          parts: [{ text: message.content }],
        })),
      };
    case 'codex':
      return {
        model: selectedModel,
        input: userMessages.map((message) => ({ role: message.role, content: message.content })),
        stream: options.stream ?? false,
      };
    case 'openai':
    default:
      return { model: selectedModel, messages: userMessages, stream: options.stream ?? false };
  }
}

export function extractChatResponseText(routeType: ChatRouteType, payload: unknown): string {
  if (!payload || typeof payload !== 'object') return '';
  const data = payload as Record<string, unknown>;

  if (routeType === 'openai') {
    const choices = data.choices as Array<{ message?: { content?: string } }> | undefined;
    return choices?.[0]?.message?.content ?? '';
  }

  if (routeType === 'claude') {
    const content = data.content as Array<{ text?: string }> | undefined;
    return (
      content
        ?.map((part) => part.text)
        .filter(Boolean)
        .join('\n') ?? ''
    );
  }

  if (routeType === 'gemini') {
    const candidates = data.candidates as
      | Array<{ content?: { parts?: Array<{ text?: string }> } }>
      | undefined;
    return (
      candidates?.[0]?.content?.parts
        ?.map((part) => part.text)
        .filter(Boolean)
        .join('\n') ?? ''
    );
  }

  const outputText = data.output_text;
  if (typeof outputText === 'string') return outputText;
  const output = data.output as Array<{ content?: Array<{ text?: string }> }> | undefined;
  return (
    output
      ?.flatMap((item) => item.content ?? [])
      .map((part) => part.text)
      .filter(Boolean)
      .join('\n') ?? ''
  );
}

export function extractChatStreamDelta(routeType: ChatRouteType, payload: unknown): string {
  if (!payload || typeof payload !== 'object') return '';
  const data = payload as Record<string, unknown>;

  if (routeType === 'openai') {
    const choices = data.choices as Array<{ delta?: { content?: string } }> | undefined;
    return choices?.[0]?.delta?.content ?? '';
  }

  if (routeType === 'claude') {
    if (data.type === 'content_block_delta') {
      const delta = data.delta as { text?: string } | undefined;
      return delta?.text ?? '';
    }
    return '';
  }

  if (routeType === 'gemini') {
    const candidates = data.candidates as
      | Array<{ content?: { parts?: Array<{ text?: string }> } }>
      | undefined;
    return (
      candidates?.[0]?.content?.parts
        ?.map((part) => part.text)
        .filter(Boolean)
        .join('') ?? ''
    );
  }

  if (data.type === 'response.output_text.delta' && typeof data.delta === 'string') {
    return data.delta;
  }
  return '';
}
