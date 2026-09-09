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
    { id: 'cron-1', projectId: 'project-1', name: 'nightly-cleanup', environment: 'production', host: 'cron.example.test', url: '', activeId: 'deploy-cron', desiredState: 'running', resourceKind: 'service', workloadMode: 'cron', template: '', cronSchedule: '0 2 * * *', cronNextRun: '2026-09-10T02:00:00Z', createdAt: '2026-09-09T00:00:00Z', settings: { kind: 'http', memoryMB: 256, cpuMillis: 1000, mountPath: '', volumeName: '', network: 'cloudrail' } },
    { id: 'db-1', projectId: 'project-1', name: 'postgres', environment: 'production', host: '', url: '', activeId: 'deploy-db', desiredState: 'running', resourceKind: 'database', workloadMode: 'web', template: 'postgres', createdAt: '2026-09-09T00:00:00Z', settings: { kind: 'postgres', memoryMB: 384, cpuMillis: 1000, mountPath: '/var/lib/postgresql/data', volumeName: 'postgres-data', network: 'cloudrail' } },
  ],
  deployments: [
    { id: 'deploy-1', serviceId: 'app-1', image: 'ghcr.io/example/storefront@sha256:abc', port: 8080, healthPath: '/health', status: 'active', error: '', logs: 'Starting up\nListening on :8080', createdAt: '2026-09-09T00:00:00Z', updatedAt: '2026-09-09T00:01:00Z' },
    { id: 'deploy-worker', serviceId: 'worker-1', image: 'ghcr.io/example/worker@sha256:abc', port: 80, healthPath: '/', status: 'active', error: '', logs: 'Waiting for jobs', createdAt: '2026-09-09T00:00:00Z', updatedAt: '2026-09-09T00:01:00Z' },
    { id: 'deploy-cron', serviceId: 'cron-1', image: 'ghcr.io/example/cleanup@sha256:abc', port: 80, healthPath: '/', status: 'active', error: '', logs: '', createdAt: '2026-09-09T00:00:00Z', updatedAt: '2026-09-09T00:01:00Z' },
    { id: 'deploy-db', serviceId: 'db-1', image: 'postgres@sha256:def', port: 5432, healthPath: '/', status: 'active', error: '', logs: '', createdAt: '2026-09-09T00:00:00Z', updatedAt: '2026-09-09T00:01:00Z' },
  ],
  events: [{ id: 1, deploymentId: 'deploy-1', stage: 'active', message: 'Traffic switched to this release.', createdAt: '2026-09-09T00:01:00Z' }],
  actions: [],
  cronRuns: [{ id: 'run-1', serviceId: 'cron-1', deploymentId: 'deploy-cron', scheduledFor: '2026-09-09T02:00:00Z', status: 'succeeded', exitCode: 0, logs: 'cleanup complete', error: '', startedAt: '2026-09-09T02:00:00Z', finishedAt: '2026-09-09T02:00:02Z' }],
};

const graph = {
  projectId: 'project-1', environment: 'production',
  resources: [
    { key: 'service:app-1', id: 'app-1', kind: 'service', name: 'storefront-api', status: 'active', workloadMode: 'web', sourceType: 'github', publicAddress: 'https://api.example.test', privateAddress: 'storefront-api.internal', position: { x: 130, y: 150 } },
    { key: 'service:worker-1', id: 'worker-1', kind: 'service', name: 'email-queue', status: 'active', workloadMode: 'worker', sourceType: 'image', privateAddress: 'email-queue.internal', position: { x: 130, y: 390 } },
    { key: 'service:cron-1', id: 'cron-1', kind: 'service', name: 'nightly-cleanup', status: 'active', workloadMode: 'cron', sourceType: 'image', privateAddress: 'nightly-cleanup.internal', position: { x: 410, y: 470 } },
    { key: 'service:db-1', id: 'db-1', kind: 'database', name: 'postgres', status: 'active', workloadMode: 'database', template: 'PostgreSQL', privateAddress: 'postgres.internal:5432', position: { x: 610, y: 150 } },
    { key: 'volume:volume-1', id: 'volume-1', kind: 'volume', name: 'postgres-data', status: 'attached', template: 'local', managedByTemplate: true, position: { x: 610, y: 380 } },
  ],
  links: [
    { id: 'reference-1', from: 'service:app-1', to: 'service:db-1', kind: 'reference', label: 'DATABASE_URL' },
    { id: 'attachment-1', from: 'service:db-1', to: 'volume:volume-1', kind: 'attachment', label: '/var/lib/postgresql/data' },
  ],
};

