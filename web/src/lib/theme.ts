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
  | 'antigravity'
  | 'kiro';

/**
 * Client 类型定义
 */
export type ClientType = 'claude' | 'openai' | 'codex' | 'gemini';

/**
 * Theme mode types
 */
export type ThemeMode = 'light' | 'dark' | 'system';

/**
 * Luxury theme types
 */
export type LuxuryTheme =
  | 'hermes'
  | 'tiffany'
  | 'chanel'
  | 'cartier'
  | 'burberry'
  | 'gucci'
  | 'dior';

/**
 * All available themes
 */
export type Theme = ThemeMode | LuxuryTheme;

/**
 * Typography configuration for a theme
 */
export interface ThemeTypography {
  displayFont: string;
  bodyFont: string;
  monoFont: string;
  letterSpacing: {
    tight: string;
    normal: string;
    wide: string;
  };
  lineHeight: {
    tight: number;
    normal: number;
    relaxed: number;
  };
}

/**
 * Spacing configuration for a theme
 */
export interface ThemeSpacing {
  multiplier: number;
  scale: 'compact' | 'balanced' | 'comfortable' | 'relaxed' | 'luxurious';
}

/**
 * Border radius configuration for a theme
 */
export interface ThemeRadius {
  sm: string;
  md: string;
  lg: string;
  xl: string;
}

/**
 * Shadow configuration for a theme
 */
export interface ThemeShadow {
  color: string;
  opacity: number;
  blur: { sm: string; md: string; lg: string; xl: string };
  spread: string;
  offset: { x: number; y: number };
}

/**
 * Animation configuration for a theme
 */
export interface ThemeAnimation {
  multiplier: number;
  easing: string;
  durations: { fast: string; normal: string; slow: string };
}

/**
 * Theme metadata interface
 */
export interface ThemeMetadata {
  id: Theme;
  name: string;
  description: string;
  baseMode: 'light' | 'dark';
  category: 'default' | 'luxury';
  brandInspiration?: string;
  accentColor: string;
  primaryColor: string;
  secondaryColor: string;
  typography?: ThemeTypography;
  spacing?: ThemeSpacing;
  radius?: ThemeRadius;
  shadow?: ThemeShadow;
  animation?: ThemeAnimation;
}

/**
 * Theme registry with metadata for all themes
 */
