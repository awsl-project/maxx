import { useEffect, useMemo, useRef, useState } from 'react';
import { ArrowUp, Bot, KeyRound, Loader2, Route, User } from 'lucide-react';
import { Badge, Button, Input } from '@/components/ui';
import { Textarea } from '@/components/ui/textarea';
import {
  buildChatRequestBody,
  CHAT_ROUTE_CONFIGS,
  extractChatResponseText,
  extractChatStreamDelta,
  getChatEndpoint,
  getChatModelRoutesEndpoint,
  getChatRouteConfig,
  getExposedChatRouteTypes,
  normalizeChatModelList,
  normalizeChatModelRouteGroups,
  parseChatHashToken,
  type ChatRouteType,
} from '@/lib/chat-route';
import { cn } from '@/lib/utils';

type ChatMessage = {
  id: string;
  role: 'user' | 'assistant';
  content: string;
  transient?: boolean;
};

const welcomeMessages: ChatMessage[] = [
  {
    id: 'welcome',
    role: 'assistant',
    content: 'Choose a route type and model, then send a prompt.',
  },
];

function buildMessageID(prefix: string) {
  return `${prefix}-${Date.now()}-${Math.random().toString(16).slice(2)}`;
}

export function ChatPage() {
  const [routeType, setRouteType] = useState<ChatRouteType>('openai');
  const [token, setToken] = useState('');
  const [model, setModel] = useState(getChatRouteConfig('openai').defaultModel);
  const [models, setModels] = useState<string[]>([]);
  const [exposedRouteTypes, setExposedRouteTypes] = useState<ChatRouteType[]>([]);
  const [modelsError, setModelsError] = useState('');
  const [isLoadingModels, setIsLoadingModels] = useState(false);
  const [messages, setMessages] = useState<ChatMessage[]>(welcomeMessages);
  const inputRef = useRef<HTMLTextAreaElement | null>(null);
  const transcriptRef = useRef<HTMLDivElement | null>(null);
  const tokenRef = useRef('');
  const [hasInput, setHasInput] = useState(false);
  const [error, setError] = useState('');
  const [isSending, setIsSending] = useState(false);

  const routeConfig = useMemo(() => getChatRouteConfig(routeType), [routeType]);
  const tokenPresent = token.trim().length > 0;
  const selectedModel = model.trim() || routeConfig.defaultModel;
  const endpoint = getChatEndpoint(routeType, selectedModel, { stream: true });
  const visibleRouteConfigs = useMemo(
    () => CHAT_ROUTE_CONFIGS.filter((config) => exposedRouteTypes.includes(config.type)),
    [exposedRouteTypes],
  );

  const updateAssistantMessage = (
    messageID: string,
    content: string,
    options: { transient?: boolean } = {},
  ) => {
    setMessages((current) =>
      current.map((message) =>
        message.id === messageID ? { ...message, content, ...options } : message,
      ),
    );
  };

  useEffect(() => {
    const refreshToken = () => {
      const nextToken = parseChatHashToken(window.location.hash);
      if (tokenRef.current && tokenRef.current !== nextToken) {
        setMessages(welcomeMessages);
        setError('');
      }
      tokenRef.current = nextToken;
      setToken(nextToken);
    };
    refreshToken();
    window.addEventListener('hashchange', refreshToken);
    return () => window.removeEventListener('hashchange', refreshToken);
  }, []);

  useEffect(() => {
    setModel(routeConfig.defaultModel);
    setModels([]);
    setModelsError('');
  }, [routeConfig.defaultModel, routeType]);

  useEffect(() => {
    if (!tokenPresent) return;

    const controller = new AbortController();
    setIsLoadingModels(true);
    setModelsError('');

    void fetch(getChatModelRoutesEndpoint(), {
      credentials: 'same-origin',
      headers: { Authorization: `Bearer ${token}` },
      signal: controller.signal,
    })
      .then(async (response) => {
        const payload = await response.json().catch(() => ({}));
        if (!response.ok) throw new Error(`Model list failed with ${response.status}`);
        const groups = normalizeChatModelRouteGroups(payload);
        return {
          routeTypes: getExposedChatRouteTypes(groups),
          models: normalizeChatModelList(groups, routeType),
        };
      })
      .then(({ routeTypes, models: nextModels }) => {
        if (controller.signal.aborted) return;
        setExposedRouteTypes(routeTypes);
        if (routeTypes.length > 0 && !routeTypes.includes(routeType)) {
          setRouteType(routeTypes[0]);
          return;
        }
        setModels(nextModels);
        if (nextModels.length > 0) setModel(nextModels[0]);
      })
      .catch((err) => {
        if (controller.signal.aborted) return;
        setModelsError(err instanceof Error ? err.message : 'Failed to load models.');
      })
      .finally(() => {
        if (!controller.signal.aborted) setIsLoadingModels(false);
      });

    return () => controller.abort();
  }, [routeType, token, tokenPresent]);

  useEffect(() => {
    const transcript = transcriptRef.current;
    if (!transcript) return;
    transcript.scrollTop = transcript.scrollHeight;
  }, [messages, isSending]);

  const handleSend = async () => {
    const content = inputRef.current?.value.trim() ?? '';
    if (!content || isSending) return;
    if (!tokenPresent) {
      setError('Missing token. Open this page as /chat#YOUR_MAXX_KEY.');
      return;
    }
    if (!selectedModel) {
      setError('Choose or enter a model name for this route type.');
      return;
    }

    const assistantMessageID = buildMessageID('assistant');
    const nextMessages: ChatMessage[] = [
      ...messages,
      { id: buildMessageID('user'), role: 'user', content },
      { id: assistantMessageID, role: 'assistant', content: '' },
    ];
    setMessages(nextMessages);
    if (inputRef.current) inputRef.current.value = '';
    setHasInput(false);
    setError('');
    setIsSending(true);

    try {
      const requestMessages = nextMessages.filter(
        (message) =>
          message.id !== 'welcome' && message.id !== assistantMessageID && !message.transient,
      );
      const response = await fetch(endpoint, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          Authorization: `Bearer ${token}`,
        },
        body: JSON.stringify(
          buildChatRequestBody(routeType, selectedModel, requestMessages, { stream: true }),
        ),
      });

      if (!response.ok) {
        const payload = await response.json().catch(() => ({}));
        const message =
          typeof payload === 'object' && payload && 'error' in payload
            ? JSON.stringify(payload.error)
            : `Request failed with ${response.status}`;
        throw new Error(message);
      }

      const contentType = response.headers.get('content-type') ?? '';
      if (!response.body || !contentType.includes('text/event-stream')) {
        const payload = await response.json().catch(() => ({}));
        const reply = extractChatResponseText(routeType, payload).trim() || 'No text response.';
        updateAssistantMessage(assistantMessageID, reply);
        return;
      }

      const reader = response.body.getReader();
      const decoder = new TextDecoder();
      let buffer = '';
      let streamedReply = '';

      const consumeSSEChunk = (chunk: string) => {
        buffer += chunk;
        const events = buffer.split(/\r?\n\r?\n/);
        buffer = events.pop() ?? '';
        for (const event of events) {
          const dataLines = event
            .split(/\r?\n/)
            .filter((line) => line.startsWith('data:'))
            .map((line) => line.slice(5).trimStart());
          if (dataLines.length === 0) continue;
          const data = dataLines.join('\n').trim();
          if (!data || data === '[DONE]') continue;
          try {
            const delta = extractChatStreamDelta(routeType, JSON.parse(data));
            if (!delta) continue;
            streamedReply += delta;
            updateAssistantMessage(assistantMessageID, streamedReply);
          } catch {
            // Ignore malformed heartbeat/metadata SSE frames.
          }
        }
      };

      while (true) {
        const { value, done } = await reader.read();
        if (done) break;
        consumeSSEChunk(decoder.decode(value, { stream: true }));
      }
      consumeSSEChunk(decoder.decode());
      if (buffer.trim())
        consumeSSEChunk(`${buffer}

`);
      if (!streamedReply.trim()) updateAssistantMessage(assistantMessageID, 'No text response.');
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Request failed.');
      updateAssistantMessage(
        assistantMessageID,
        'The request failed. Check the selected route type, model, and token, then try again.',
        { transient: true },
      );
    } finally {
      setIsSending(false);
    }
  };

  return (
    <main className="h-svh overflow-hidden bg-[#f7f7f5] text-[#111827] dark:bg-[#111111] dark:text-[#f5f5f0]">
      <div className="mx-auto flex h-full w-full max-w-none flex-col px-2 py-3 sm:px-3 lg:px-4">
        <header className="flex flex-col gap-3 border-b border-black/10 pb-4 dark:border-white/10 md:flex-row md:items-center md:justify-between">
          <div>
            <div className="flex items-center gap-2 text-xs font-medium uppercase tracking-[0.22em] text-muted-foreground">
              <Route className="size-3.5" /> Maxx Chat
            </div>
            <h1 className="mt-2 text-2xl font-semibold tracking-tight">Chat by route type</h1>
          </div>
          <div className="flex flex-wrap items-center gap-2">
            <Badge variant={tokenPresent ? 'success' : 'warning'} className="gap-1.5">
              <KeyRound className="size-3" />{' '}
              {tokenPresent ? 'Token loaded from hash' : 'Missing hash token'}
            </Badge>
          </div>
        </header>

        <section className="grid min-h-0 flex-1 gap-3 overflow-hidden py-3 lg:grid-cols-[240px_minmax(0,1fr)]">
          <aside className="overflow-y-auto rounded-3xl border border-black/10 bg-white/80 p-3 shadow-sm backdrop-blur dark:border-white/10 dark:bg-white/5">
            <p className="px-2 pb-2 text-xs font-medium uppercase tracking-wide text-muted-foreground">
              Route type
            </p>
            <div className="grid gap-2">
              {(visibleRouteConfigs.length > 0 ? visibleRouteConfigs : CHAT_ROUTE_CONFIGS).map(
                (config) => {
                  const disabled = visibleRouteConfigs.length === 0;
                  return (
                    <Button
                      key={config.type}
                      type="button"
                      variant={routeType === config.type ? 'default' : 'ghost'}
                      className="h-11 justify-start rounded-2xl"
                      disabled={disabled}
                      onClick={() => setRouteType(config.type)}
                    >
                      {config.label}
                    </Button>
                  );
                },
              )}
            </div>

            {visibleRouteConfigs.length === 0 && tokenPresent ? (
              <div className="mt-4 rounded-2xl border border-dashed border-border p-3 text-xs text-muted-foreground">
                No route type is exposed by the user-panel model list yet.
              </div>
            ) : null}

            <div className="mt-4 rounded-2xl bg-muted/60 p-3 text-xs text-muted-foreground">
              <label className="font-medium text-foreground" htmlFor="chat-model-select">
                Model name
              </label>
              {models.length > 0 ? (
                <select
                  id="chat-model-select"
                  value={model}
                  onChange={(event) => setModel(event.target.value)}
                  className="mt-2 h-10 w-full rounded-xl border border-border bg-background px-3 text-sm text-foreground"
                >
                  {models.map((modelName) => (
                    <option key={modelName} value={modelName}>
                      {modelName}
                    </option>
                  ))}
                </select>
              ) : (
                <Input
                  id="chat-model-select"
                  value={model}
                  onChange={(event) => setModel(event.target.value)}
                  className="mt-2 h-10 rounded-xl"
                  placeholder={routeConfig.defaultModel}
                />
              )}
              <p className="mt-2">
                {isLoadingModels
                  ? 'Loading models for this route type...'
                  : models.length > 0
                    ? `${models.length} models loaded for ${routeConfig.label}.`
                    : modelsError || 'User-panel has not exposed models for this route type.'}
              </p>
            </div>

            <div className="mt-4 rounded-2xl bg-muted/60 p-3 text-xs text-muted-foreground">
              <p className="font-medium text-foreground">Current endpoint</p>
              <p className="mt-1 break-all font-mono">{endpoint}</p>
            </div>
          </aside>

          <section className="flex min-h-0 min-w-0 flex-col rounded-3xl border border-black/10 bg-white shadow-sm dark:border-white/10 dark:bg-[#181818]">
            <div className="flex items-center justify-between border-b border-black/10 px-4 py-3 dark:border-white/10">
              <div>
                <p className="text-sm font-medium">{routeConfig.label}</p>
                <p className="text-xs text-muted-foreground">Model: {selectedModel}</p>
              </div>
              <Badge variant="outline" className="font-mono">
                {routeConfig.type}
              </Badge>
            </div>

            <div
              ref={transcriptRef}
              className="min-h-0 flex-1 space-y-4 overflow-y-auto px-3 py-4 sm:px-4"
            >
              {messages.map((message) => (
                <div
                  key={message.id}
                  className={cn('flex min-w-0 gap-3', message.role === 'user' && 'justify-end')}
                >
                  {message.role === 'assistant' ? (
                    <div className="flex size-8 shrink-0 items-center justify-center rounded-full bg-black text-white dark:bg-white dark:text-black">
                      <Bot className="size-4" />
                    </div>
                  ) : null}
                  <div
                    className={cn(
                      'max-w-[min(980px,92%)] overflow-hidden break-words rounded-3xl px-4 py-3 text-sm leading-6 [overflow-wrap:anywhere] whitespace-pre-wrap',
                      message.role === 'user'
                        ? 'bg-black text-white dark:bg-white dark:text-black'
                        : 'bg-muted text-foreground',
                    )}
                  >
                    {message.content ||
                      (message.role === 'assistant' && isSending ? 'Streaming…' : '')}
                  </div>
                  {message.role === 'user' ? (
                    <div className="flex size-8 shrink-0 items-center justify-center rounded-full bg-muted text-muted-foreground">
                      <User className="size-4" />
                    </div>
                  ) : null}
                </div>
              ))}
              {isSending ? (
                <div className="flex items-center gap-2 text-sm text-muted-foreground">
                  <Loader2 className="size-4 animate-spin" /> Waiting for {routeConfig.label}{' '}
                  route...
                </div>
              ) : null}
            </div>

            {error ? (
              <div className="mx-4 mb-3 rounded-2xl border border-destructive/30 bg-destructive/10 px-4 py-2 text-sm text-destructive">
                {error}
              </div>
            ) : null}

            <div className="shrink-0 border-t border-black/10 p-4 dark:border-white/10">
              <div className="flex min-w-0 items-end gap-2 rounded-3xl border border-black/10 bg-[#f7f7f5] p-2 dark:border-white/10 dark:bg-black/20">
                <Textarea
                  ref={inputRef}
                  onInput={(event) => setHasInput(event.currentTarget.value.trim().length > 0)}
                  onKeyDown={(event) => {
                    if (
                      event.key === 'Enter' &&
                      !event.shiftKey &&
                      !event.nativeEvent.isComposing
                    ) {
                      event.preventDefault();
                      void handleSend();
                    }
                  }}
                  placeholder="Message Maxx..."
                  className="max-h-52 min-h-12 resize-none overflow-y-auto border-0 bg-transparent shadow-none focus-visible:ring-0"
                />
                <Button
                  type="button"
                  size="icon"
                  className="size-10 shrink-0 rounded-full"
                  disabled={!hasInput || isSending}
                  onClick={handleSend}
                  aria-label="Send message"
                >
                  {isSending ? (
                    <Loader2 className="size-4 animate-spin" />
                  ) : (
                    <ArrowUp className="size-4" />
                  )}
                </Button>
              </div>
            </div>
          </section>
        </section>
      </div>
    </main>
  );
}
