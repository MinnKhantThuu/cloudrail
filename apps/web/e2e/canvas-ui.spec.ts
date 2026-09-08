import { expect, test } from '@playwright/test';
import { mkdirSync } from 'node:fs';
import { fileURLToPath } from 'node:url';

const screenshots = fileURLToPath(new URL('../../../.data/screenshots/', import.meta.url));
mkdirSync(screenshots, { recursive: true });

const state = {
  projects: [{ id: 'project-1', name: 'Commerce API', createdAt: '2026-09-09T00:00:00Z' }],
  environments: [{ projectId: 'project-1', name: 'production' }],
  services: [
    { id: 'app-1', projectId: 'project-1', name: 'storefront-api', environment: 'production', host: 'api.example.test', url: 'https://api.example.test', activeId: 'deploy-1', desiredState: 'running', resourceKind: 'service', workloadMode: 'web', template: '', createdAt: '2026-09-09T00:00:00Z', settings: { kind: 'http', memoryMB: 256, cpuMillis: 1000, mountPath: '', volumeName: '', network: 'cloudrail' } },
    { id: 'worker-1', projectId: 'project-1', name: 'email-queue', environment: 'production', host: 'worker.example.test', url: '', activeId: 'deploy-worker', desiredState: 'running', resourceKind: 'service', workloadMode: 'worker', template: '', createdAt: '2026-09-09T00:00:00Z', settings: { kind: 'http', memoryMB: 256, cpuMillis: 1000, mountPath: '', volumeName: '', network: 'cloudrail' } },
    { id: 'db-1', projectId: 'project-1', name: 'postgres', environment: 'production', host: '', url: '', activeId: 'deploy-db', desiredState: 'running', resourceKind: 'database', workloadMode: 'web', template: 'postgres', createdAt: '2026-09-09T00:00:00Z', settings: { kind: 'postgres', memoryMB: 384, cpuMillis: 1000, mountPath: '/var/lib/postgresql/data', volumeName: 'postgres-data', network: 'cloudrail' } },
  ],
  deployments: [
    { id: 'deploy-1', serviceId: 'app-1', image: 'ghcr.io/example/storefront@sha256:abc', port: 8080, healthPath: '/health', status: 'active', error: '', logs: 'Starting up\nListening on :8080', createdAt: '2026-09-09T00:00:00Z', updatedAt: '2026-09-09T00:01:00Z' },
    { id: 'deploy-worker', serviceId: 'worker-1', image: 'ghcr.io/example/worker@sha256:abc', port: 80, healthPath: '/', status: 'active', error: '', logs: 'Waiting for jobs', createdAt: '2026-09-09T00:00:00Z', updatedAt: '2026-09-09T00:01:00Z' },
    { id: 'deploy-db', serviceId: 'db-1', image: 'postgres@sha256:def', port: 5432, healthPath: '/', status: 'active', error: '', logs: '', createdAt: '2026-09-09T00:00:00Z', updatedAt: '2026-09-09T00:01:00Z' },
  ],
  events: [{ id: 1, deploymentId: 'deploy-1', stage: 'active', message: 'Traffic switched to this release.', createdAt: '2026-09-09T00:01:00Z' }],
  actions: [],
};

const graph = {
  projectId: 'project-1', environment: 'production',
  resources: [
    { key: 'service:app-1', id: 'app-1', kind: 'service', name: 'storefront-api', status: 'active', workloadMode: 'web', sourceType: 'github', publicAddress: 'https://api.example.test', privateAddress: 'storefront-api.internal', position: { x: 130, y: 150 } },
    { key: 'service:worker-1', id: 'worker-1', kind: 'service', name: 'email-queue', status: 'active', workloadMode: 'worker', sourceType: 'image', privateAddress: 'email-queue.internal', position: { x: 130, y: 390 } },
    { key: 'service:db-1', id: 'db-1', kind: 'database', name: 'postgres', status: 'active', workloadMode: 'database', template: 'PostgreSQL', privateAddress: 'postgres.internal:5432', position: { x: 610, y: 150 } },
    { key: 'volume:volume-1', id: 'volume-1', kind: 'volume', name: 'postgres-data', status: 'attached', template: 'local', position: { x: 610, y: 380 } },
  ],
  links: [
    { id: 'reference-1', from: 'service:app-1', to: 'service:db-1', kind: 'reference', label: 'DATABASE_URL' },
    { id: 'attachment-1', from: 'service:db-1', to: 'volume:volume-1', kind: 'attachment', label: '/var/lib/postgresql/data' },
  ],
};

async function mockWorkspace(page: import('@playwright/test').Page, savedLayouts: unknown[]) {
  const canvas = structuredClone(graph);
  await page.route('**/*', async route => {
    const request = route.request();
    const url = new URL(request.url());
    if (url.pathname === '/auth/status') return route.fulfill({ json: { configured: true, email: 'owner@example.com' } });
    if (url.pathname === '/api/state') return route.fulfill({ json: state });
    if (url.pathname === '/api/node') return route.fulfill({ json: { enrolled: true, revoked: false, online: true, lastSeen: '2026-09-09T00:02:00Z', metrics: { memoryTotal: 2147483648, memoryAvailable: 1073741824, diskFree: 10737418240, diskTotal: 21474836480, cpuPercent: 8 } } });
    if (url.pathname.endsWith('/canvas/layout') && request.method() === 'PUT') {
      const body = request.postDataJSON() as {positions: {resourceKey: string; x: number; y: number}[]};
      savedLayouts.push(body);
      for (const position of body.positions) {
        const resource = canvas.resources.find(item => item.key === position.resourceKey);
        if (resource) resource.position = { x: position.x, y: position.y };
      }
      return route.fulfill({ json: { ok: true } });
    }
    if (url.pathname.endsWith('/canvas')) return route.fulfill({ json: canvas });
    if (url.pathname.endsWith('/metrics')) return route.fulfill({ json: { memoryBytes: 71303168, cpuPercent: 1.8, status: 'running' } });
    if (url.pathname === '/api/backups') return route.fulfill({ json: [] });
    if (url.pathname.endsWith('/variables')) return route.fulfill({ json: { names: [] } });
    if (url.pathname.startsWith('/api/')) return route.fulfill({ json: {} });
    return route.continue();
  });
}

