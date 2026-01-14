import type { ClientType } from '@/lib/transport';
import { getProviderColorVar } from '@/lib/theme';
import duckcodingLogo from '@/assets/icons/duckcoding.gif';

// Antigravity brand color - 从 CSS 变量获取
export const ANTIGRAVITY_COLOR = getProviderColorVar('antigravity');

// Quick templates for Custom provider
export type QuickTemplate = {
  id: string;
  name: string;
  description: string;
  icon: 'grid' | 'layers';
  logoUrl?: string; // 可选的 logo 图片 URL
  supportedClients: ClientType[];
  clientBaseURLs: Partial<Record<ClientType, string>>;
};

export const quickTemplates: QuickTemplate[] = [
  {
    id: '88code',
    name: '88 Code',
    description: 'Claude + Codex + Gemini',
    icon: 'grid',
    supportedClients: ['claude', 'codex', 'gemini'],
    clientBaseURLs: {
      claude: 'https://www.88code.ai/api',
      codex: 'https://88code.ai/openai/v1',
      gemini: 'https://www.88code.ai/gemini',
    },
  },
  {
    id: 'aicodemirror',
    name: 'AI Code Mirror',
    description: 'Claude + Codex + Gemini',
    icon: 'layers',
    supportedClients: ['claude', 'codex', 'gemini'],
    clientBaseURLs: {
      claude: 'https://api.aicodemirror.com/api/claudecode',
      codex: 'https://api.aicodemirror.com/api/codex/backend-api/codex',
      gemini: 'https://api.aicodemirror.com/api/gemini',
    },
  },
  {
    id: 'duckcoding',
    name: 'DuckCoding',
    description: 'Claude + Codex + Gemini',
    icon: 'grid',
    logoUrl: duckcodingLogo,
    supportedClients: ['claude', 'codex', 'gemini'],
    clientBaseURLs: {
      claude: 'https://jp.duckcoding.com',
      codex: 'https://jp.duckcoding.com/v1',
      gemini: 'https://jp.duckcoding.com',
    },
  },
];

// Client config
export type ClientConfig = {
  id: ClientType;
  name: string;
  enabled: boolean;
  urlOverride: string;
};

export const defaultClients: ClientConfig[] = [
  { id: 'claude', name: 'Claude', enabled: true, urlOverride: '' },
  { id: 'codex', name: 'Codex', enabled: false, urlOverride: '' },
  { id: 'gemini', name: 'Gemini', enabled: false, urlOverride: '' },
];

// Form data types
export type ProviderFormData = {
  type: 'custom' | 'antigravity';
  name: string;
  selectedTemplate: string | null;
  baseURL: string;
  apiKey: string;
  clients: ClientConfig[];
};

// Create step type
export type CreateStep = 'select-type' | 'custom-config' | 'antigravity-import';
