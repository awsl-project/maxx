/**
 * Transport React Context
 *
 * 提供 Transport 实例给 React 组件树
 * 确保 Transport 在组件渲染前已完成初始化
 */

import { createContext, useContext, useState, useEffect, type ReactNode } from 'react';
import type { Transport, TransportType } from './interface';
import { initializeTransport, getTransport, isTransportReady, getTransportType } from './factory';
import { buildTransportConfig } from '../backend-config';

/**
 * Transport Context 的值类型
 */
interface TransportContextValue {
  transport: Transport;
  type: TransportType;
  isReady: boolean;
}

const TransportContext = createContext<TransportContextValue | null>(null);

/**
 * Transport Provider 的 props
 */
interface TransportProviderProps {
  children: ReactNode;
  /** 加载中显示的内容 */
  fallback?: ReactNode;
  /** 初始化错误时显示的内容 */
  errorFallback?: (error: Error) => ReactNode;
}

/**
 * Transport 初始化状态
 */
type InitState =
  | { status: 'loading' }
  | { status: 'ready'; transport: Transport; type: TransportType }
  | { status: 'error'; error: Error };

/**
 * TransportProvider 组件
 *
 * 在组件树顶层包裹，确保 Transport 初始化完成后才渲染子组件
 *
 * @example
 * ```tsx
 * <TransportProvider fallback={<Loading />}>
 *   <App />
 * </TransportProvider>
 * ```
 */
export function TransportProvider({
  children,
  fallback = null,
  errorFallback,
}: TransportProviderProps) {
  const [state, setState] = useState<InitState>(() => {
    // 检查是否已经初始化完成
    if (isTransportReady()) {
      const transport = getTransport();
      const type = getTransportType()!;
      return { status: 'ready', transport, type };
    }
    return { status: 'loading' };
  });

  const readyTransport = state.status === 'ready' ? state.transport : null;

  useEffect(() => {
    let cancelled = false;

    if (state.status === 'error') {
      return () => {
        cancelled = true;
      };
    }

    // 已经 ready 后，后台尝试连接 WebSocket（不阻塞 UI）
    if (readyTransport) {
      if (!readyTransport.isConnected()) {
        readyTransport
          .connect()
          .then(() => {
            if (!cancelled) {
              console.log('[TransportProvider] WebSocket connected');
            }
          })
          .catch((error) => {
            if (!cancelled) {
              console.error('[TransportProvider] WebSocket connect failed (non-blocking):', error);
            }
          });
      }

      return () => {
        cancelled = true;
      };
    }

    console.log('[TransportProvider] Initializing transport...');

    initializeTransport(buildTransportConfig())
      .then((transport) => {
        if (!cancelled) {
          const type = getTransportType()!;
          console.log('[TransportProvider] Ready:', transport.constructor.name);
          setState({ status: 'ready', transport, type });
        }
      })
      .catch((error) => {
        if (!cancelled) {
          console.error('[TransportProvider] Error:', error);
          setState({ status: 'error', error });
        }
      });

    return () => {
      cancelled = true;
    };
  }, [state.status, readyTransport]);

  // 加载中
  if (state.status === 'loading') {
    return <>{fallback}</>;
  }

  // 错误
  if (state.status === 'error') {
    if (errorFallback) {
      return <>{errorFallback(state.error)}</>;
    }
    // 默认错误显示
    return (
      <div style={{ padding: 20, color: 'red' }}>
        <h2>Transport Initialization Failed</h2>
        <pre>{state.error.message}</pre>
      </div>
    );
  }

  // 就绪
  const value: TransportContextValue = {
    transport: state.transport,
    type: state.type,
    isReady: true,
  };

  return <TransportContext.Provider value={value}>{children}</TransportContext.Provider>;
}

/**
 * useTransport Hook
 *
 * 从 Context 获取 Transport 实例
 * 必须在 TransportProvider 内使用
 *
 * @throws Error 如果在 TransportProvider 外使用
 *
 * @example
 * ```tsx
 * function MyComponent() {
 *   const { transport, type } = useTransport();
 *   // ...
 * }
 * ```
 */
export function useTransport(): TransportContextValue {
  const context = useContext(TransportContext);

  if (!context) {
    throw new Error(
      '[useTransport] Must be used within a TransportProvider. ' +
        'Wrap your app with <TransportProvider> first.',
    );
  }

  return context;
}

/**
 * useTransportType Hook
 *
 * 获取当前 Transport 类型
 */
export function useTransportType(): TransportType {
  return useTransport().type;
}
