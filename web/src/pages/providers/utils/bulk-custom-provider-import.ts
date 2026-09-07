import type {
  ClientType,
  CreateProviderData,
  ProviderConfigCustomDisguise,
  ReasoningPolicy,
} from '@/lib/transport';

export type BulkCustomProviderCommand = {
  lineNumber: number;
  name: string;
  baseURL: string;
  apiKey: string;
  clients: ClientType[];
  supportModels: string[];
  exposedModelsEnabled: boolean;
  exposedModels: string[];
  modelMapping: Record<string, string>;
  responseModelMapping: Record<string, string>;
  backend?: 'ollama';
  logo?: string;
  clientBaseURL?: Partial<Record<ClientType, string>>;
  clientMultiplier?: Partial<Record<ClientType, number>>;
  disguise?: ProviderConfigCustomDisguise;
  reasoning?: ReasoningPolicy;
  quotaEnabled: boolean;
  disableErrorCooldown: boolean;
  consecutiveErrorFreezeEnabled: boolean;
  consecutiveErrorFreezeThreshold?: number;
  smartMappingRetryEnabled: boolean;
  smartMappingRetryLimit?: number;
  maxConcurrency: number;
  excludeFromExport: boolean;
  responsesPassthrough?: boolean;
  responsesWebSocket: boolean;
};

export type BulkCustomProviderParseError = {
  lineNumber: number;
  message: string;
};

export type BulkCustomProviderParseResult = {
  commands: BulkCustomProviderCommand[];
  errors: BulkCustomProviderParseError[];
};

const CLIENT_TYPES = new Set<ClientType>(['claude', 'codex', 'gemini', 'openai']);
const VALUE_FLAGS = new Set([
  'name',
  'base-url',
  'api-key',
  'clients',
  'models',
  'exposed-models',
  'map',
  'response-map',
  'backend',
  'logo',
  'client-base-url',
  'client-multiplier',
  'disguise',
  'reasoning',
  'consecutive-error-freeze-threshold',
  'smart-mapping-retry-limit',
  'max-concurrency',
]);
const BOOLEAN_FLAGS = new Set([
  'quota-enabled',
  'disable-error-cooldown',
  'consecutive-error-freeze',
  'smart-mapping-retry',
  'exclude-from-export',
  'enable-exposed-models',
  'responses-passthrough',
  'no-responses-passthrough',
  'responses-websocket',
]);

function splitCommandLines(input: string): Array<{ lineNumber: number; text: string }> {
  return input
    .split(/\r?\n/)
    .map((text, index) => ({ lineNumber: index + 1, text: text.trim() }))
    .filter(({ text }) => text.length > 0 && !text.startsWith('#'));
}

export function tokenizeProviderCommand(input: string): string[] {
  const tokens: string[] = [];
  let current = '';
  let quote: '"' | "'" | null = null;
  let escaping = false;

  for (const char of input) {
    if (escaping) {
      current += char;
      escaping = false;
      continue;
    }

    if (char === '\\') {
      escaping = true;
      continue;
    }

    if (quote) {
      if (char === quote) {
        quote = null;
      } else {
        current += char;
      }
      continue;
    }

    if (char === '"' || char === "'") {
      quote = char;
      continue;
    }

    if (/\s/.test(char)) {
      if (current) {
        tokens.push(current);
        current = '';
      }
      continue;
    }

    current += char;
  }

  if (escaping) current += '\\';
  if (current) tokens.push(current);
  if (quote) {
    throw new Error(`Unclosed ${quote} quote`);
  }

  return tokens;
}

function splitList(value: string): string[] {
  return value
    .split(',')
    .map((item) => item.trim())
    .filter(Boolean);
}

function parseClients(value: string): { clients: ClientType[]; errors: string[] } {
  const errors: string[] = [];
  const clients: ClientType[] = [];
  const seen = new Set<ClientType>();

  for (const rawClient of splitList(value)) {
    const client = rawClient.toLowerCase() as ClientType;
    if (!CLIENT_TYPES.has(client)) {
      errors.push(`Unsupported client "${rawClient}"`);
      continue;
    }
    if (!seen.has(client)) {
      clients.push(client);
      seen.add(client);
    }
  }

  return { clients, errors };
}