export const THEME_REGISTRY: Record<Theme, ThemeMetadata> = {
  light: {
    id: 'light',
    name: 'Light',
    description: 'Clean and bright',
    baseMode: 'light',
    category: 'default',
    accentColor: 'oklch(0.9772 0 0)', // Light gray background
    primaryColor: 'oklch(0.3261 0 0)', // Black
    secondaryColor: 'oklch(0.9772 0 0)', // Light gray
  },
  dark: {
    id: 'dark',
    name: 'Dark',
    description: 'Easy on the eyes',
    baseMode: 'dark',
    category: 'default',
    accentColor: 'oklch(0.2741 0.0055 286.0329)', // Dark gray background
    primaryColor: 'oklch(0.9848 0 0)', // White
    secondaryColor: 'oklch(0.2741 0.0055 286.0329)', // Dark gray
  },
  system: {
    id: 'system',
    name: 'System',
    description: 'Matches your system preference',
    baseMode: 'light',
    category: 'default',
    accentColor: 'oklch(0.5 0 0)', // Medium gray (represents both light and dark)
    primaryColor: 'oklch(0.3261 0 0)', // Black
    secondaryColor: 'oklch(0.9772 0 0)', // Light gray
  },
  hermes: {
    id: 'hermes',
    name: 'Hermès',
    description: 'Warm sophistication with iconic orange',
    baseMode: 'light',
    category: 'luxury',
    brandInspiration: 'Hermès',
    accentColor: 'oklch(0.72 0.14 80)',
    primaryColor: 'oklch(0.62 0.16 50)',
    secondaryColor: 'oklch(0.88 0.03 55)',
    typography: {
      displayFont: "'Cormorant Garamond', 'Playfair Display', Georgia, serif",
      bodyFont: "'Crimson Text', 'Source Serif Pro', Georgia, serif",
      monoFont: "'IBM Plex Mono', 'SF Mono', Monaco, monospace",
      letterSpacing: { tight: '-0.01em', normal: '0em', wide: '0.02em' },
      lineHeight: { tight: 1.3, normal: 1.5, relaxed: 1.7 },
    },
    spacing: { multiplier: 1.2, scale: 'relaxed' },
    radius: { sm: '12px', md: '18px', lg: '28px', xl: '40px' },
    shadow: {
      color: 'oklch(0.42 0.10 48)',
      opacity: 0.15,
      blur: { sm: '8px', md: '16px', lg: '24px', xl: '32px' },
      spread: '0px',
      offset: { x: 0, y: 2 },
    },
    animation: {
      multiplier: 1.35,
      easing: 'cubic-bezier(0.34, 0, 0.15, 1)',
      durations: { fast: '270ms', normal: '405ms', slow: '675ms' },
    },
  },
  tiffany: {
    id: 'tiffany',
    name: 'Tiffany',
    description: 'Elegant robin\'s egg blue',
    baseMode: 'light',
    category: 'luxury',
    brandInspiration: 'Tiffany & Co.',
    accentColor: 'oklch(0.72 0.13 188)',
    primaryColor: 'oklch(0.68 0.11 192)',
    secondaryColor: 'oklch(0.92 0.008 205)',
    typography: {
      displayFont: "'Montserrat', 'Inter', -apple-system, sans-serif",
      bodyFont: "'Inter Variable', 'Inter', system-ui, sans-serif",
      monoFont: "'JetBrains Mono', 'SF Mono', monospace",
      letterSpacing: { tight: '-0.015em', normal: '0.005em', wide: '0.03em' },
      lineHeight: { tight: 1.35, normal: 1.55, relaxed: 1.75 },
    },
    spacing: { multiplier: 1.12, scale: 'comfortable' },
    radius: { sm: '14px', md: '20px', lg: '30px', xl: '42px' },
    shadow: {
      color: 'oklch(0.68 0.11 192)',
      opacity: 0.06,
      blur: { sm: '6px', md: '12px', lg: '20px', xl: '28px' },
      spread: '0px',
      offset: { x: 0, y: 1 },
    },
    animation: {
      multiplier: 1.18,
      easing: 'cubic-bezier(0.22, 0.08, 0.22, 1)',
      durations: { fast: '236ms', normal: '354ms', slow: '590ms' },
    },
  },
  chanel: {
    id: 'chanel',
    name: 'Chanel',
    description: 'Timeless black and white elegance',
    baseMode: 'dark',
    category: 'luxury',
    brandInspiration: 'Chanel',
    accentColor: 'oklch(0.78 0.13 82)',
    primaryColor: 'oklch(0.98 0.002 280)',
    secondaryColor: 'oklch(0.22 0.006 280)',
    typography: {
      displayFont: "'Futura', 'Helvetica Neue', Arial, sans-serif",
      bodyFont: "'Helvetica Neue', 'Arial', sans-serif",
      monoFont: "'Fira Code', 'Consolas', monospace",
      letterSpacing: { tight: '0em', normal: '0.01em', wide: '0.05em' },
      lineHeight: { tight: 1.25, normal: 1.45, relaxed: 1.65 },
    },
    spacing: { multiplier: 0.92, scale: 'compact' },
    radius: { sm: '2px', md: '3px', lg: '5px', xl: '8px' },
    shadow: {
      color: 'oklch(0.08 0.002 280)',
      opacity: 0.35,
      blur: { sm: '4px', md: '8px', lg: '12px', xl: '16px' },
      spread: '0px',
      offset: { x: 0, y: 1 },
    },
    animation: {
      multiplier: 0.8,
      easing: 'cubic-bezier(0.45, 0, 0.65, 1)',
      durations: { fast: '160ms', normal: '240ms', slow: '400ms' },
    },
  },
  cartier: {
    id: 'cartier',
    name: 'Cartier',
    description: 'Rich burgundy with gold accents',
    baseMode: 'dark',
    category: 'luxury',
    brandInspiration: 'Cartier',
    accentColor: 'oklch(0.74 0.15 82)',
    primaryColor: 'oklch(0.42 0.20 18)',
    secondaryColor: 'oklch(0.68 0.14 78)',
    typography: {
      displayFont: "'Libre Baskerville', 'Baskerville', Georgia, serif",
      bodyFont: "'Lora', 'Georgia', serif",
      monoFont: "'Source Code Pro', Monaco, monospace",
      letterSpacing: { tight: '-0.005em', normal: '0.01em', wide: '0.04em' },
      lineHeight: { tight: 1.35, normal: 1.6, relaxed: 1.8 },
    },
    spacing: { multiplier: 1.25, scale: 'luxurious' },
    radius: { sm: '7px', md: '11px', lg: '20px', xl: '32px' },
    shadow: {
      color: 'oklch(0.38 0.20 18)',
      opacity: 0.22,
      blur: { sm: '10px', md: '20px', lg: '30px', xl: '40px' },
      spread: '2px',
      offset: { x: 0, y: 3 },
    },
    animation: {
      multiplier: 1.45,
      easing: 'cubic-bezier(0.28, 0, 0.18, 1)',
      durations: { fast: '290ms', normal: '435ms', slow: '725ms' },
    },
  },
  burberry: {
    id: 'burberry',
    name: 'Burberry',
    description: 'Classic heritage tan',
    baseMode: 'light',
    category: 'luxury',
    brandInspiration: 'Burberry',
    accentColor: 'oklch(0.48 0.20 22)',
    primaryColor: 'oklch(0.58 0.09 62)',
    secondaryColor: 'oklch(0.89 0.018 66)',
    typography: {
      displayFont: "'Merriweather', 'Times New Roman', serif",
      bodyFont: "'Merriweather', 'Georgia', serif",
      monoFont: "'Courier Prime', 'Courier New', monospace",
      letterSpacing: { tight: '-0.01em', normal: '0em', wide: '0.025em' },
      lineHeight: { tight: 1.4, normal: 1.65, relaxed: 1.85 },
    },
    spacing: { multiplier: 1.08, scale: 'balanced' },
    radius: { sm: '9px', md: '13px', lg: '20px', xl: '30px' },
    shadow: {
      color: 'oklch(0.48 0.04 64)',
      opacity: 0.12,
      blur: { sm: '7px', md: '14px', lg: '22px', xl: '30px' },
      spread: '0px',
      offset: { x: 0, y: 2 },
    },
    animation: {
      multiplier: 1.12,
      easing: 'cubic-bezier(0.32, 0, 0.22, 1)',
      durations: { fast: '224ms', normal: '336ms', slow: '560ms' },
    },
  },
  gucci: {
    id: 'gucci',
    name: 'Gucci',
    description: 'Bold forest green with gold',
    baseMode: 'dark',
    category: 'luxury',
    brandInspiration: 'Gucci',
    accentColor: 'oklch(0.70 0.15 78)',
    primaryColor: 'oklch(0.38 0.14 152)',
    secondaryColor: 'oklch(0.42 0.20 18)',
    typography: {
      displayFont: "'Cinzel', 'Palatino', serif",
      bodyFont: "'Raleway', 'Helvetica', sans-serif",
      monoFont: "'Roboto Mono', monospace",
      letterSpacing: { tight: '0em', normal: '0.015em', wide: '0.06em' },
      lineHeight: { tight: 1.3, normal: 1.5, relaxed: 1.7 },
    },
    spacing: { multiplier: 1.05, scale: 'balanced' },
    radius: { sm: '11px', md: '16px', lg: '24px', xl: '34px' },
    shadow: {
      color: 'oklch(0.32 0.14 152)',
      opacity: 0.20,
      blur: { sm: '9px', md: '18px', lg: '27px', xl: '36px' },
      spread: '1px',
      offset: { x: 0, y: 2 },
    },
    animation: {
      multiplier: 1.05,
      easing: 'cubic-bezier(0.42, 0, 0.12, 1)',
      durations: { fast: '210ms', normal: '315ms', slow: '525ms' },
    },
  },
  dior: {
    id: 'dior',
    name: 'Dior',
    description: 'Soft understated elegance',
    baseMode: 'light',
    category: 'luxury',
    brandInspiration: 'Dior',
    accentColor: 'oklch(0.66 0.09 22)',
    primaryColor: 'oklch(0.52 0.018 258)',
    secondaryColor: 'oklch(0.88 0.006 255)',
    typography: {
      displayFont: "'Didot', 'Bodoni Moda', 'Playfair Display', serif",
      bodyFont: "'Lato', 'Helvetica Neue', sans-serif",
      monoFont: "'Anonymous Pro', monospace",
      letterSpacing: { tight: '-0.02em', normal: '0.005em', wide: '0.035em' },
      lineHeight: { tight: 1.3, normal: 1.55, relaxed: 1.75 },
    },
    spacing: { multiplier: 1.1, scale: 'comfortable' },
    radius: { sm: '10px', md: '16px', lg: '24px', xl: '36px' },
    shadow: {
      color: 'oklch(0.50 0.015 260)',
      opacity: 0.05,
      blur: { sm: '5px', md: '10px', lg: '18px', xl: '26px' },
      spread: '0px',
      offset: { x: 0, y: 1 },
    },
    animation: {
      multiplier: 1.25,
      easing: 'cubic-bezier(0.26, 0.08, 0.18, 1)',
      durations: { fast: '250ms', normal: '375ms', slow: '625ms' },
    },
  },
};

