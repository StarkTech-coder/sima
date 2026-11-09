// Browser-only regression test. Playwright is a separate test tool, not an app dependency.
// Set PLAYWRIGHT_MODULE to an installed playwright-core index.mjs and run against a fresh demo.
import assert from 'node:assert/strict';
import { pathToFileURL } from 'node:url';
import { join } from 'node:path';
import { tmpdir } from 'node:os';

if (!process.env.PLAYWRIGHT_MODULE) throw Error('Set PLAYWRIGHT_MODULE to playwright-core/index.mjs');
const { chromium } = await import(pathToFileURL(process.env.PLAYWRIGHT_MODULE).href);
const base = process.env.SIMA_URL || 'http://127.0.0.1:8080';
const browser = await chromium.launch({ channel: 'chrome', headless: true });
const errors = [], failedRequests = [], responses = [];
try {
  const page = await browser.newPage({ viewport: { width: 1440, height: 1100 } });
  page.on('pageerror', e => errors.push(e.message));
  page.on('console', m => { if (m.type() === 'error') errors.push(m.text()); });
  page.on('requestfailed', r => failedRequests.push(`${r.url()}: ${r.failure()?.errorText}`));
  page.on('response', r => { if (r.url().startsWith(base)) responses.push({ url: r.url(), status: r.status() }); });
  await page.goto(base);
  await page.waitForFunction(() => document.querySelectorAll('.vehicle-card').length === 3 && !document.getElementById('disconnect').disabled);
  assert.deepEqual(await page.locator('.vehicle-title h3').allTextContents(), ['AUV-01','AUV-02','AUV-03']);
  assert.equal(await page.locator('#tasks tr').count(), 4);
  assert.ok(await page.locator('#events .event').count() > 0);
  assert.equal(await page.locator('body').evaluate(e => getComputedStyle(e).backgroundColor), 'rgb(7, 15, 22)');
  for (const selector of ['.vehicle-card', '#tasks', '#events', '#vehicles', '#routes']) assert.ok(await page.locator(selector).first().isVisible(), selector);
  const screenshot = join(tmpdir(), 'sima-dashboard.png');
  await page.screenshot({ path: screenshot, fullPage: true });
  console.log('PASS direct render: 3 cards, 4 tasks, telemetry, routes, events, dark CSS');
  console.log('Screenshot:', screenshot);

  // Same sandbox flags as VS Code Simple Browser, with a same-origin ancestor.
  await page.route(`${base}/__preview_test`, r => r.fulfill({ contentType: 'text/html', body: '<!doctype html><iframe src="/" style="width:1400px;height:1100px" sandbox="allow-scripts allow-forms allow-same-origin allow-downloads"></iframe>' }));
  await page.goto(`${base}/__preview_test`);
  const frame = page.frameLocator('iframe');
  await frame.locator('.vehicle-card').first().waitFor();
  assert.equal(await frame.locator('.vehicle-card').count(), 3);
  assert.ok(await frame.locator('.vehicle-card').nth(2).isVisible());
  console.log('PASS sandboxed iframe render (white-screen regression)');

  await frame.locator('#disconnect').click();
  const auv02 = frame.locator('.vehicle-card').filter({ hasText: 'AUV-02' });
  await auv02.locator('.status').filter({ hasText: 'ERİŞİLEMEZ' }).waitFor();
  assert.match(await frame.locator('#tasks').innerText(), /AUV-02/);
  assert.match(await auv02.locator('.vehicle-mode').innerText(), /canlı konum bilinmiyor/);
  const stateDuringOutage = await (await page.request.get(`${base}/api/state`)).json();
  assert.equal(stateDuringOutage.tasks[1].owner, 'AUV-02');
  assert.equal(stateDuringOutage.tasks[3].owner, 'AUV-02');
  console.log('PASS Disconnect click: unreachable, stale telemetry, ownership retained');

  await frame.locator('#reconnect').click();
  await frame.locator('#tasks tr').filter({ hasText: 'S-04' }).filter({ hasText: 'NESİL 2' }).waitFor();
  await auv02.locator('.status').filter({ hasText: /^ERİŞİLEBİLİR$/ }).waitFor();
  const recovered = await (await page.request.get(`${base}/api/state`)).json();
  assert.equal(recovered.tasks[3].generation, 2);
  assert.notEqual(recovered.tasks[3].owner, 'AUV-02');
  assert.ok(recovered.events.some(e => e.message.includes('serbest bırakma makbuzu doğrulandı')));
  assert.ok(recovered.events.some(e => e.kind === 'reconcile'));
  const after = join(tmpdir(), 'sima-dashboard-reconnected.png');
  await page.screenshot({ path: after, fullPage: true });
  console.log('PASS Reconnect click: snapshot reconciliation, release receipt, generation 2 ->', recovered.tasks[3].owner);
  console.log('Screenshot:', after);
  assert.deepEqual(errors, [], 'Browser console/runtime errors');
  assert.deepEqual(failedRequests, [], 'Failed browser requests');
  assert.ok(responses.every(r => r.status < 400), JSON.stringify(responses.filter(r => r.status >= 400)));
  console.log('PASS browser errors=0, failed requests=0, HTTP responses all successful');

  // Ensure the fix did not permit arbitrary third-party embedding.
  const blocked = await browser.newPage();
  const violations = [];
  blocked.on('console', m => { if (m.text().includes('frame-ancestors')) violations.push(m.text()); });
  await blocked.route('http://untrusted.example/', r => r.fulfill({ contentType: 'text/html', body: `<iframe src="${base}"></iframe>` }));
  await blocked.goto('http://untrusted.example/');
  await blocked.waitForEvent('console', { predicate: m => m.text().includes('frame-ancestors') }).catch(() => {
    assert.ok(violations.length, 'Expected third-party frame CSP rejection');
  });
  assert.ok(violations.length);
  console.log('PASS untrusted third-party iframe blocked');
} finally {
  await browser.close();
}