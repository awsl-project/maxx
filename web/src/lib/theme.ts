/**
 * 主题配置
 * 统一的颜色管理系统 - 所有颜色从 CSS 变量获取
 */

/**
 * Provider 类型定义
 */
export type ProviderType =
  | 'anthropic'
  | 'openai'
  | 'deepseek'
  | 'google'
  | 'azure'
  | 'aws'
  | 'cohere'
  | 'mistral'
  | 'custom'
  | 'antigravity';

/**
 * Client 类型定义
 */
export type ClientType = 'claude' | 'openai' | 'codex' | 'gemini';

/**
 * 颜色变量名称类型（所有可用的 CSS 变量）
 */
export type ColorVariable =
  | 'background'
  | 'foreground'
  | 'primary'
  | 'secondary'
  | 'border'
  | 'success'
  | 'warning'
  | 'error'
  | 'info'
  | `provider-${ProviderType}`
  | `client-${ClientType}`;

// 保留旧的 colors 对象用于向后兼容（已弃用，将在未来版本移除）
/** @deprecated 使用 CSS 变量和工具函数替代 */
export const colors = {
  background: '#1E1E1E',
  surfacePrimary: '#252526',
  surfaceSecondary: '#2D2D30',
  surfaceHover: '#3C3C3C',
  border: '#3C3C3C',
  textPrimary: '#CCCCCC',
  textSecondary: '#8C8C8C',
  textMuted: '#5A5A5A',
  accent: '#0078D4',
  accentHover: '#1084D9',
  accentSubtle: 'rgba(0, 120, 212, 0.15)',
  success: '#4EC9B0',
  warning: '#DDB359',
  error: '#F14C4C',
  info: '#4FC1FF',
  providers: {
    anthropic: '#D4A574',
    openai: '#10A37F',
    deepseek: '#4A90D9',
    google: '#4285F4',
    azure: '#0089D6',
    aws: '#FF9900',
    cohere: '#D97706',
    mistral: '#F97316',
    custom: '#8C8C8C',
  },
} as const;

// 间距系统
export const spacing = {
  xs: '4px',
  sm: '8px',
  md: '12px',
  lg: '16px',
  xl: '24px',
  xxl: '32px',
} as const;

// 排版系统
export const typography = {
  caption: { size: '11px', lineHeight: '1.4', weight: 400 },
  body: { size: '13px', lineHeight: '1.5', weight: 400 },
  headline: { size: '15px', lineHeight: '1.4', weight: 600 },
  title3: { size: '17px', lineHeight: '1.3', weight: 600 },
  title2: { size: '20px', lineHeight: '1.2', weight: 700 },
  title1: { size: '24px', lineHeight: '1.2', weight: 700 },
  largeTitle: { size: '28px', lineHeight: '1.1', weight: 700 },
} as const;

// 圆角
export const borderRadius = {
  sm: '4px',
  md: '8px',
  lg: '12px',
} as const;

// 阴影
export const shadows = {
  card: '0 2px 8px rgba(0, 0, 0, 0.3)',
  cardHover: '0 4px 12px rgba(0, 0, 0, 0.4)',
} as const;

/**
 * 从 CSS 变量获取计算后的颜色值
 *
 * @param varName - CSS 变量名称（不含 -- 前缀）
 * @param element - 可选的 DOM 元素，默认为 document.documentElement
 * @returns 计算后的颜色值（如 "oklch(0.7324 0.0867 56.4182)"）
 *
 * @example
 * const anthropicColor = getComputedColor('provider-anthropic')
 * // 返回: "oklch(0.7324 0.0867 56.4182)"
 */
export function getComputedColor(
  varName: ColorVariable,
  element: HTMLElement = document.documentElement
): string {
  return getComputedStyle(element)
    .getPropertyValue(`--${varName}`)
    .trim();
}

/**
 * 获取 Provider 的品牌色 CSS 变量名
 *
 * @param provider - Provider 类型
 * @returns CSS 变量引用字符串（如 "var(--provider-anthropic)"）
 *
 * @example
 * const colorVar = getProviderColorVar('anthropic')
 * // 返回: "var(--provider-anthropic)"
 *
 * // 用于组件样式
 * <div style={{ color: getProviderColorVar(provider.type) }}>
 */
export function getProviderColorVar(provider: ProviderType): string {
  return `var(--provider-${provider})`;
}

/**
 * 获取 Provider 的计算后颜色值
 *
 * @param provider - Provider 类型
 * @returns 计算后的颜色值
 *
 * @example
 * const color = getProviderColor('anthropic')
 * // 用于需要实际颜色值的场景（如 SVG fill、第三方库）
 */
export function getProviderColor(provider: ProviderType): string {
  return getComputedColor(`provider-${provider}`);
}

/**
 * 获取 Provider 显示名称
 */
export function getProviderDisplayName(type: string): string {
  const names: Record<string, string> = {
    anthropic: 'Anthropic',
    openai: 'OpenAI',
    deepseek: 'DeepSeek',
    google: 'Google',
    azure: 'Azure',
    aws: 'AWS Bedrock',
    cohere: 'Cohere',
    mistral: 'Mistral',
    custom: 'Custom',
  };
  return names[type.toLowerCase()] || type;
}

/**
 * 获取 Client 的品牌色 CSS 变量名
 *
 * @param client - Client 类型
 * @returns CSS 变量引用字符串
 *
 * @example
 * const colorVar = getClientColorVar('claude')
 * // 返回: "var(--client-claude)"
 */
export function getClientColorVar(client: ClientType): string {
  return `var(--client-${client})`;
}

/**
 * 获取 Client 的计算后颜色值
 *
 * @param client - Client 类型
 * @returns 计算后的颜色值
 *
 * @example
 * const color = getClientColor('claude')
 */
export function getClientColor(client: ClientType): string {
  return getComputedColor(`client-${client}`);
}

/**
 * 为颜色添加透明度（用于背景等场景）
 *
 * @param color - OKLCh 格式的颜色字符串
 * @param opacity - 透明度（0-1）
 * @returns 带透明度的颜色字符串
 *
 * @example
 * const bgColor = withOpacity(getProviderColor('anthropic'), 0.2)
 * // 返回: "oklch(0.7324 0.0867 56.4182 / 0.2)"
 */
export function withOpacity(color: string, opacity: number): string {
  // 处理 oklch(...) 格式
  if (color.startsWith('oklch(')) {
    const inner = color.slice(6, -1); // 移除 "oklch(" 和 ")"
    return `oklch(${inner} / ${opacity})`;
  }

  // 处理其他格式（HEX、RGB 等）- 降级处理
  console.warn(`withOpacity: 不支持的颜色格式 "${color}"，建议使用 OKLCh 格式`);
  return color;
}

/** @deprecated 使用 getClientColorVar 或 getClientColor 替代 */
export const clientColors: Record<string, string> = {
  claude: colors.providers.anthropic,
  openai: colors.providers.openai,
  codex: colors.providers.openai,
  gemini: colors.providers.google,
};
