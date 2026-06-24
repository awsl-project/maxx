import type { ClientType, CreateProviderData } from '@/lib/transport';

export type BulkCustomProviderCommand = {
  lineNumber: number;
  name: string;
  baseURL: string;
  apiKey: string;
  clients: ClientType[];
  supportModels: string[];
  modelMapping: Record<string, string>;
  responseModelMapping: Record<string, string>;
  backend?: 'ollama';
  logo?: string;
  disableErrorCooldown: boolean;
  excludeFromExport: boolean;
  responsesPassthrough?: boolean;
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
  'map',
  'response-map',
  'backend',
  'logo',
]);
const BOOLEAN_FLAGS = new Set([
  'disable-error-cooldown',
  'exclude-from-export',
  'responses-passthrough',
  'no-responses-passthrough',
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
    modelMapping: {} as Record<string, string>,
    responseModelMapping: {} as Record<string, string>,
    backend: undefined as 'ollama' | undefined,
    logo: undefined as string | undefined,
    disableErrorCooldown: false,
    excludeFromExport: false,
    responsesPassthrough: undefined as boolean | undefined,
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
      if (flag === 'disable-error-cooldown') parsed.disableErrorCooldown = true;
      if (flag === 'exclude-from-export') parsed.excludeFromExport = true;
      if (flag === 'responses-passthrough') parsed.responsesPassthrough = true;
      if (flag === 'no-responses-passthrough') parsed.responsesPassthrough = false;
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
    config: {
      disableErrorCooldown: command.disableErrorCooldown,
      custom: {
        baseURL: command.baseURL,
        backend: command.backend,
        apiKey: command.apiKey,
        modelMapping:
          Object.keys(command.modelMapping).length > 0 ? command.modelMapping : undefined,
        responseModelMapping:
          Object.keys(command.responseModelMapping).length > 0
            ? command.responseModelMapping
            : undefined,
        responsesPassthrough: command.responsesPassthrough,
      },
    },
    supportedClientTypes: command.clients,
    supportModels: command.supportModels,
    excludeFromExport: command.excludeFromExport,
  };
}