function parseMappingList(value: string): { mapping: Record<string, string>; errors: string[] } {
  const mapping: Record<string, string> = {};
  const errors: string[] = [];

  for (const rawEntry of splitList(value)) {
    const match = rawEntry.match(/^(.*?)\s*(?:=|->)\s*(.*?)$/);
    if (!match) {
      errors.push(`Invalid mapping "${rawEntry}". Use source=target or source->target`);
      continue;
    }

    const source = match[1]?.trim();
    const target = match[2]?.trim();
    if (!source || !target) {
      errors.push(`Invalid mapping "${rawEntry}". Source and target are required`);
      continue;
    }

    mapping[source] = target;
  }

  return { mapping, errors };
}

function setMapValues(target: Record<string, string>, source: Record<string, string>) {
  for (const [key, value] of Object.entries(source)) {
    target[key] = value;
  }
}

function parseJSONFlag<T>(value: string, label: string): { value?: T; error?: string } {
  try {
    return { value: JSON.parse(value) as T };
  } catch {
    return { error: `${label} must be valid JSON` };
  }
}

function isPlainObject(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value);
}

function parseClientBaseURL(value: string): {
  value?: Partial<Record<ClientType, string>>;
  errors: string[];
} {
  const parsed = parseJSONFlag<Record<string, unknown>>(value, 'Client base URL');
  if (parsed.error) return { errors: [parsed.error] };
  if (!isPlainObject(parsed.value)) return { errors: ['Client base URL must be a JSON object'] };

  const result: Partial<Record<ClientType, string>> = {};
  const errors: string[] = [];
  for (const [rawClient, rawURL] of Object.entries(parsed.value)) {
    const client = rawClient.toLowerCase() as ClientType;
    if (!CLIENT_TYPES.has(client)) {
      errors.push(`Unsupported client "${rawClient}"`);
      continue;
    }
    if (typeof rawURL !== 'string' || !rawURL.trim()) {
      errors.push(`Client base URL for "${rawClient}" must be a non-empty string`);
      continue;
    }
    result[client] = rawURL.trim();
  }

  return { value: result, errors };
}

function parseClientMultiplier(value: string): {
  value?: Partial<Record<ClientType, number>>;
  errors: string[];
} {
  const parsed = parseJSONFlag<Record<string, unknown>>(value, 'Client multiplier');
  if (parsed.error) return { errors: [parsed.error] };
  if (!isPlainObject(parsed.value)) return { errors: ['Client multiplier must be a JSON object'] };

  const result: Partial<Record<ClientType, number>> = {};
  const errors: string[] = [];
  for (const [rawClient, rawMultiplier] of Object.entries(parsed.value)) {
    const client = rawClient.toLowerCase() as ClientType;
    if (!CLIENT_TYPES.has(client)) {
      errors.push(`Unsupported client "${rawClient}"`);
      continue;
    }
    if (
      typeof rawMultiplier !== 'number' ||
      !Number.isSafeInteger(rawMultiplier) ||
      rawMultiplier < 0
    ) {
      errors.push(`Client multiplier for "${rawClient}" must be 0 or greater`);
      continue;
    }
    result[client] = rawMultiplier;
  }

  return { value: result, errors };
}

