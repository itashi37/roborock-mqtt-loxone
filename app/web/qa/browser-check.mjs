// Isolated browser QA. Every API response and SSE event is local fixture data;
// no request from this harness can reach a robot, Miniserver or updater.
import assert from 'node:assert/strict';
import { createRequire } from 'node:module';
import { mkdir, writeFile } from 'node:fs/promises';
import { dirname, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { spawn } from 'node:child_process';

const webRoot = resolve(dirname(fileURLToPath(import.meta.url)), '..');
const require = createRequire(process.env.QA_NODE_MODULES ? join(process.env.QA_NODE_MODULES, 'qa-require.cjs') : import.meta.url);
const { chromium } = require('playwright');
const origin = process.env.QA_ORIGIN || 'http://127.0.0.1:5173';
const output = resolve(process.env.QA_OUTPUT || join(webRoot, 'qa', 'artifacts'));
const now = new Date().toISOString();
const timestamp = Math.floor(Date.now() / 1000);
const slug = 'roborock-qrevo-curv';
const secondSlug = 'roborock-etage';
const exactInputs = ['RR_roborock_qrevo_curv_running', 'Existing custom room VI', 'RR_roborock_qrevo_curv_battery'];
const defaultStatus = { state: 'charging', battery: 86, fan_speed: 'balanced', mop_mode: 'standard', water_box: 'moderate', clean_time: 0, clean_area: 0, error_code: 0, error: '', in_cleaning: false, consumables: { main_brush_work_time: 0, side_brush_work_time: 0, filter_work_time: 0, sensor_dirty_time: 0, dust_collection_work_times: 0 }, consumable_percents: { main_brush: 94, side_brush: 88, filter: 79, sensor: 100, dust_collection: 96 } };
const devices = [
  { slug, name: 'Roborock Qrevo Curv', model: 'roborock.vacuum.a75', online: true, status: defaultStatus },
  { slug: secondSlug, name: 'Roborock Étage', model: 'roborock.vacuum.s8', online: true, status: { ...defaultStatus, state: 'cleaning', battery: 72, in_cleaning: true, clean_time: 824, clean_area: 23800000, program: 'full' } },
];
const capability = { supported: true, source: 'observed', confidence: 'confirmed', last_checked: now };
const capabilities = Object.fromEntries(['rooms', 'scenes', 'fan', 'mop', 'water', 'locate', 'dock', 'dock_empty', 'mop_wash', 'mop_dry', 'stop'].map(key => [key, capability]));
const healthFor = robot => ({ slug: robot.slug, online: true, in_error: false, error_code: 0, dock_state: robot.status.state, last_poll_attempt: now, last_poll_success: now, last_communication: now, status_latency_ms: 42, consecutive_failures: 0, backoff_seconds: 0 });
const robots = devices.map((device, index) => ({
  slug: device.slug, name: device.name, model: device.model, online: true, mqtt_enabled: false, direct_enabled: true,
  core: { online: 1, state: device.status.state, battery: device.status.battery, current_room_id: index ? 18 : 0, current_room_name: index ? 'Chambre' : '', error_code: 0, last_seen: timestamp },
  topics: Object.fromEntries(['core', 'activity', 'command', 'last_command'].map(field => [field, `loxone/roborock/${device.slug}/${field}`])),
  rooms: [{ id: 16, roborock_name: 'Cuisine', effective_name: 'Cuisine', conflict: false, command: 'clean_room_id:16' }, { id: 18, roborock_name: 'Salon', effective_name: 'Salon', conflict: false, command: 'clean_room_id:18' }],
  scenes: [{ id: 42, name: 'Après les repas', command: 'scene_id:42' }],
  diagnostics: { recent: [{ type: 'event', ts: timestamp - 15, event: 'cleaning_completed' }], last_activity: { type: 'event', ts: timestamp - 15, event: 'cleaning_completed' } },
  capabilities, health: healthFor(device),
  direct_inputs: [
    { name: index ? 'RR_roborock_etage_running' : exactInputs[0], field: 'running', kind: 'digital' },
    { name: index ? 'RR_roborock_etage_current_room_id' : exactInputs[1], field: 'current_room_id', kind: 'analog' },
    { name: index ? 'RR_roborock_etage_battery' : exactInputs[2], field: 'battery', kind: 'analog' },
    { name: `RR_${device.slug.replaceAll('-', '_')}_error_text`, field: 'error_text', kind: 'text' },
    { name: `RR_${device.slug.replaceAll('-', '_')}_dock_error_status`, field: 'dock_error_status', kind: 'analog' },
  ],
  direct_outputs: [
    ...['start', 'pause', 'dock'].map(command => ({ name: command, method: 'POST', path: `/api/loxone/direct/v1/devices/${device.slug}/commands/${command}`, command, level: 'safe' })),
    { name: 'Cuisine', method: 'POST', path: `/api/loxone/direct/v1/devices/${device.slug}/commands/rooms/16`, command: 'clean_room_id:16', level: 'safe' },
    { name: 'Après les repas', method: 'POST', path: `/api/loxone/direct/v1/devices/${device.slug}/commands/scenes/42`, command: 'scene_id:42', level: 'safe' },
  ],
}));
const fleet = { health: 'healthy', updated_at: now, robots: 2, online: 2, in_error: 0, poll_failures: 0, devices: devices.map(healthFor) };
const integration = {
  project: 'roborock-mqtt-loxone', upstream: 'mqtt-home/roborock-mqtt', enabled: false, bridge_started: true,
  topic: 'loxone/roborock', subscription_limit: 16, subscriptions_per_robot: 2, subscriptions_required: 0, exceeds_limit: false,
  direct_enabled: true, direct_api_username: 'existing-loxone-user', direct_token_configured: true,
  direct_diagnostics: { last_transmission: now, inputs: [{ robot: slug, field: 'running', input: exactInputs[0], kind: 'digital', last_value: '0', last_success: now, consecutive_retries: 0 }] },
  health_inputs: [{ name: 'RR_bridge_alive', field: 'bridge_alive', kind: 'digital' }], robots, fleet,
  template_status: { native_generation: false, format_verified: false, reason: 'CSV integration pack', required_samples: [] },
};
const setup = {
  setup_complete: true, authenticated: true, roborock_username: 'maison@example.test',
  mqtt: { enabled: false, url: 'tcp://existing-broker:1883', retain: true, topic: 'home/roborock', qos: 1, username: '', tls: false, password_configured: false },
  loxone: {
    enabled: false, topic: 'loxone/roborock', command_debounce_ms: 4500, command_timeout_seconds: 180,
    devices: { [slug]: { mqtt: false, direct: true }, [secondSlug]: { mqtt: false, direct: true } },
    direct: { enabled: true, scheme: 'http', host: '192.0.2.20', port: 80, username: 'existing-miniserver-user', timeout_seconds: 8, max_retries: 4, retry_delay_ms: 800, input_prefix: 'RR', inputs: { [slug]: { current_room_id: exactInputs[1] } }, api_username: 'existing-loxone-user', allowed_cidrs: ['192.0.2.0/24'], allow_get_commands: false, rate_limit_per_minute: 70, password_configured: true, api_token_configured: true },
  },
  mqtt_diagnostics: { enabled: false, connected: false, subscriptions: 0 },
  direct_diagnostics: { queued: 0, last_success_at: now, last_transmission: now },
};
const updateSettings = { version: 1, mode: 'notify', channel: 'stable', window_start: '03:00', window_end: '05:00', delay_hours: 24, allowed_days: [0, 1, 2, 3, 4, 5, 6], prevent_robot_active: true, prevent_cleaning: true, prevent_command_in_progress: true, allow_edge_automatic: false };
const update = { channel: 'stable', current_version: '1.7.0', latest_version: '1.7.1', available: true, artifact_ready: true, artifact_status: 'ready', checked_at: now, published_at: now, release_notes: 'Améliorations de la passerelle Loxone.', release_url: 'https://example.test/release' };
const system = { product: 'Roborock Bridge', version: '1.7.0', git_commit: 'fixture', build_time: now, go_version: 'go1.26.2', architecture: 'linux/amd64', channel: 'stable', uptime_seconds: 18240, started_at: now, last_restart: now, health: { status: 'healthy', live: true, ready: true, uptime_seconds: 18240, components: {} }, data_volume: { path: '/data', writable: true, free_bytes: 128000000000 }, transports: { direct: { enabled: true, connected: true, last_success: now }, mqtt: { enabled: false, connected: false } }, update, update_settings: updateSettings, auto_update: { last_check: now, last_decision: 'notification_only', guard: { robot_active: true, cleaning: true, commands_in_flight: 0 } }, supervisor: { kind: 'docker', data_dir: '/data', log_mode: 'stdout', restart_supported: true } };

await mkdir(output, { recursive: true });
let vite;
let browser;
let serverOutput = '';
const errors = [];
const warnings = [];
const apiRequests = [];
const screenshots = [];
const result = { screenshots, apiRequests, errors, warnings, checks: [] };
let fixtureSetup = setup;
let failSetup = false;
let failRequestCode = true;
let expectedNetworkError = false;
async function waitForServer() {
  for (let attempt = 0; attempt < 600; attempt++) {
    try { if ((await fetch(origin, { signal: AbortSignal.timeout(2000) })).ok) return; } catch {}
    await new Promise(resolve => setTimeout(resolve, 100));
  }
  throw new Error(`Vite did not start: ${serverOutput}`);
}
async function capture(page, name) {
  await page.screenshot({ path: join(output, `${name}.png`), fullPage: true, animations: 'disabled' });
  screenshots.push(`${name}.png`);
  const overflow = await page.evaluate(() => ({ document: document.documentElement.scrollWidth, viewport: window.innerWidth }));
  assert.ok(overflow.document <= overflow.viewport + 1, `${name}: horizontal overflow ${overflow.document} > ${overflow.viewport}`);
}

try {
  if (process.env.QA_START_SERVER !== '0') {
    vite = spawn(process.execPath, [join(webRoot, 'node_modules/vite/bin/vite.js'), '--host', '127.0.0.1', '--port', '5173', '--strictPort'], { cwd: webRoot, stdio: ['ignore', 'pipe', 'pipe'] });
    vite.stdout.on('data', chunk => { serverOutput += chunk; });
    vite.stderr.on('data', chunk => { serverOutput += chunk; });
  }
  await waitForServer();
  browser = await chromium.launch({ headless: true, executablePath: process.env.QA_CHROME_PATH });
  const context = await browser.newContext({ viewport: { width: 1440, height: 1000 }, locale: 'fr-FR', reducedMotion: 'reduce' });
  await context.addInitScript(({ devices }) => {
    const streams = new Set();
    class FixtureEventSource extends EventTarget {
      static CONNECTING = 0; static OPEN = 1; static CLOSED = 2;
      CONNECTING = 0; OPEN = 1; CLOSED = 2; readyState = 0;
      onopen = null; onmessage = null; onerror = null;
      constructor(url) {
        super(); this.url = url; streams.add(this);
        this.timer = setTimeout(() => {
          if (this.readyState === 2) return;
          this.readyState = 1; this.onopen?.(new Event('open'));
          for (const device of devices) this.onmessage?.(new MessageEvent('message', { data: JSON.stringify({ device: device.slug, ...device.status }) }));
        }, 30);
      }
      close() { this.readyState = 2; clearTimeout(this.timer); streams.delete(this); }
    }
    window.EventSource = FixtureEventSource;
    window.__qaEmitStatus = status => { for (const stream of streams) if (stream.readyState === 1) stream.onmessage?.(new MessageEvent('message', { data: JSON.stringify(status) })); };
  }, { devices });
  await context.route('**/*', async route => {
    const request = route.request();
    const url = new URL(request.url());
    if (!url.pathname.startsWith('/api/')) {
      if (url.origin === origin) return route.continue();
      errors.push(`Blocked unexpected external request: ${request.url()}`);
      return route.fulfill({ status: 200, body: '' });
    }
    const method = request.method();
    const path = url.pathname;
    apiRequests.push({ method, path, body: request.postDataJSON() });
    let body;
    if (method === 'OPTIONS') return route.fulfill({ status: 204, headers: { 'Access-Control-Allow-Origin': '*', 'Access-Control-Allow-Methods': '*', 'Access-Control-Allow-Headers': '*' } });
    if (method === 'GET') {
      if (path === '/api/setup/status') {
        if (failSetup) return route.fulfill({ status: 503, contentType: 'application/json', headers: { 'Access-Control-Allow-Origin': '*' }, body: JSON.stringify({ error: 'Fixture connection unavailable' }) });
        body = fixtureSetup;
      }
      else if (path === '/api/auth/status') body = { authenticated: fixtureSetup.authenticated, user: fixtureSetup.roborock_username, devices: fixtureSetup.authenticated ? 2 : 0 };
      else if (path === '/api/devices') body = devices;
      else if (path === '/api/loxone/integration') body = integration;
      else if (path === '/api/system/status') body = system;
      else if (path === '/api/system/updates/operation') body = { stage: 'idle' };
      else if (path === '/api/fleet/health') body = fleet;
      else if (/^\/api\/devices\/[^/]+\/scenes$/.test(path)) body = [{ id: 42, name: 'Après les repas' }];
      else if (/^\/api\/devices\/[^/]+\/advanced-diagnostics$/.test(path)) body = { diagnostics: { method: 'fixture', collected_at: now, fields: {} }, capabilities };
    } else if (method === 'POST' && /^\/api\/loxone\/devices\/[^/]+\/command$/.test(path)) body = { status: 'published', topic: `loxone/roborock/${slug}/command` };
    else if (method === 'POST' && path === '/api/system/updates/check') body = update;
    else if (method === 'PUT' && path === '/api/setup/settings') body = setup;
    else if (method === 'POST' && path === '/api/setup/direct/test') body = { ok: true };
    else if (method === 'POST' && path === '/api/loxone/direct/resend') body = { ok: true };
    else if (method === 'POST' && path === '/api/auth/request-code') {
      if (failRequestCode) return route.fulfill({ status: 503, contentType: 'application/json', headers: { 'Access-Control-Allow-Origin': '*' }, body: JSON.stringify({ error: 'Connexion momentanément indisponible.' }) });
      body = { status: 'success' };
    }
    else if (method === 'POST' && path === '/api/auth/login') {
      fixtureSetup.authenticated = true;
      fixtureSetup.setup_complete = true;
      fixtureSetup.roborock_username = 'test@example.test';
      fixtureSetup.loxone.direct.enabled = false;
      body = { authenticated: true, devices: 2 };
    }
    if (body === undefined) {
      errors.push(`Unexpected API request (blocked): ${method} ${path}`);
      body = { error: `Unmocked QA request: ${method} ${path}` };
    }
    return route.fulfill({ status: errors.at(-1)?.includes(`${method} ${path}`) ? 501 : method === 'POST' && path.endsWith('/command') ? 202 : 200, contentType: 'application/json', headers: { 'Access-Control-Allow-Origin': '*' }, body: JSON.stringify(body) });
  });
  const page = await context.newPage();
  const listen = page => {
    page.on('pageerror', error => errors.push(error.stack || error.message));
    page.on('console', message => { if (message.type() === 'error' && !(expectedNetworkError && message.text().includes('Failed to load resource'))) errors.push(message.text()); else if (message.type() === 'warning') warnings.push(message.text()); });
  };
  listen(page);
  await page.goto(origin);
  await page.getByRole('heading', { name: 'Mes robots.' }).waitFor();
  await page.getByRole('heading', { name: devices[1].name, exact: true }).waitFor();
  await page.getByText('Opérationnelle', { exact: true }).waitFor();
  await capture(page, 'desktop-dashboard');
  result.checks.push('Two robots and healthy Direct-only connection rendered on dashboard');

  await page.goto(`${origin}/loxone`);
  await page.getByText(exactInputs[0], { exact: true }).waitFor();
  assert.ok(await page.getByRole('button', { name: 'Télécharger le dossier', exact: true }).isEnabled(), 'Initial robot export selection was lost');
  for (const value of exactInputs) assert.ok(await page.getByText(value, { exact: true }).count(), `Existing VI was changed: ${value}`);
  await capture(page, 'desktop-loxone-inputs');
  robots[0].health.dock_state = 'error';
  await page.getByRole('button', { name: 'Actualiser', exact: true }).click();
  await page.evaluate(({ slug, status }) => window.__qaEmitStatus({ device: slug, ...status, dock_error_status: 38 }), { slug, status: defaultStatus });
  await page.getByText('Station à vérifier', { exact: true }).waitFor();
  await capture(page, 'desktop-station-alert');
  await page.evaluate(({ slug, status }) => window.__qaEmitStatus({ device: slug, ...status, dock_error_status: 0 }), { slug, status: defaultStatus });
  await page.getByText('Aucune erreur signalée', { exact: true }).waitFor();
  assert.equal(await page.getByText('Station à vérifier', { exact: true }).count(), 0, 'A fresh zero station error did not clear stale health error');
  robots[0].health.dock_state = 'charging';
  result.checks.push('Live station error is visible and explicit zero clears a stale station-health error');
  await page.getByRole('tab', { name: /Commandes/ }).first().click();
  await page.getByText(`/api/loxone/direct/v1/devices/${slug}/commands/start`, { exact: true }).waitFor();
  await capture(page, 'desktop-loxone-commands');
  await page.getByRole('button', { name: 'Pause', exact: true }).click();
  await page.getByText('Commande « Pause » envoyée au robot.', { exact: true }).waitFor();
  await page.getByRole('button', { name: devices[1].name, exact: true }).click();
  await page.getByText('RR_roborock_etage_running', { exact: true }).waitFor();
  assert.equal(await page.getByText(exactInputs[1], { exact: true }).count(), 0, 'Selecting another robot retained the first robot’s inputs');
  await capture(page, 'desktop-second-robot-loxone');
  result.checks.push('Exact existing VI names and canonical per-robot Direct API command URLs preserved');
  result.checks.push('Direct-only Loxone control submits to existing endpoint; robot selection keeps each robot’s inputs separate');

  await page.goto(`${origin}/devices/${slug}`);
  await page.getByRole('heading', { name: devices[0].name, exact: true }).waitFor();
  await page.getByRole('button', { name: 'Pause', exact: true }).click();
  await page.getByText('Commande transmise au robot.', { exact: false }).waitFor();
  assert.ok(apiRequests.some(request => request.method === 'POST' && request.path === `/api/loxone/devices/${slug}/command` && request.body.command === 'pause'));
  const beforeScene = apiRequests.filter(request => request.method === 'POST' && request.path.endsWith('/command')).length;
  await page.getByRole('button', { name: 'Après les repas', exact: true }).click();
  await page.getByRole('dialog').waitFor();
  assert.equal(apiRequests.filter(request => request.method === 'POST' && request.path.endsWith('/command')).length, beforeScene, 'Program was submitted before confirmation');
  await page.getByRole('dialog').getByRole('button', { name: 'Lancer le programme', exact: true }).click();
  await page.getByText('Commande transmise au robot.', { exact: false }).waitFor();
  assert.ok(apiRequests.some(request => request.method === 'POST' && request.path === `/api/loxone/devices/${slug}/command` && request.body.command === 'scene_id:42'));
  await capture(page, 'desktop-robot');
  result.checks.push('Robot control uses existing bridge command endpoint; request intercepted by fixture');
  result.checks.push('Program launch requires confirmation and submits canonical scene_id command');

  await page.goto(`${origin}/setup`);
  await page.getByText('192.0.2.20', { exact: false }).or(page.locator('input[value="192.0.2.20"]')).first().waitFor();
  await capture(page, 'desktop-settings');
  await page.getByRole('button', { name: 'Enregistrer les réglages', exact: true }).click();
  await page.getByText('Réglages enregistrés.', { exact: false }).waitFor();
  const save = apiRequests.findLast(request => request.method === 'PUT' && request.path === '/api/setup/settings');
  assert.ok(save, 'Saving settings did not call existing setup endpoint');
  assert.deepEqual(save.body.loxone.direct.inputs, setup.loxone.direct.inputs);
  assert.deepEqual(save.body.loxone.devices, setup.loxone.devices);
  assert.equal(save.body.loxone.direct.input_prefix, setup.loxone.direct.input_prefix);
  assert.equal(save.body.loxone.direct.api_username, setup.loxone.direct.api_username);
  assert.equal(save.body.loxone.command_debounce_ms, setup.loxone.command_debounce_ms);
  assert.equal(save.body.loxone.command_timeout_seconds, setup.loxone.command_timeout_seconds);
  assert.equal(save.body.loxone.direct.api_token, undefined, 'Existing token must stay omitted for backend preservation');
  assert.equal(apiRequests.filter(request => request.path.includes('/token/rotate')).length, 0, 'Opening settings unexpectedly rotated the token');
  result.checks.push('Settings save preserves VI mappings, prefix, per-robot selection, API user and command timings without rotating token');

  await page.goto(`${origin}/system`);
  await page.getByText('1.7.0', { exact: false }).first().waitFor();
  await capture(page, 'desktop-updates');
  assert.equal(apiRequests.filter(request => request.path.endsWith('/updates/install')).length, 0, 'Opening updates unexpectedly installed an update');
  result.checks.push('Updates page renders existing updater state without starting installation');

  await page.setViewportSize({ width: 390, height: 844 });
  await page.goto(origin);
  await page.getByRole('heading', { name: 'Mes robots.' }).waitFor();
  await capture(page, 'mobile-dashboard');
  await page.getByRole('button', { name: 'Ouvrir le menu', exact: true }).click();
  await page.getByRole('navigation', { name: 'Navigation principale' }).getByRole('link', { name: 'Loxone', exact: true }).click();
  await page.getByText(exactInputs[0], { exact: true }).waitFor();
  await capture(page, 'mobile-loxone');
  await page.goto(`${origin}/devices/${slug}`);
  await page.getByRole('heading', { name: devices[0].name, exact: true }).waitFor();
  await capture(page, 'mobile-robot');
  await page.goto(`${origin}/setup`);
  await page.locator('input[value="192.0.2.20"]').waitFor();
  await capture(page, 'mobile-settings');
  await page.goto(`${origin}/system`);
  await page.getByText('1.7.0', { exact: false }).first().waitFor();
  await capture(page, 'mobile-updates');
  result.checks.push('Mobile navigation works and dashboard/Loxone have no page-level horizontal overflow');

  expectedNetworkError = true;
  failSetup = true;
  const disconnected = await context.newPage();
  listen(disconnected);
  await disconnected.goto(`${origin}/setup`);
  await disconnected.getByText('Le bridge ne répond pas pour le moment.', { exact: false }).waitFor();
  assert.equal(await disconnected.getByRole('heading', { name: 'Relions vos robots à Loxone.' }).count(), 0, 'Failed request was interpreted as an empty new installation');
  failSetup = false;
  await disconnected.getByRole('button', { name: 'Réessayer', exact: true }).click();
  await disconnected.locator('input[value="192.0.2.20"]').waitFor();
  await disconnected.close();
  result.checks.push('Temporary setup fetch failure preserves existing-installation mode and recovers on retry');

  fixtureSetup = structuredClone(setup);
  fixtureSetup.setup_complete = false;
  fixtureSetup.authenticated = false;
  fixtureSetup.roborock_username = '';
  fixtureSetup.mqtt.url = '';
  fixtureSetup.loxone.direct.host = '';
  fixtureSetup.loxone.direct.username = '';
  fixtureSetup.loxone.direct.password_configured = false;
  fixtureSetup.loxone.direct.api_token_configured = false;
  const fresh = await context.newPage();
  listen(fresh);
  await fresh.setViewportSize({ width: 390, height: 844 });
  await fresh.goto(origin);
  await fresh.getByRole('heading', { name: 'Relions vos robots à Loxone.' }).waitFor();
  await fresh.getByRole('tab', { name: 'Connexion Loxone', exact: true }).click();
  await fresh.getByLabel('Adresse du Miniserver', { exact: true }).fill('192.0.2.55');
  await fresh.getByLabel('Utilisateur Loxone', { exact: true }).fill('new-user');
  await fresh.getByLabel('Mot de passe Loxone', { exact: true }).fill('fixture-password');
  await fresh.getByRole('tab', { name: 'Compte & robots', exact: true }).click();
  await fresh.getByLabel('Adresse e-mail du compte Roborock').fill('test@example.test');
  await fresh.getByRole('button', { name: 'Recevoir un code de connexion', exact: true }).click();
  await fresh.getByRole('alert').getByText('Connexion momentanément indisponible.', { exact: true }).waitFor();
  assert.equal(await fresh.getByLabel('Adresse e-mail du compte Roborock').inputValue(), 'test@example.test');
  await capture(fresh, 'mobile-fresh-setup-recoverable-error');
  failRequestCode = false;
  await fresh.getByRole('button', { name: 'Recevoir un code de connexion', exact: true }).click();
  await fresh.getByLabel('Code reçu par e-mail').fill('123456');
  await fresh.getByRole('button', { name: 'Connecter', exact: true }).click();
  await fresh.getByText('Compte connecté. Vos robots ont été récupérés.', { exact: true }).waitFor();
  await fresh.getByRole('tab', { name: 'Connexion Loxone', exact: true }).click();
  assert.equal(await fresh.getByLabel('Adresse du Miniserver', { exact: true }).inputValue(), '192.0.2.55');
  assert.equal(await fresh.getByLabel('Utilisateur Loxone', { exact: true }).inputValue(), 'new-user');
  assert.equal(await fresh.getByLabel('Mot de passe Loxone', { exact: true }).inputValue(), 'fixture-password');
  assert.ok(await fresh.getByRole('checkbox', { name: /Connexion directe HTTP à Loxone/ }).isChecked());
  await capture(fresh, 'mobile-fresh-setup-login-preserved');
  await fresh.close();
  expectedNetworkError = false;
  result.checks.push('Fresh setup displays recoverable login network errors and retains the entered email');
  result.checks.push('Fresh setup retains unsaved Miniserver address, username, password and Direct mode across login');
  assert.deepEqual(errors, [], 'Browser runtime/console/API errors detected');
  result.passed = true;
} catch (error) {
  result.passed = false;
  result.failure = error.stack || error.message;
  process.exitCode = 1;
} finally {
  await browser?.close();
  vite?.kill('SIGTERM');
  await writeFile(join(output, 'results.json'), `${JSON.stringify(result, null, 2)}\n`);
  console.log(JSON.stringify({ passed: result.passed, screenshots, checks: result.checks, errors, warnings, failure: result.failure, requestCount: apiRequests.length }, null, 2));
}