test('canvas exposes resources, connections, creation and saved layout', async ({ page }) => {
  const savedLayouts: unknown[] = [];
  const errors: string[] = [];
  page.on('pageerror', error => errors.push(error.message));
  await mockWorkspace(page, savedLayouts);
  await page.goto('/');

  await expect(page.getByRole('region', { name: 'Project canvas' })).toBeVisible();
  await expect(page.getByRole('button', { name: 'storefront-api resource' })).toBeVisible();
  await expect(page.getByRole('button', { name: 'postgres resource' })).toBeVisible();
  await expect(page.getByRole('button', { name: 'postgres-data resource' })).toBeVisible();
  await expect(page.getByRole('button', { name: 'email-queue resource' })).toBeVisible();
  await expect(page.locator('.canvas-links path')).toHaveCount(3);
  await expect(page.locator('.detail-pane')).toHaveCount(0);

  await page.getByRole('button', { name: 'storefront-api resource' }).click();
  await expect(page.getByRole('region', { name: 'storefront-api details' })).toBeVisible();
  await expect.poll(() => new URL(page.url()).searchParams.get('resource')).toBe('service:app-1');
  await page.getByLabel('Close service details').click();

  await page.getByRole('button', { name: 'email-queue resource' }).click();
  await expect(page.getByRole('region', { name: 'email-queue details' })).toContainText('Route-free worker process is active');
  await page.getByLabel('Close service details').click();

  await page.getByRole('button', { name: 'postgres-data resource' }).click();
  const volume = page.getByRole('region', { name: 'postgres-data details' });
  await expect(volume).toBeVisible();
  await expect(volume.getByText('Persistent volume')).toBeVisible();
  await volume.getByRole('button', { name: /postgres/ }).click();
  await expect(page.getByRole('region', { name: 'postgres details' })).toBeVisible();
  await page.getByLabel('Close service details').click();

  await page.keyboard.press('Control+K');
  const palette = page.getByRole('dialog', { name: 'Add to your canvas' });
  await expect(palette).toBeVisible();
  await expect(palette.getByRole('button', { name: /GitHub Repository/ })).toBeEnabled();
  await expect(palette.getByRole('button', { name: /Background Worker/ })).toBeEnabled();
  await expect(palette.getByRole('button', { name: /Cron Job/ })).toBeEnabled();
  await expect(palette.getByRole('button', { name: /Redis/ })).toBeDisabled();
  await palette.getByRole('button', { name: /Background Worker/ }).click();
  await expect(page.getByRole('dialog', { name: 'Background Worker' })).toBeVisible();
  await expect(page.getByLabel('Resource name')).toBeVisible();
  await expect(page.getByLabel('Workload')).toHaveValue('worker');
  await expect(page.getByLabel('Source', { exact: true })).toHaveValue('empty');
  await page.keyboard.press('Escape');

  const node = page.getByRole('button', { name: 'storefront-api resource' });
  const box = await node.boundingBox();
  expect(box).not.toBeNull();
  await page.mouse.move(box!.x + 80, box!.y + 35);
  await page.mouse.down();
  await page.mouse.move(box!.x + 170, box!.y + 105, { steps: 5 });
  await page.mouse.up();
  await expect.poll(() => savedLayouts.length).toBe(1);
  expect(savedLayouts[0]).toMatchObject({ positions: [{ resourceKey: 'service:app-1' }] });

  await node.click();
  await page.reload();
  await expect(page.getByRole('region', { name: 'storefront-api details' })).toBeVisible();
  await expect(page.getByRole('button', { name: 'storefront-api resource' })).toHaveCSS('left', '220px');
  await page.getByLabel('Close service details').click();

  await page.locator('.canvas-viewport').click({ button: 'right', position: { x: 380, y: 480 } });
  await expect(page.getByRole('dialog', { name: 'Add to your canvas' })).toBeVisible();
  await page.getByLabel('Close resource palette').click();
  await page.getByLabel('Fit canvas').click();
  await page.screenshot({ path: screenshots + 'canvas-desktop.png', fullPage: true });
  expect(errors).toEqual([]);
});

test('canvas and resource drawer fit a phone viewport', async ({ page }) => {
  const errors: string[] = [];
  page.on('pageerror', error => errors.push(error.message));
  await mockWorkspace(page, []);
  await page.setViewportSize({ width: 390, height: 844 });
  await page.goto('/');
  await expect(page.getByRole('button', { name: 'storefront-api resource' })).toBeVisible();
  await page.getByRole('button', { name: 'postgres-data resource' }).click();
  await expect(page.getByRole('region', { name: 'postgres-data details' })).toBeVisible();
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBeTruthy();
  await page.screenshot({ path: screenshots + 'canvas-mobile.png', fullPage: true });
  expect(errors).toEqual([]);
});