function parseLine(lineNumber: number, text: string): BulkCustomProviderParseResult {
  const errors: BulkCustomProviderParseError[] = [];
  let tokens: string[];

  try {
    tokens = tokenizeProviderCommand(text);
  } catch (error) {
    return {
      commands: [],
      errors: [{ lineNumber, message: error instanceof Error ? error.message : String(error) }],
    };
  }

  if (tokens[0] === 'provider' && tokens[1] === 'add') {
    tokens = tokens.slice(2);
  } else if (tokens[0] === 'add') {
    tokens = tokens.slice(1);
  } else {
    errors.push({ lineNumber, message: 'Command must start with "provider add" or "add"' });
  }

  const parsed = {
    name: '',
    baseURL: '',
    apiKey: '',
    clients: [] as ClientType[],
    supportModels: [] as string[],
    exposedModelsEnabled: false,
    exposedModels: [] as string[],
    modelMapping: {} as Record<string, string>,
    responseModelMapping: {} as Record<string, string>,
    backend: undefined as 'ollama' | undefined,
    logo: undefined as string | undefined,
    clientBaseURL: undefined as Partial<Record<ClientType, string>> | undefined,
    clientMultiplier: undefined as Partial<Record<ClientType, number>> | undefined,
    disguise: undefined as ProviderConfigCustomDisguise | undefined,
    reasoning: undefined as ReasoningPolicy | undefined,
    quotaEnabled: false,
    disableErrorCooldown: false,
    consecutiveErrorFreezeEnabled: false,
    consecutiveErrorFreezeThreshold: undefined as number | undefined,
    smartMappingRetryEnabled: false,
    smartMappingRetryLimit: undefined as number | undefined,
    maxConcurrency: 0,
    excludeFromExport: false,
    responsesPassthrough: undefined as boolean | undefined,
    responsesWebSocket: false,
  };

  for (let index = 0; index < tokens.length; index += 1) {
    const token = tokens[index];
    if (!token.startsWith('--')) {
      errors.push({ lineNumber, message: `Unexpected token "${token}"` });
      continue;
    }

    const rawFlag = token.slice(2);
    const [flag, inlineValue] = rawFlag.split(/=(.*)/s, 2);

    if (BOOLEAN_FLAGS.has(flag)) {
      if (inlineValue !== undefined) {
        errors.push({ lineNumber, message: `Flag --${flag} does not accept a value` });
        continue;
      }
      if (flag === 'quota-enabled') parsed.quotaEnabled = true;
      if (flag === 'disable-error-cooldown') parsed.disableErrorCooldown = true;
      if (flag === 'consecutive-error-freeze') parsed.consecutiveErrorFreezeEnabled = true;
      if (flag === 'smart-mapping-retry') parsed.smartMappingRetryEnabled = true;
      if (flag === 'exclude-from-export') parsed.excludeFromExport = true;
      if (flag === 'enable-exposed-models') parsed.exposedModelsEnabled = true;
      if (flag === 'responses-passthrough') parsed.responsesPassthrough = true;
      if (flag === 'no-responses-passthrough') parsed.responsesPassthrough = false;
      if (flag === 'responses-websocket') parsed.responsesWebSocket = true;
      continue;
    }

    if (!VALUE_FLAGS.has(flag)) {
      errors.push({ lineNumber, message: `Unknown flag --${flag}` });
      continue;
    }

    const value = inlineValue ?? tokens[index + 1];
    if (value === undefined || (!inlineValue && value.startsWith('--'))) {
      errors.push({ lineNumber, message: `Missing value for --${flag}` });
      continue;
    }
    if (inlineValue === undefined) index += 1;

    switch (flag) {
      case 'name':
        parsed.name = value.trim();
        break;
      case 'base-url':
        parsed.baseURL = value.trim();
        break;
      case 'api-key':
        parsed.apiKey = value.trim();
        break;
      case 'clients': {
        const result = parseClients(value);
        parsed.clients = result.clients;
        result.errors.forEach((message) => errors.push({ lineNumber, message }));
        break;
      }
      case 'models':
        parsed.supportModels = splitList(value);
        break;
      case 'exposed-models':
        parsed.exposedModels = splitList(value);
        parsed.exposedModelsEnabled = true;
        break;
      case 'map': {
        const result = parseMappingList(value);
        setMapValues(parsed.modelMapping, result.mapping);
        result.errors.forEach((message) => errors.push({ lineNumber, message }));
        break;
      }
      case 'response-map': {
        const result = parseMappingList(value);
        setMapValues(parsed.responseModelMapping, result.mapping);
        result.errors.forEach((message) => errors.push({ lineNumber, message }));
        break;
      }
      case 'backend':
        if (value !== 'http' && value !== 'ollama') {
          errors.push({ lineNumber, message: 'Backend must be "http" or "ollama"' });
        }
        parsed.backend = value === 'ollama' ? 'ollama' : undefined;
        break;
      case 'logo':
        parsed.logo = value.trim();
        break;
      case 'client-base-url': {
        const result = parseClientBaseURL(value);
        parsed.clientBaseURL = result.value;
        result.errors.forEach((message) => errors.push({ lineNumber, message }));
        break;
      }
      case 'client-multiplier': {
        const result = parseClientMultiplier(value);
        parsed.clientMultiplier = result.value;
        result.errors.forEach((message) => errors.push({ lineNumber, message }));
        break;
      }
      case 'disguise': {
        const result = parseJSONFlag<ProviderConfigCustomDisguise>(value, 'Disguise');
        if (result.error) {
          errors.push({ lineNumber, message: result.error });
        } else {
          parsed.disguise = result.value;
        }
        break;
      }
      case 'reasoning': {
        const result = parseJSONFlag<ReasoningPolicy>(value, 'Reasoning');
        if (result.error) {
          errors.push({ lineNumber, message: result.error });
        } else {
          parsed.reasoning = result.value;
        }
        break;
      }
      case 'consecutive-error-freeze-threshold': {
        const threshold = Number.parseInt(value, 10);
        if (!Number.isFinite(threshold) || threshold < 1 || threshold > 100) {
          errors.push({
            lineNumber,
            message: 'Consecutive error freeze threshold must be between 1 and 100',
          });
        } else {
          parsed.consecutiveErrorFreezeThreshold = threshold;
        }
        break;
      }
      case 'smart-mapping-retry-limit': {
        const parsedLimit = Number.parseInt(value, 10);
        if (!Number.isFinite(parsedLimit) || parsedLimit < 1 || parsedLimit > 20) {
          errors.push({
            lineNumber,
            message: 'Smart mapping retry limit must be between 1 and 20',
          });
        } else {
          parsed.smartMappingRetryLimit = parsedLimit;
        }
        break;
      }
      case 'max-concurrency': {
        const maxConcurrency = Number(value);
        if (!/^\d+$/.test(value) || !Number.isSafeInteger(maxConcurrency)) {
          errors.push({ lineNumber, message: 'Max concurrency must be 0 or greater' });
        } else {
          parsed.maxConcurrency = maxConcurrency;
        }
        break;
      }
    }
  }

  if (!parsed.name) errors.push({ lineNumber, message: 'Provider name is required' });
  if (!parsed.baseURL) errors.push({ lineNumber, message: 'Base URL is required' });
  if (parsed.backend !== 'ollama' && !parsed.apiKey) {
    errors.push({ lineNumber, message: 'API key is required for http custom providers' });
  }
  if (parsed.clients.length === 0) {
    errors.push({ lineNumber, message: 'At least one client is required' });
  }
  if (parsed.smartMappingRetryEnabled && !parsed.disableErrorCooldown) {
    errors.push({
      lineNumber,
      message: 'Smart mapping retry requires --disable-error-cooldown',
    });
  }
  if (parsed.consecutiveErrorFreezeEnabled && !parsed.disableErrorCooldown) {
    errors.push({
      lineNumber,
      message: 'Consecutive error freeze requires --disable-error-cooldown',
    });
  }

  if (errors.length > 0) {
    return { commands: [], errors };
  }

  return {
    commands: [{ lineNumber, ...parsed }],
    errors: [],
  };
}

