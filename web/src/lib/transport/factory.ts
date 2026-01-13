/**
 * Transport 工厂函数和环境检测
 *
 * 重要：Transport 必须通过 initializeTransport() 异步初始化
 * 不要在模块顶层调用 getTransport()，这会导致竞态条件
 */

import type { Transport, TransportType, TransportConfig } from './interface';
import { HttpTransport } from './http-transport';

/**
 * Transport 初始化状态
 */
type TransportState =
  | { status: 'uninitialized' }
  | { status: 'initializing' }
  | { status: 'ready'; transport: Transport; type: TransportType }
  | { status: 'error'; error: Error };

let state: TransportState = { status: 'uninitialized' };
let initPromise: Promise<Transport> | null = null;

/**
 * 检测当前运行环境
 * 最可靠的方式是检测 protocol，Wails 使用 wails:// 协议
 */
export function detectTransportType(): TransportType {
  if (typeof window !== 'undefined') {
    // 最可靠的检测：Wails webview 使用 wails:// 协议
    if (window.location.protocol === 'wails:') {
      return 'wails';
    }
    // Wails v3
    if (window.__WAILS__) {
      return 'wails';
    }
    // Wails v2 - 检测 window.go (Go bindings) 或 window.runtime (Wails runtime)
    const win = window as unknown as { go?: unknown; runtime?: unknown };
    if (win.go || win.runtime) {
      return 'wails';
    }
  }
  return 'http';
}

/**
 * 检测是否在 Wails 环境中运行
 */
export function isWailsEnvironment(): boolean {
  return detectTransportType() === 'wails';
}

/**
 * 等待 Wails runtime 准备好
 * 在生产构建中，window.go 可能在脚本执行时还没有被注入
 */
function waitForWailsRuntime(timeout = 5000): Promise<boolean> {
  return new Promise((resolve) => {
    // 最可靠的检测：通过协议判断是否在 Wails 环境
    if (window.location.protocol === 'wails:') {
      console.log('[Transport] Wails environment confirmed by protocol');
      resolve(true);
      return;
    }

    // 然后检查 Wails 对象
    const win = window as unknown as { go?: unknown; runtime?: unknown };
    if (win.go || win.runtime || window.__WAILS__) {
      console.log('[Transport] Wails runtime already available');
      resolve(true);
      return;
    }

    console.log('[Transport] Waiting for Wails runtime...');
    const startTime = Date.now();

    // 使用 setInterval 更可靠地检测
    const checkInterval = setInterval(() => {
      const w = window as unknown as { go?: unknown; runtime?: unknown };
      if (w.go || w.runtime || window.__WAILS__) {
        console.log('[Transport] Wails runtime detected after wait');
        clearInterval(checkInterval);
        resolve(true);
        return;
      }

      if (Date.now() - startTime > timeout) {
        console.log(
          '[Transport] Wails runtime wait timeout, window keys:',
          Object.keys(window).filter(
            (k) => k.includes('go') || k.includes('wails') || k.includes('runtime')
          )
        );
        clearInterval(checkInterval);
        resolve(false);
        return;
      }
    }, 50); // 每 50ms 检查一次
  });
}

/**
 * 创建 Transport 实例
 * 注意：Wails 环境下使用动态导入，避免在 web 模式下导入 @wailsio/runtime
 */
async function createTransportAsync(
  initialType: TransportType,
  config?: TransportConfig
): Promise<Transport> {
  // 如果初始检测明确为 http（非 wails:// 协议），直接使用 HttpTransport
  // 无需等待 Wails runtime
  if (initialType === 'http') {
    return new HttpTransport(config);
  }

  // 对于可能是 Wails 环境的情况，等待 runtime 准备好
  const isWails = await waitForWailsRuntime();

  if (isWails) {
    // 动态导入 WailsTransport，只在 Wails 环境下加载
    const { WailsTransport } = await import('./wails-transport');
    console.log('[Transport] Using WailsTransport');
    return new WailsTransport(config);
  }

  // Fallback to HTTP
  return new HttpTransport(config);
}

/**
 * 初始化全局 Transport 单例（异步）
 * 在应用启动时调用一次
 *
 * 重要：必须在使用 getTransport() 之前调用此方法
 */
export async function initializeTransport(config?: TransportConfig): Promise<Transport> {
  // 已经初始化完成
  if (state.status === 'ready') {
    return state.transport;
  }

  // 正在初始化中，等待完成
  if (state.status === 'initializing' && initPromise) {
    return initPromise;
  }

  // 初始化失败过
  if (state.status === 'error') {
    throw state.error;
  }

  // 开始初始化
  state = { status: 'initializing' };
  const detectedType = detectTransportType();
  console.log('[Transport] Initializing... detected type:', detectedType);

  initPromise = createTransportAsync(detectedType, config)
    .then((transport) => {
      state = { status: 'ready', transport, type: detectedType };
      console.log('[Transport] Ready:', transport.constructor.name);
      return transport;
    })
    .catch((error) => {
      state = { status: 'error', error };
      console.error('[Transport] Initialization failed:', error);
      throw error;
    });

  return initPromise;
}

/**
 * 获取全局 Transport 单例
 *
 * 重要：必须先调用 initializeTransport() 完成初始化
 * 在 React 组件中，使用 useTransport() hook 来获取 transport
 *
 * @throws Error 如果 transport 未初始化
 */
export function getTransport(): Transport {
  if (state.status === 'ready') {
    return state.transport;
  }

  if (state.status === 'uninitialized') {
    throw new Error(
      '[Transport] Transport not initialized. Call initializeTransport() first, or use useTransport() hook.'
    );
  }

  if (state.status === 'initializing') {
    throw new Error(
      '[Transport] Transport is still initializing. Use useTransport() hook or await initializeTransport().'
    );
  }

  if (state.status === 'error') {
    throw state.error;
  }

  // This should never happen
  throw new Error('[Transport] Unknown transport state');
}

/**
 * 获取当前 Transport 初始化状态
 * 用于 TransportProvider 等需要检查状态的场景
 */
export function getTransportState(): TransportState {
  return state;
}

/**
 * 检查 Transport 是否已初始化完成
 */
export function isTransportReady(): boolean {
  return state.status === 'ready';
}

/**
 * 获取 Transport 类型
 * 仅在初始化完成后有效
 */
export function getTransportType(): TransportType | null {
  if (state.status === 'ready') {
    return state.type;
  }
  return null;
}

/**
 * 重置 Transport 单例（用于测试）
 */
export function resetTransport(): void {
  if (state.status === 'ready') {
    state.transport.disconnect();
  }
  state = { status: 'uninitialized' };
  initPromise = null;
}