async function mockWorkspace(page: import('@playwright/test').Page, savedLayouts: unknown[], savedRuntime: unknown[] = [], savedCreates: unknown[] = [], savedAttachments: unknown[] = [], savedBucketActions: unknown[] = [], savedVariables: unknown[] = []) {
  const canvas = structuredClone(graph);
  const workspaceState = structuredClone(state);
  const variableState: Record<string, {names:string[];variables:{name:string;kind:string;value?:string;targetServiceId?:string;targetServiceName?:string;targetVariable?:string}[]}> = {
    'app-1': { names: ['DATABASE_URL','PUBLIC_ORIGIN','SESSION_SECRET'], variables: [
      { name: 'DATABASE_URL', kind: 'reference', targetServiceId: 'db-1', targetServiceName: 'postgres', targetVariable: 'DATABASE_URL' },
      { name: 'PUBLIC_ORIGIN', kind: 'plain', value: 'https://api.example.test' },
      { name: 'SESSION_SECRET', kind: 'secret' },
    ] },
    'worker-1': { names: ['API_ORIGIN'], variables: [{ name: 'API_ORIGIN', kind: 'plain', value: 'http://email-queue.internal' }] },
    'db-1': { names: ['DATABASE_URL'], variables: [{ name: 'DATABASE_URL', kind: 'secret' }] },
  };
  await page.route('**/*', async route => {
    const request = route.request();
    const url = new URL(request.url());
    if (url.pathname === '/auth/status') return route.fulfill({ json: { configured: true, email: 'owner@example.com' } });
    if (url.pathname === '/api/state') return route.fulfill({ json: workspaceState });
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
    if (url.pathname.endsWith('/runtime') && request.method() === 'PUT') {
      savedRuntime.push(request.postDataJSON());
      return route.fulfill({ json: request.postDataJSON() });
    }
    if (url.pathname === '/api/projects/project-1/databases' && request.method() === 'POST') {
      const body = request.postDataJSON() as {name:string;environment:string;template:string};
      savedCreates.push(body);
      const config = body.template === 'mysql'
        ? { id: 'mysql-1', version: '8.4.7', port: 3306, mountPath: '/var/lib/mysql', memoryMB: 512, x: 1120 }
        : body.template === 'mongo'
          ? { id: 'mongo-1', version: '8.0.29', port: 27017, mountPath: '/data/db', memoryMB: 512, x: 1350 }
          : { id: 'redis-1', version: '8.2.2', port: 6379, mountPath: '/data', memoryMB: 128, x: 890 };
      const { id, version, port, mountPath } = config;
      const service = { id, projectId: 'project-1', name: body.name, environment: body.environment, host: '', url: '', activeId: '', desiredState: 'running', resourceKind: 'database', workloadMode: 'web', template: body.template, templateVersion: version, createdAt: '2026-09-09T00:00:00Z', settings: { kind: body.template, memoryMB: config.memoryMB, cpuMillis: 1000, mountPath, volumeName: id + '-data', network: 'cloudrail' } };
      workspaceState.services.push(service);
      canvas.resources.push({ key: 'service:' + id, id, kind: 'database', name: body.name, status: 'queued', workloadMode: 'web', sourceType: 'template', template: body.template, templateVersion: version, privateAddress: `db-${id}:${port}`, position: { x: config.x, y: 340 } });
      return route.fulfill({ status: 201, json: service });
    }
    if (url.pathname === '/api/projects/project-1/volumes' && request.method() === 'POST') {
      const body = request.postDataJSON() as {name:string;environment:string};
      savedCreates.push(body);
      canvas.resources.push({ key: 'volume:volume-2', id: 'volume-2', kind: 'volume', name: body.name, status: 'available', position: { x: 900, y: 560 } });
      return route.fulfill({ status: 201, json: { id: 'volume-2', projectId: 'project-1', ...body, managedByTemplate: false } });
    }
    if (url.pathname === '/api/projects/project-1/buckets' && request.method() === 'POST') {
      const body = request.postDataJSON() as {name:string;environment:string;endpoint:string;region:string;bucketName:string;forcePathStyle:boolean};
      savedBucketActions.push({ kind: 'create', ...body });
      const bucket = { key: 'bucket:bucket-1', id: 'bucket-1', kind: 'bucket', name: body.name, status: 'available', template: 's3', endpoint: body.endpoint, region: body.region, bucketName: body.bucketName, forcePathStyle: body.forcePathStyle, credentialVersion: 1, position: { x: 1100, y: 330 } };
      canvas.resources.push(bucket);
      return route.fulfill({ status: 201, json: { id: bucket.id, projectId: 'project-1', ...body, provider: 's3', credentialVersion: 1 } });
    }
    if (url.pathname === '/api/buckets/bucket-1/bindings/app-1' && request.method() === 'PUT') {
      const body = request.postDataJSON() as {variablePrefix:string};
      savedBucketActions.push({ kind: 'bind', ...body });
      const bucket = canvas.resources.find(item => item.id === 'bucket-1');
      if (bucket) bucket.status = 'connected';
      canvas.links.push({ id: 'bucket-binding-1', from: 'service:app-1', to: 'bucket:bucket-1', kind: 'bucket-binding', label: body.variablePrefix + '_*' });
      return route.fulfill({ json: { ok: true } });
    }
    if (url.pathname === '/api/buckets/bucket-1/credentials' && request.method() === 'PUT') {
      savedBucketActions.push({ kind: 'rotate', ...request.postDataJSON() });
      const bucket = canvas.resources.find(item => item.id === 'bucket-1');
      if (bucket) bucket.credentialVersion = 2;
      return route.fulfill({ json: { id: 'bucket-1', credentialVersion: 2 } });
    }
    if (url.pathname === '/api/volumes/volume-2/attachment' && request.method() === 'PUT') {
      const body = request.postDataJSON() as {serviceId:string;mountPath:string};
      savedAttachments.push(body);
      const volume = canvas.resources.find(item => item.id === 'volume-2');
      if (volume) volume.status = 'attached';
      canvas.links.push({ id: 'attachment-2', from: 'volume:volume-2', to: 'service:' + body.serviceId, kind: 'volume-attachment', label: body.mountPath });
      return route.fulfill({ json: { ok: true } });
    }
    if (url.pathname === '/api/backups') return route.fulfill({ json: [] });
    const collection = url.pathname.match(/^\/api\/services\/([^/]+)\/variables$/);
    if (collection && request.method() === 'GET') return route.fulfill({ json: variableState[collection[1]] || { names: [], variables: [] } });
    const rename = url.pathname.match(/^\/api\/services\/([^/]+)\/variables\/([^/]+)\/rename$/);
    if (rename && request.method() === 'POST') {
      const oldName = decodeURIComponent(rename[2]); const body = request.postDataJSON() as {name:string}; const values = variableState[rename[1]];
      const item = values.variables.find(variable => variable.name === oldName);
      if (item) item.name = body.name;
      values.names = values.variables.map(variable => variable.name);
      for (const link of canvas.links) if (link.id === `reference:${rename[1]}:${oldName}`) { link.id = `reference:${rename[1]}:${body.name}`; link.label = `${body.name} → ${item?.targetVariable}`; }
      savedVariables.push({ action: 'rename', serviceId: rename[1], oldName, ...body });
      return route.fulfill({ json: { ok: true } });
    }
    const variable = url.pathname.match(/^\/api\/services\/([^/]+)\/variables\/([^/]+)$/);
    if (variable && request.method() === 'PUT') {
      const serviceId = variable[1], name = decodeURIComponent(variable[2]); const body = request.postDataJSON() as {kind:string;value?:string;targetServiceId?:string;targetVariable?:string};
      const values = variableState[serviceId] || (variableState[serviceId] = { names: [], variables: [] });
      values.variables = values.variables.filter(item => item.name !== name);
      const targetService = workspaceState.services.find(item => item.id === body.targetServiceId);
      values.variables.push({ name, kind: body.kind, value: body.kind === 'plain' ? body.value : undefined, targetServiceId: body.targetServiceId, targetServiceName: targetService?.name, targetVariable: body.targetVariable });
      values.variables.sort((a,b) => a.name.localeCompare(b.name)); values.names = values.variables.map(item => item.name);
      if (body.kind === 'reference') {
        canvas.links = canvas.links.filter(item => item.id !== `reference:${serviceId}:${name}`);
        canvas.links.push({ id: `reference:${serviceId}:${name}`, from: `service:${serviceId}`, to: `service:${body.targetServiceId}`, kind: 'variable-reference', label: `${name} → ${body.targetVariable}` });
      }
      savedVariables.push({ action: 'save', serviceId, name, ...body });
      return route.fulfill({ json: { ok: true } });
    }
    if (variable && request.method() === 'DELETE') {
      const serviceId = variable[1], name = decodeURIComponent(variable[2]); const values = variableState[serviceId];
      values.variables = values.variables.filter(item => item.name !== name); values.names = values.variables.map(item => item.name);
      canvas.links = canvas.links.filter(item => item.id !== `reference:${serviceId}:${name}`);
      savedVariables.push({ action: 'delete', serviceId, name });
      return route.fulfill({ json: { ok: true } });
    }
    if (url.pathname.startsWith('/api/')) return route.fulfill({ json: {} });
    return route.continue();
  });
}

