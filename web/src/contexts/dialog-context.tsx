import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useRef,
  useState,
  type ReactNode,
} from 'react';
import { useTranslation } from 'react-i18next';
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@/components/ui';
import { Button } from '@/components/ui/button';

type DialogButtonVariant = React.ComponentProps<typeof Button>['variant'];

interface BaseDialogOptions {
  title?: ReactNode;
  description?: ReactNode;
  confirmText?: ReactNode;
  confirmVariant?: DialogButtonVariant;
}

interface ConfirmDialogOptions extends BaseDialogOptions {
  cancelText?: ReactNode;
}

interface AlertDialogOptions extends BaseDialogOptions {}

interface DialogContextValue {
  alert: (options: AlertDialogOptions) => Promise<void>;
  confirm: (options: ConfirmDialogOptions) => Promise<boolean>;
}

type DialogRequest =
  | {
      id: number;
      kind: 'alert';
      options: AlertDialogOptions;
      resolve: () => void;
    }
  | {
      id: number;
      kind: 'confirm';
      options: ConfirmDialogOptions;
      resolve: (value: boolean) => void;
    };

const DialogContext = createContext<DialogContextValue | null>(null);

interface DialogProviderProps {
  children: ReactNode;
}

export function DialogProvider({ children }: DialogProviderProps) {
  const { t } = useTranslation();
  const [activeRequest, setActiveRequest] = useState<DialogRequest | null>(null);
  const activeRequestRef = useRef<DialogRequest | null>(null);
  const queueRef = useRef<DialogRequest[]>([]);
  const nextIdRef = useRef(1);

  const resolveRequest = useCallback((request: DialogRequest, confirmed: boolean) => {
    if (request.kind === 'confirm') {
      request.resolve(confirmed);
      return;
    }

    request.resolve();
  }, []);

  const showNextRequest = useCallback(() => {
    const nextRequest = queueRef.current.shift() ?? null;
    activeRequestRef.current = nextRequest;
    setActiveRequest(nextRequest);
  }, []);

  const closeActiveRequest = useCallback(
    (confirmed: boolean) => {
      const request = activeRequestRef.current;
      if (!request) return;

      activeRequestRef.current = null;
      resolveRequest(request, confirmed);
      showNextRequest();
    },
    [resolveRequest, showNextRequest],
  );

  const enqueueRequest = useCallback((request: DialogRequest) => {
    if (activeRequestRef.current) {
      queueRef.current.push(request);
      return;
    }

    activeRequestRef.current = request;
    setActiveRequest(request);
  }, []);

  const confirm = useCallback(
    (options: ConfirmDialogOptions) =>
      new Promise<boolean>((resolve) => {
        enqueueRequest({
          id: nextIdRef.current++,
          kind: 'confirm',
          options,
          resolve,
        });
      }),
    [enqueueRequest],
  );

  const alert = useCallback(
    (options: AlertDialogOptions) =>
      new Promise<void>((resolve) => {
        enqueueRequest({
          id: nextIdRef.current++,
          kind: 'alert',
          options,
          resolve,
        });
      }),
    [enqueueRequest],
  );

  useEffect(() => {
    return () => {
      const pendingRequests = [
        ...(activeRequestRef.current ? [activeRequestRef.current] : []),
        ...queueRef.current,
      ];

      pendingRequests.forEach((request) => resolveRequest(request, false));
      activeRequestRef.current = null;
      queueRef.current = [];
    };
  }, [resolveRequest]);

  const value = useMemo(
    () => ({
      alert,
      confirm,
    }),
    [alert, confirm],
  );

  const title =
    activeRequest?.options.title ??
    t(activeRequest?.kind === 'confirm' ? 'common.confirm' : 'nav.notifications');
  const confirmText =
    activeRequest?.options.confirmText ??
    t(activeRequest?.kind === 'confirm' ? 'common.confirm' : 'common.ok');
  const confirmVariant = activeRequest?.options.confirmVariant ?? 'default';
  const cancelText =
    activeRequest?.kind === 'confirm'
      ? (activeRequest.options.cancelText ?? t('common.cancel'))
      : null;

  return (
    <DialogContext.Provider value={value}>
      {children}
      <AlertDialog
        key={activeRequest?.id ?? 0}
        open={activeRequest !== null}
        onOpenChange={(open) => {
          if (!open) {
            closeActiveRequest(false);
          }
        }}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>{title}</AlertDialogTitle>
            {activeRequest?.options.description ? (
              <AlertDialogDescription>{activeRequest.options.description}</AlertDialogDescription>
            ) : null}
          </AlertDialogHeader>
          <AlertDialogFooter>
            {activeRequest?.kind === 'confirm' ? (
              <AlertDialogCancel>{cancelText}</AlertDialogCancel>
            ) : null}
            <AlertDialogAction variant={confirmVariant} onClick={() => closeActiveRequest(true)}>
              {confirmText}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </DialogContext.Provider>
  );
}

export function useDialog() {
  const context = useContext(DialogContext);

  if (!context) {
    throw new Error('useDialog must be used within a DialogProvider');
  }

  return context;
}
