import type { ClientType, Provider } from '@/lib/transport';
import { getProviderColorVar } from '@/lib/theme';
import type { LucideIcon } from 'lucide-react';
import { Wand2, Zap, Server, Mail, Globe } from 'lucide-react';
import duckcodingLogo from '@/assets/icons/duckcoding.gif';
import freeDuckLogo from '@/assets/icons/free-duck.gif';

// ===== Provider Type Configuration =====
// 通用的 Provider 类型配置，添加新类型只需在这里配置

export type ProviderTypeKey = 'custom' | 'antigravity' | 'kiro';

export interface ProviderTypeConfig {
  key: ProviderTypeKey;
  label: string;
  icon: LucideIcon;
  color: string;
  // 是否使用邮箱作为显示信息（账号类型）
  isAccountBased: boolean;
  // 获取显示信息的函数
  getDisplayInfo: (provider: Provider) => string;
}

// Provider 类型配置表
export const PROVIDER_TYPE_CONFIGS: Record<ProviderTypeKey, ProviderTypeConfig> = {
  antigravity: {
    key: 'antigravity',
    label: 'Antigravity',
    icon: Wand2,
    color: getProviderColorVar('antigravity'),
    isAccountBased: true,
    getDisplayInfo: (p) => p.config?.antigravity?.email || 'Unknown',
  },
  kiro: {
    key: 'kiro',
    label: 'Kiro',
    icon: Zap,
    color: getProviderColorVar('kiro'),
    isAccountBased: true,
    getDisplayInfo: (p) => p.config?.kiro?.email || 'Kiro Account',
  },
  custom: {
    key: 'custom',
    label: 'Custom',
    icon: Server,
    color: getProviderColorVar('custom'),
    isAccountBased: false,
    getDisplayInfo: (p) => {
      if (p.config?.custom?.baseURL) return p.config.custom.baseURL;
      for (const ct of p.supportedClientTypes || []) {
        const url = p.config?.custom?.clientBaseURL?.[ct];
        if (url) return url;
      }
      return 'Not configured';
    },
  },
};

// 获取 Provider 类型配置的辅助函数
export function getProviderTypeConfig(type: string): ProviderTypeConfig {
  return PROVIDER_TYPE_CONFIGS[type as ProviderTypeKey] || PROVIDER_TYPE_CONFIGS.custom;
}

// 获取显示图标（邮箱或 URL）
export function getDisplayIcon(type: string): LucideIcon {
  const config = getProviderTypeConfig(type);
  return config.isAccountBased ? Mail : Globe;
}

// 保留旧的导出以保持兼容性
export const ANTIGRAVITY_COLOR = PROVIDER_TYPE_CONFIGS.antigravity.color;
export const KIRO_COLOR = PROVIDER_TYPE_CONFIGS.kiro.color;

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
  {
    id: 'freeduck',
    name: 'Free Duck',
    description: '免费站点 · 只有 Claude Code',
    icon: 'grid',
    logoUrl: freeDuckLogo,
    supportedClients: ['claude'],
    clientBaseURLs: {
      claude: 'https://free.duckcoding.com',
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
  type: 'custom' | 'antigravity' | 'kiro';
  name: string;
  selectedTemplate: string | null;
  baseURL: string;
  apiKey: string;
  clients: ClientConfig[];
};

// Create step type
export type CreateStep = 'select-type' | 'custom-config' | 'antigravity-import' | 'kiro-import';