test('canvas exposes resources, connections, creation and saved layout', async ({ page }) => {
  const savedLayouts: unknown[] = [];
  const savedRuntime: unknown[] = [];
  const savedCreates: unknown[] = [];
  const savedAttachments: unknown[] = [];
  const savedBucketActions: unknown[] = [];
  const savedVariables: unknown[] = [];
  const errors: string[] = [];
  page.on('pageerror', error => errors.push(error.message));
  await mockWorkspace(page, savedLayouts, savedRuntime, savedCreates, savedAttachments, savedBucketActions, savedVariables);
  await page.goto('/');

  await expect(page.getByRole('region', { name: 'Project canvas' })).toBeVisible();
  await expect(page.getByRole('button', { name: 'storefront-api resource' })).toBeVisible();
  await expect(page.getByRole('button', { name: 'postgres resource' })).toBeVisible();
  await expect(page.getByRole('button', { name: 'postgres-data resource' })).toBeVisible();
  await expect(page.getByRole('button', { name: 'email-queue resource' })).toBeVisible();
  await expect(page.getByRole('button', { name: 'nightly-cleanup resource' })).toBeVisible();
  await expect(page.locator('.canvas-links path')).toHaveCount(3);
  await expect(page.locator('.detail-pane')).toHaveCount(0);

  await page.getByRole('button', { name: 'storefront-api resource' }).click();
  await expect(page.getByRole('region', { name: 'storefront-api details' })).toBeVisible();
  await expect.poll(() => new URL(page.url()).searchParams.get('resource')).toBe('service:app-1');
  await page.getByRole('tab', { name: 'Variables' }).click();
  await expect(page.getByText('SESSION_SECRET')).toBeVisible();
  await expect(page.getByText('••••••••')).toBeVisible();
  await expect(page.getByText('https://api.example.test')).toBeVisible();
  await page.getByLabel('Variable name').fill('WORKER_URL');
  await page.getByLabel('Value type').selectOption('reference');
  await page.getByLabel('Target service').selectOption('worker-1');
  await page.getByLabel('Target variable').selectOption('API_ORIGIN');
  await page.getByRole('button', { name: 'Connect reference' }).click();
  await expect(page.getByText('Reference connected. The canvas link and next deployment will use it.')).toBeVisible();
  await expect(page.getByText('email-queue · API_ORIGIN')).toBeVisible();
  await expect.poll(async()=>page.locator('.canvas-links path').count()).toBe(4);
  await page.getByRole('button', { name: 'Rename WORKER_URL' }).click();
  await page.getByRole('textbox', { name: 'Rename WORKER_URL' }).fill('INTERNAL_API_URL');
  await page.getByRole('button', { name: 'Rename', exact: true }).click();
  await expect(page.getByText('Variable renamed. Connected references were updated.')).toBeVisible();
  await expect(page.getByText('INTERNAL_API_URL')).toBeVisible();
  expect(savedVariables).toEqual([
    { action: 'save', serviceId: 'app-1', name: 'WORKER_URL', kind: 'reference', targetServiceId: 'worker-1', targetVariable: 'API_ORIGIN' },
    { action: 'rename', serviceId: 'app-1', oldName: 'WORKER_URL', name: 'INTERNAL_API_URL' },
  ]);
  await page.getByLabel('Close service details').click();

  await page.getByRole('button', { name: 'nightly-cleanup resource' }).click();
  await expect(page.getByRole('region', { name: 'nightly-cleanup details' })).toContainText('0 2 * * * UTC');
  await page.getByRole('tab', { name: 'Settings' }).click();
  await expect(page.getByLabel('Cron expression')).toHaveValue('0 2 * * *');
  await expect(page.getByText('cleanup complete')).toBeVisible();
  await page.screenshot({ path: screenshots + 'cron-settings-desktop.png', fullPage: true });
  await page.getByLabel('Close service details').click();

  await page.getByRole('button', { name: 'email-queue resource' }).click();
  await expect(page.getByRole('region', { name: 'email-queue details' })).toContainText('Route-free worker process is active');
  await page.getByRole('tab', { name: 'Settings' }).click();
  await page.getByLabel('Start command override').fill('node worker.js');
  await page.getByLabel('Pre-deploy command').fill('node migrate.js');
  await expect(page.getByLabel('Pre-deploy timeout (seconds)')).toHaveValue('300');
  await page.getByLabel('Restart policy').selectOption('always');
  await expect(page.getByLabel('Maximum retries')).toBeDisabled();
  await page.getByRole('button', { name: 'Save runtime settings' }).click();
  await expect(page.getByText('Runtime settings saved. Deploy again to apply.')).toBeVisible();
  expect(savedRuntime).toEqual([{ startCommand: 'node worker.js', preDeployCommand: 'node migrate.js', preDeployTimeoutSeconds: 300, restartPolicy: 'always', restartMaxRetries: 0 }]);
  await page.screenshot({ path: screenshots + 'runtime-settings-desktop.png', fullPage: true });
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
  await expect(palette.getByRole('button', { name: /Redis/ })).toBeEnabled();
  await palette.getByRole('button', { name: /Redis/ }).click();
  const redisDialog = page.getByRole('dialog', { name: 'Redis' });
  await expect(redisDialog.getByText('Redis data service')).toBeVisible();
  await redisDialog.getByLabel('Resource name').fill('session-cache');
  await redisDialog.getByRole('button', { name: 'Create resource', exact: true }).click();
  await expect(page.getByRole('button', { name: 'session-cache resource' })).toBeVisible();
  await expect(page.getByRole('region', { name: 'session-cache details' })).toContainText('Private Redis · db-redis-1:6379');
  await page.getByRole('tab', { name: 'Settings' }).click();
  await expect(page.getByText('Stop this resource before backing up or restoring its volume.')).toBeVisible();
  await expect(page.getByRole('button', { name: 'Create backup', exact: true })).toBeDisabled();
  expect(savedCreates).toEqual([{ name: 'session-cache', environment: 'production', template: 'redis' }]);
  await page.getByLabel('Close service details').click();

  await page.keyboard.press('Control+K');
  const mysqlOption = page.getByRole('dialog', { name: 'Add to your canvas' }).getByRole('button', { name: /MySQL/ });
  await expect(mysqlOption).toBeEnabled();
  await mysqlOption.click();
  const mysqlDialog = page.getByRole('dialog', { name: 'MySQL' });
  await expect(mysqlDialog.getByText('MySQL database')).toBeVisible();
  await mysqlDialog.getByLabel('Resource name').fill('orders-db');
  await mysqlDialog.getByRole('button', { name: 'Create resource', exact: true }).click();
  await expect(page.getByRole('button', { name: 'orders-db resource' })).toBeVisible();
  await expect(page.getByRole('region', { name: 'orders-db details' })).toContainText('Private MySQL · db-mysql-1:3306');
  await page.getByRole('tab', { name: 'Settings' }).click();
  await expect(page.getByLabel('Connection variable name')).toHaveValue('MYSQL_URL');
  await expect(page.getByText('MySQL logical backups run while the database is running.')).toBeVisible();
  expect(savedCreates.at(-1)).toEqual({ name: 'orders-db', environment: 'production', template: 'mysql' });
  await page.getByLabel('Close service details').click();

  await page.keyboard.press('Control+K');
  const mongoOption = page.getByRole('dialog', { name: 'Add to your canvas' }).getByRole('button', { name: /MongoDB/ });
  await expect(mongoOption).toBeEnabled();
  await mongoOption.click();
  const mongoDialog = page.getByRole('dialog', { name: 'MongoDB' });
  await expect(mongoDialog.getByText('MongoDB database')).toBeVisible();
  await mongoDialog.getByLabel('Resource name').fill('catalog-db');
  await mongoDialog.getByRole('button', { name: 'Create resource', exact: true }).click();
  await expect(page.getByRole('button', { name: 'catalog-db resource' })).toBeVisible();
  await expect(page.getByRole('region', { name: 'catalog-db details' })).toContainText('Private MongoDB · db-mongo-1:27017');
  await page.getByRole('tab', { name: 'Settings' }).click();
  await expect(page.getByLabel('Connection variable name')).toHaveValue('MONGO_URL');
  await expect(page.getByText('MongoDB logical backups run while the database is running.')).toBeVisible();
  expect(savedCreates.at(-1)).toEqual({ name: 'catalog-db', environment: 'production', template: 'mongo' });
  await page.getByLabel('Close service details').click();

  await page.keyboard.press('Control+K');
  await page.getByRole('dialog', { name: 'Add to your canvas' }).getByRole('button', { name: /^Volume/ }).click();
  const volumeDialog = page.getByRole('dialog', { name: 'Persistent Volume' });
  await volumeDialog.getByLabel('Resource name').fill('uploads');
  await volumeDialog.getByRole('button', { name: 'Create resource', exact: true }).click();
  await expect(page.getByRole('button', { name: 'uploads resource' })).toBeVisible();
  const volumeDrawer = page.getByRole('region', { name: 'uploads details' });
  await volumeDrawer.getByLabel('Application').selectOption('worker-1');
  await volumeDrawer.getByLabel('Mount path').fill('/uploads');
  await volumeDrawer.getByRole('button', { name: 'Attach volume' }).click();
  await expect(volumeDrawer.getByText('Volume attached. Deploy the application to apply it.')).toBeVisible();
  expect(savedCreates.at(-1)).toEqual({ name: 'uploads', environment: 'production' });
  expect(savedAttachments).toEqual([{ serviceId: 'worker-1', mountPath: '/uploads' }]);
  await page.getByLabel('Close resource details').click();

  await page.keyboard.press('Control+K');
  await page.getByRole('dialog', { name: 'Add to your canvas' }).getByRole('button', { name: /S3-compatible Bucket/ }).click();
  const bucketDialog = page.getByRole('dialog', { name: 'S3-compatible Bucket' });
  await bucketDialog.getByLabel('Resource name').fill('uploads-bucket');
  await bucketDialog.getByLabel('S3 endpoint').fill('http://minio:9000');
  await bucketDialog.getByLabel('Remote bucket name').fill('app-uploads');
  await bucketDialog.getByLabel('Access key ID').fill('browser-access');
  await bucketDialog.getByLabel('Secret access key').fill('browser-secret-key');
  await bucketDialog.getByRole('button', { name: 'Create resource', exact: true }).click();
  const bucketDrawer = page.getByRole('region', { name: 'uploads-bucket details' });
  await expect(bucketDrawer).toContainText('http://minio:9000');
  await bucketDrawer.getByLabel('Application').selectOption('app-1');
  await bucketDrawer.getByLabel('Variable prefix').fill('uploads');
  await bucketDrawer.getByRole('button', { name: 'Connect application' }).click();
  await expect(bucketDrawer.getByText('Bucket variables connected. Redeploy the application to apply them.')).toBeVisible();
  await bucketDrawer.getByLabel('Access key ID').fill('rotated-access');
  await bucketDrawer.getByLabel('Secret access key').fill('rotated-secret-key');
  await bucketDrawer.getByRole('button', { name: 'Rotate credentials' }).click();
  await expect(bucketDrawer.getByText('Credentials rotated. Redeploy connected applications to use version 2.')).toBeVisible();
  await page.screenshot({ path: screenshots + 'bucket-settings-desktop.png', fullPage: true });
  expect(savedBucketActions).toEqual([
    { kind: 'create', name: 'uploads-bucket', environment: 'production', endpoint: 'http://minio:9000', region: 'us-east-1', bucketName: 'app-uploads', accessKeyId: 'browser-access', secretAccessKey: 'browser-secret-key', forcePathStyle: true },
    { kind: 'bind', variablePrefix: 'UPLOADS' },
    { kind: 'rotate', accessKeyId: 'rotated-access', secretAccessKey: 'rotated-secret-key' },
  ]);
  await page.getByLabel('Close resource details').click();

  await page.keyboard.press('Control+K');
  await page.getByRole('dialog', { name: 'Add to your canvas' }).getByRole('button', { name: /Background Worker/ }).click();
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