export function parseBulkCustomProviderCommands(input: string): BulkCustomProviderParseResult {
  const commands: BulkCustomProviderCommand[] = [];
  const errors: BulkCustomProviderParseError[] = [];

  for (const line of splitCommandLines(input)) {
    const result = parseLine(line.lineNumber, line.text);
    commands.push(...result.commands);
    errors.push(...result.errors);
  }

  return { commands, errors };
}

export function toCreateProviderData(command: BulkCustomProviderCommand): CreateProviderData {
  return {
    type: 'custom',
    name: command.name,
    logo: command.logo,
    maxConcurrency: command.maxConcurrency,
    config: {
      quotaEnabled: command.quotaEnabled,
      disableErrorCooldown: command.disableErrorCooldown,
      consecutiveErrorFreezeEnabled:
        command.disableErrorCooldown && command.consecutiveErrorFreezeEnabled,
      consecutiveErrorFreezeThreshold: command.consecutiveErrorFreezeThreshold,
      smartMappingRetryEnabled: command.disableErrorCooldown && command.smartMappingRetryEnabled,
      smartMappingRetryLimit: command.smartMappingRetryLimit ?? 1,
      reasoning: command.reasoning,
      custom: {
        baseURL: command.baseURL,
        backend: command.backend,
        apiKey: command.apiKey,
        clientBaseURL: command.clientBaseURL,
        clientMultiplier: command.clientMultiplier,
        disguise: command.disguise,
        modelMapping:
          Object.keys(command.modelMapping).length > 0 ? command.modelMapping : undefined,
        responseModelMapping:
          Object.keys(command.responseModelMapping).length > 0
            ? command.responseModelMapping
            : undefined,
        responsesPassthrough: command.responsesPassthrough,
        responsesWebSocket: command.responsesWebSocket,
      },
    },
    supportedClientTypes: command.clients,
    supportModels: command.supportModels,
    exposedModelsEnabled: command.exposedModelsEnabled,
    exposedModels: command.exposedModels,
    excludeFromExport: command.excludeFromExport,
  };
}
