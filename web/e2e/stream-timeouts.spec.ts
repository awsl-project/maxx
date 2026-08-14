import { expect, test } from '@playwright/test';

const SETTINGS = [
  'openai_chat_stream_timeouts_enabled',
  'openai_chat_stream_first_event_timeout_ms',
  'openai_chat_stream_idle_timeout_ms',
];

async function resetStreamTimeoutSettings(request: Parameters<typeof test>[0]['request']) {
  for (const key of SETTINGS) {
    await request.delete(`/api/admin/settings/${key}`);
  }
}

test.describe('stream timeout settings', () => {
  test.beforeEach(async ({ request }) => {
    await resetStreamTimeoutSettings(request);
  });

  test.afterEach(async ({ request }) => {
    await resetStreamTimeoutSettings(request);
  });

  test('keeps upstream stream timeouts opt-in and persists user values', async ({ page }) => {
    await page.goto('/settings');

    const streamTimeoutsTitle = page.getByText('OpenAI Chat stream timeouts', { exact: true });
    await expect(streamTimeoutsTitle).toBeAttached({ timeout: 15000 });
    await streamTimeoutsTitle.scrollIntoViewIfNeeded();
    await expect(streamTimeoutsTitle).toBeVisible();
    const enabledSwitch = page.getByRole('switch', {
      name: 'Enable OpenAI Chat upstream stream timeouts',
    });
    const firstEventInput = page.getByLabel('OpenAI Chat first event timeout');
    const idleInput = page.getByLabel('OpenAI Chat event idle timeout');

    await expect(enabledSwitch).not.toBeChecked();
    await expect(firstEventInput).toBeDisabled();
    await expect(idleInput).toBeDisabled();
    await expect(firstEventInput).toHaveValue('20000');
    await expect(idleInput).toHaveValue('45000');

    await enabledSwitch.click();
    await expect(firstEventInput).toBeEnabled();
    await expect(idleInput).toBeEnabled();

    await firstEventInput.fill('15000');
    await idleInput.fill('55000');
    const saveButton = page.getByRole('button', { name: 'Save', exact: true });
    await expect(saveButton).toBeEnabled();
    const saveResponses = Promise.all(
      SETTINGS.map((key) =>
        page.waitForResponse(
          (response) =>
            response.url().includes(`/api/admin/settings/${key}`) &&
            response.request().method() === 'PUT' &&
            response.ok(),
        ),
      ),
    );
    await saveButton.click();
    await saveResponses;

    await page.reload();
    await expect(enabledSwitch).toBeChecked();
    await expect(firstEventInput).toHaveValue('15000');
    await expect(idleInput).toHaveValue('55000');

    await firstEventInput.fill('999');
    await saveButton.click();
    await expect(
      page.getByText('Timeouts must be integers between 1000 and 600000 ms.'),
    ).toBeVisible();
  });
});
