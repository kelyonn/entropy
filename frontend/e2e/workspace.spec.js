import { test, expect } from '@playwright/test';

async function startSandbox(page) {
  await page.goto('/');
  await expect(page.getByText('All commands run in isolation.')).toBeVisible();
  await expect(page.getByText('rm -rf /', { exact: true })).toBeVisible();
  await page.getByRole('button', { name: 'Start sandbox' }).click();
  await expect(page.getByText('interactive', { exact: true })).toBeVisible({ timeout: 45_000 });
}

async function run(page, command, wait = 1500) {
  await page.locator('.xterm').click();
  await page.keyboard.type(command);
  await page.keyboard.press('Enter');
  await page.waitForTimeout(wait);
}

test('interactive terminal tracks commands and explains failures', async ({ page }) => {
  await startSandbox(page);
  await run(page, 'cd /var/log && pwd');
  await expect(page.locator('.term-well')).toContainText('root@entropy:/var/log');
  await run(page, 'ls /definitely-missing', 2500);
  const activity = page.locator('section', { hasText: 'Activity' });
  await expect(activity).toContainText('exit 2');
  await expect(activity).toContainText('ls /definitely-missing');
  // The fake model echoes the command it was asked about.
  await expect(page.locator('section', { hasText: 'Explanation' })).toContainText('exit 2 from', { timeout: 15_000 });
  await run(page, 'sleep 30', 800);
  await page.keyboard.press('Control+C');
  await expect(activity).toContainText('exit 130', { timeout: 10_000 });
});

test('starter lab resolves from learner commands', async ({ page }) => {
  await startSandbox(page);
  await page.getByRole('button', { name: /First steps: processes and signals/ }).click();
  await page.getByRole('button', { name: 'Start incident' }).click();
  await expect(page.getByText('In progress')).toBeVisible({ timeout: 30_000 });
  await run(page, 'ps aux | grep lesson');
  await run(page, 'free -m');
  await run(page, 'cat /opt/lesson/.pid > /root/answer.txt');
  await run(page, 'kill $(cat /root/answer.txt)', 3000);
  await expect(page.getByText('Resolved', { exact: true })).toBeVisible({ timeout: 20_000 });
  await expect(page.getByText(/A zombie|kill sends a signal/)).toBeVisible();
});