/**
 * Get theme metadata with runtime validation
 * Returns 'system' theme metadata as fallback for invalid theme IDs
 */
export function getThemeMetadata(theme: Theme): ThemeMetadata {
  // Runtime check for invalid theme IDs (e.g., stale values from localStorage)
  if (!(theme in THEME_REGISTRY)) {
    console.warn(`Invalid theme ID "${theme}" detected. Falling back to "system" theme.`);
    return THEME_REGISTRY.system;
  }
  return THEME_REGISTRY[theme];
}

/**
 * Check if theme is a luxury theme
 */
export function isLuxuryTheme(theme: Theme): boolean {
  return THEME_REGISTRY[theme].category === 'luxury';
}

/**
 * Get the base mode (light/dark) for a theme
 */
export function getThemeBaseMode(theme: Theme): 'light' | 'dark' {
  return THEME_REGISTRY[theme].baseMode;
}

/**
 * Get all luxury themes
 */
export function getLuxuryThemes(): ThemeMetadata[] {
  return Object.values(THEME_REGISTRY).filter(t => t.category === 'luxury');
}

/**
 * Get all default themes
 */
export function getDefaultThemes(): ThemeMetadata[] {
  return Object.values(THEME_REGISTRY).filter(t => t.category === 'default');
}

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
  element: HTMLElement = document.documentElement,
): string {
  return getComputedStyle(element).getPropertyValue(`--${varName}`).trim();
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
