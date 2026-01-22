import type { ClientType, Provider } from '@/lib/transport';
import { getProviderColorVar } from '@/lib/theme';
import type { LucideIcon } from 'lucide-react';
import { Wand2, Zap, Server, Mail, Globe } from 'lucide-react';
import duckcodingLogo from '@/assets/icons/duckcoding.gif';
import freeDuckLogo from '@/assets/icons/free-duck.gif';
import nvidiaLogo from '@/assets/icons/nvidia.svg';
import logo88code from '@/assets/icons/88code.svg';
import aicodemirrorLogo from '@/assets/icons/aicodemirror.png';
import zhipuLogo from '@/assets/icons/zhipu.svg';

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
  // 是否在创建流程中隐藏
  hidden?: boolean;
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
    hidden: true,
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

// Model mapping for templates
export type TemplateModelMapping = {
  pattern: string; // e.g., '*claude*', '*sonnet*'
  target: string; // e.g., 'meta/llama-3.1-70b-instruct'
};

// Quick templates for Custom provider
export type QuickTemplate = {
  id: string;
  name: string;
  description: string;
  nameKey?: string; // i18n translation key for name
  descriptionKey?: string; // i18n translation key for description
  icon: 'grid' | 'layers';
  logoUrl?: string; // 可选的 logo 图片 URL
  supportedClients: ClientType[];
  clientBaseURLs: Partial<Record<ClientType, string>>;
  modelMappings?: TemplateModelMapping[]; // 可选的模型映射
};

export const quickTemplates: QuickTemplate[] = [
  {
    id: '88code',
    name: '88 Code',
    description: 'Claude + Codex + Gemini',
    nameKey: 'addProvider.templates.88code.name',
    descriptionKey: 'addProvider.templates.88code.description',
    icon: 'grid',
    logoUrl: logo88code,
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
    nameKey: 'addProvider.templates.aicodemirror.name',
    descriptionKey: 'addProvider.templates.aicodemirror.description',
    icon: 'layers',
    logoUrl: aicodemirrorLogo,
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
    nameKey: 'addProvider.templates.duckcoding.name',
    descriptionKey: 'addProvider.templates.duckcoding.description',
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
    nameKey: 'addProvider.templates.freeduck.name',
    descriptionKey: 'addProvider.templates.freeduck.description',
    icon: 'grid',
    logoUrl: freeDuckLogo,
    supportedClients: ['claude'],
    clientBaseURLs: {
      claude: 'https://free.duckcoding.com',
    },
  },
  {
    id: 'nvidia',
    name: 'NVIDIA',
    description: 'NVIDIA NIM · OpenAI 兼容',
    nameKey: 'addProvider.templates.nvidia.name',
    descriptionKey: 'addProvider.templates.nvidia.description',
    icon: 'layers',
    logoUrl: nvidiaLogo,
    supportedClients: ['openai'],
    clientBaseURLs: {
      openai: 'https://integrate.api.nvidia.com',
    },
    modelMappings: [{ pattern: '*', target: 'minimaxai/minimax-m2.1' }],
  },
  {
    id: 'zhipu',
    name: '智谱 AI',
    description: 'Claude Code · GLM-4.7',
    nameKey: 'addProvider.templates.zhipu.name',
    descriptionKey: 'addProvider.templates.zhipu.description',
    icon: 'grid',
    logoUrl: zhipuLogo,
    supportedClients: ['claude'],
    clientBaseURLs: {
      claude: 'https://open.bigmodel.cn/api/anthropic',
    },
  },
];

// Client config
export type ClientConfig = {
  id: ClientType;
  name: string;
  enabled: boolean;
  urlOverride: string;
  multiplier: number; // 10000=1倍
};

export const defaultClients: ClientConfig[] = [
  { id: 'claude', name: 'Claude', enabled: true, urlOverride: '', multiplier: 10000 },
  { id: 'openai', name: 'OpenAI', enabled: false, urlOverride: '', multiplier: 10000 },
  { id: 'codex', name: 'Codex', enabled: false, urlOverride: '', multiplier: 10000 },
  { id: 'gemini', name: 'Gemini', enabled: false, urlOverride: '', multiplier: 10000 },
];

// Form data types
export type ProviderFormData = {
  type: 'custom' | 'antigravity' | 'kiro';
  name: string;
  selectedTemplate: string | null;
  baseURL: string;
  apiKey: string;
  clients: ClientConfig[];
  modelMappings?: TemplateModelMapping[]; // 模型映射
  logo?: string; // Logo URL
};

// Create step type
export type CreateStep = 'select-type' | 'custom-config' | 'antigravity-import' | 'kiro-import';
