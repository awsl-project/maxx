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

/**
 * Provides a queue-based alert/confirm dialog system so dialogs never overlap.
 */
export function DialogProvider({ children }: DialogProviderProps) {
  const { t } = useTranslation();
  const [activeRequest, setActiveRequest] = useState<DialogRequest | null>(null);
  const activeRequestRef = useRef<DialogRequest | null>(null);
  const queueRef = useRef<DialogRequest[]>([]);
  const nextIdRef = useRef(1);

  /**
   * Resolves the current dialog request with the provided user decision.
   */
  const resolveRequest = useCallback((request: DialogRequest, confirmed: boolean) => {
    if (request.kind === 'confirm') {
      request.resolve(confirmed);
      return;
    }

    request.resolve();
  }, []);

  /**
   * Advances the queue to the next pending dialog, if any.
   */
  const showNextRequest = useCallback(() => {
    const nextRequest = queueRef.current.shift() ?? null;
    activeRequestRef.current = nextRequest;
    setActiveRequest(nextRequest);
  }, []);

  /**
   * Closes the active dialog and continues with the next queued request.
   */
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

  /**
   * Queues a dialog request, immediately showing it when no dialog is active.
   */
  const enqueueRequest = useCallback((request: DialogRequest) => {
    if (activeRequestRef.current) {
      queueRef.current.push(request);
      return;
    }

    activeRequestRef.current = request;
    setActiveRequest(request);
  }, []);

  /**
   * Shows a confirm dialog and resolves with the user's choice.
   */
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

  /**
   * Shows an alert dialog and resolves when the user acknowledges.
   */
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

      pendingRequests.forEach((request) => {
        resolveRequest(request, false);
      });
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

/**
 * Hook that exposes the dialog helpers; must be used inside DialogProvider.
 */
export function useDialog() {
  const context = useContext(DialogContext);

  if (!context) {
    throw new Error('useDialog must be used within a DialogProvider');
  }

  return context;
}
