import { createElement, act } from 'react';
import { createRoot } from 'react-dom/client';
import { describe, it, expect, vi, afterEach } from 'vitest';
import { WorkBuddyAIForm } from './workbuddy-ai-form';
const mocks = vi.hoisted(() => ({
  create: vi.fn(),
  update: vi.fn(),
  mapping: vi.fn(),
  discover: vi.fn(),
  listMappings: vi.fn().mockResolvedValue([]),
  updateMapping: vi.fn().mockResolvedValue({}),
}));
vi.mock('@/components/layout/page-header', () => ({
  PageHeader: ({ children }: { children: import('react').ReactNode }) =>
    createElement('header', {}, children),
}));
vi.mock('react-router-dom', () => ({ useNavigate: () => vi.fn() }));
vi.mock('react-i18next', () => ({ useTranslation: () => ({ t: (key: string) => key }) }));
vi.mock('@/hooks/queries', () => ({
  useCreateProvider: () => ({ mutateAsync: mocks.create }),
  useUpdateProvider: () => ({ mutateAsync: mocks.update }),
  useCreateModelMapping: () => ({ mutateAsync: mocks.mapping }),
}));
vi.mock('@/lib/transport', () => ({
  getTransport: () => ({
    getWorkBuddyAIModels: mocks.discover,
    getModelMappings: mocks.listMappings,
    updateModelMapping: mocks.updateMapping,
  }),
}));
vi.mock('./provider-model-mappings', () => ({
  ProviderModelMappings: () => createElement('div', { 'data-testid': 'saved-mappings' }),
}));
vi.mock('./provider-support-models', () => ({
  ProviderSupportModels: ({
    supportModels,
    onChange,
  }: {
    supportModels: string[];
    onChange: (v: string[]) => void;
  }) =>
    createElement(
      'button',
      { onClick: () => onChange([]), 'data-testid': 'support' },
      supportModels.join(','),
    ),
}));
vi.mock('./model-mapping-editor', () => ({
  ModelMappingEditor: ({
    value,
    onChange,
  }: {
    value: Record<string, string>;
    onChange: (v: Record<string, string>) => void;
  }) =>
    createElement(
      'div',
      { 'data-testid': 'pending' },
      JSON.stringify(value),
      createElement(
        'button',
        { onClick: () => onChange({ alias: 'target', second: 'second-target' }) },
        'add mapping',
      ),
    ),
}));
(globalThis as Record<string, unknown>).IS_REACT_ACT_ENVIRONMENT = true;
afterEach(() => vi.clearAllMocks());
async function render(provider?: Record<string, unknown>) {
  const host = document.createElement('div');
  document.body.append(host);
  const root = createRoot(host);
  const close = vi.fn();
  await act(async () =>
    root.render(createElement(WorkBuddyAIForm, { provider: provider as never, onClose: close })),
  );
  const click = async (text: string) => {
    const button = [...host.querySelectorAll('button')].find((b) => b.textContent === text)!;
    await act(async () => button.click());
  };
  return {
    host,
    close,
    click,
    cleanup: async () => {
      await act(async () => root.unmount());
      host.remove();
    },
  };
}
describe('WorkBuddy model policy persistence', () => {
  it('reconciles a committed mapping after response loss without duplicating it', async () => {
    mocks.create.mockResolvedValue({ id: 42, name: 'WorkBuddy AI', type: 'workbuddy-ai' });
    const persisted: Record<string, unknown>[] = [];
    mocks.listMappings.mockImplementation(async () => persisted);
    mocks.mapping
      .mockImplementationOnce(async (input) => {
        persisted.push({ ...input, id: 99, priority: 1000 });
        throw new Error('response lost');
      })
      .mockResolvedValue({});
    const ui = await render();
    const input = ui.host.querySelector('#workbuddy-accessToken') as HTMLInputElement;
    await act(async () => {
      Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value')!.set!.call(
        input,
        'dummy',
      );
      input.dispatchEvent(new Event('input', { bubbles: true }));
    });
    await ui.click('add mapping');
    await ui.click('common.save');
    expect(ui.close).not.toHaveBeenCalled();
    await ui.click('common.save');
    expect(mocks.mapping.mock.calls.map(([value]) => value.pattern)).toEqual(['alias', 'second']);
    expect(mocks.updateMapping).toHaveBeenCalledWith(
      99,
      expect.objectContaining({
        pattern: 'alias',
        target: 'target',
        providerID: 42,
        priority: 1000,
      }),
    );
    expect(mocks.create).toHaveBeenCalledOnce();
    expect(ui.close).toHaveBeenCalledOnce();
    mocks.listMappings.mockResolvedValue([]);
    await ui.cleanup();
  });
  it('loads existing support models, never imports discovered catalog, and saves explicit clearing', async () => {
    mocks.discover.mockResolvedValue({ models: ['catalog-only'] });
    mocks.update.mockResolvedValue({});
    const ui = await render({
      id: 41,
      type: 'workbuddy-ai',
      name: 'test',
      supportModels: ['target'],
      config: { workbuddyAI: { uid: 'dummy' } },
    });
    expect(ui.host.querySelector('[data-testid=support]')?.textContent).toBe('target');
    await ui.click('addProvider.workbuddyAI.discoverModels');
    expect(ui.host.querySelector('[data-testid=support]')?.textContent).toBe('target');
    await act(async () =>
      (ui.host.querySelector('[data-testid=support]') as HTMLButtonElement).click(),
    );
    await ui.click('common.save');
    expect(mocks.update.mock.calls[0][0]).toMatchObject({ id: 41, data: { supportModels: [] } });
    expect(mocks.mapping).not.toHaveBeenCalled();
    expect(ui.close).toHaveBeenCalledOnce();
    await ui.cleanup();
  });
  it('creates scoped mappings only after obtaining provider ID and retries without duplicate providers', async () => {
    mocks.create.mockResolvedValue({ id: 42, name: 'WorkBuddy AI', type: 'workbuddy-ai' });
    mocks.mapping
      .mockResolvedValueOnce({})
      .mockRejectedValueOnce(new Error('offline'))
      .mockResolvedValueOnce({});
    mocks.update.mockResolvedValue({});
    const ui = await render();
    const input = ui.host.querySelector('#workbuddy-accessToken') as HTMLInputElement;
    await act(async () => {
      Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value')!.set!.call(
        input,
        'dummy',
      );
      input.dispatchEvent(new Event('input', { bubbles: true }));
    });
    await ui.click('add mapping');
    await ui.click('common.save');
    expect(ui.close).not.toHaveBeenCalled();
    expect(ui.host.querySelector('[data-testid=pending]')?.textContent).toContain('second-target');
    expect(ui.host.querySelector('[data-testid=pending]')?.textContent).not.toContain('alias');
    expect(ui.host.querySelector('[data-testid=saved-mappings]')).toBeNull();
    await ui.click('common.save');
    expect(mocks.mapping.mock.calls.map(([input]) => input.pattern)).toEqual([
      'alias',
      'second',
      'second',
    ]);
    expect(mocks.mapping.mock.calls[2][0]).toMatchObject({
      scope: 'provider',
      providerID: 42,
      providerType: 'workbuddy-ai',
    });
    expect(mocks.create).toHaveBeenCalledOnce();
    expect(mocks.mapping.mock.calls[0][0]).toEqual({
      scope: 'provider',
      providerID: 42,
      providerType: 'workbuddy-ai',
      pattern: 'alias',
      target: 'target',
      isEnabled: true,
    });
    expect(ui.close).toHaveBeenCalledOnce();
    await ui.cleanup();
  });
});
