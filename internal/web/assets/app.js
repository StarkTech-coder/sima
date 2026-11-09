'use strict';
const $ = id => document.getElementById(id);
const colors = { 'AUV-01': '#4ce1d2', 'AUV-02': '#f3be65', 'AUV-03': '#b1a0ff' };
const statuses = { pending: 'Bekliyor', offered: 'Makbuz bekleniyor', queued: 'Başlatılmamış', running: 'Keşifte', completed: 'Tamamlandı', released: 'Serbest', held: 'Güvenli bekleme' };
const health = { healthy: 'ERİŞİLEBİLİR', suspect: 'ŞÜPHELİ', unreachable: 'ERİŞİLEMEZ', unknown: 'BİLİNMİYOR' };
const modes = { ready: 'Görev bekliyor', surveying: 'Yetkili bölüm yürütülüyor', 'local-policy': 'Yerel sınırlı keşif politikası', 'segment-complete': 'Yetkili bölüm tamamlandı', 'safe-hold': 'Yerel güvenli bekleme' };
const kinds = { system: 'SİSTEM', decision: 'KARAR', receipt: 'MAKBUZ', reconcile: 'UZLAŞTIRMA', link: 'İLETİŞİM', warning: 'DİKKAT' };
let state = null, controlling = false;
const esc = value => String(value ?? '').replace(/[&<>"']/g, c => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]));
const num = (value, digits = 0) => Number.isFinite(value) ? value.toFixed(digits) : '—';
const percent = value => Math.max(0, Math.min(100, (value || 0) * 100));
const time = value => new Date(value).toLocaleTimeString('tr-TR', { hour12: false });

function render(s) {
    state = s;
    const vehicles = s.vehicles || [], tasks = s.tasks || [];
    const online = vehicles.filter(v => v.health === 'healthy').length;
    const progress = tasks.reduce((sum, t) => sum + t.progress, 0) / Math.max(tasks.length, 1);
    $('online').innerHTML = `${online} <small>/ 3 erişilebilir</small>`;
    $('progress').innerHTML = `${num(percent(progress))}<small>%</small>`;
    $('mission-state').textContent = !s.ready ? 'Hazırlanıyor' : tasks.every(t => t.status === 'completed') ? 'Tamamlandı' : online < 3 ? 'Bağlantı kısıtlı' : 'Keşif sürüyor';
    $('clock').textContent = time(s.now);
    $('link-title').textContent = !s.ready ? 'Başlangıç makbuzları bekleniyor' : s.disconnected ? 'AUV-02 iletişimi kesildi' : 'AUV-02 bağlantısı açık';
    $('disconnect').disabled = controlling || !s.ready || s.disconnected;
    $('reconnect').disabled = controlling || !s.ready || !s.disconnected;
    $('routes').innerHTML = tasks.map(t => `<polyline points="${(t.task.route || []).map(p => `${p.x * 10},${p.y * 6}`).join(' ')}" fill="none" stroke="${colors[t.owner] || '#607888'}" stroke-width="2" stroke-dasharray="5 8" opacity=".55"/><text x="${(t.task.route?.[0]?.x || 0) * 10}" y="${(t.task.route?.[0]?.y || 0) * 6 - 12}" fill="${colors[t.owner] || '#607888'}" font-size="11" font-family="monospace">${esc(t.task.id)} · G${t.generation}</text>`).join('');
    $('vehicles').innerHTML = vehicles.filter(v => v.snapshot.epoch).map(v => {
        const a = v.snapshot, stale = !v.synchronized || v.health !== 'healthy', color = stale ? '#9aa9b3' : colors[a.id];
        return `<g transform="translate(${a.position.x * 10},${a.position.y * 6})"><circle r="24" fill="${color}" opacity=".08"/><circle r="19" fill="none" stroke="${color}" opacity=".4" ${stale ? 'stroke-dasharray="3 4"' : ''}/><path d="M-13 0 L-7 -6 L12 -4 L18 0 L12 4 L-7 6 Z" fill="${color}"/><path d="M-6 -5 L-12 -12 L-1 -5 M-6 5 L-12 12 L-1 5" fill="${color}"/><text x="0" y="39" text-anchor="middle" fill="${color}" font-size="13" font-family="monospace">${esc(a.id)}</text>${stale ? '<text x="0" y="53" text-anchor="middle" fill="#9aa9b3" font-size="9">SON BİLİNEN KONUM</text>' : ''}</g>`;
    }).join('');
    $('cards').innerHTML = vehicles.map(v => {
        const a = v.snapshot, stale = !v.synchronized || v.health !== 'healthy';
        const age = a.epoch ? Math.max(0, (new Date(s.now) - new Date(v.lastSeen)) / 1000) : null;
        const active = (a.assignments || []).find(x => x.status === 'running');
        return `<article class="vehicle-card ${stale ? 'stale' : ''}" style="--accent:${colors[a.id]}"><div class="vehicle-title"><h3>${esc(a.id)}</h3><span class="status ${stale ? 'warn' : ''}">${esc(health[v.health] || v.health)}</span></div><div class="vehicle-mode">${stale ? 'Son bildirilen durum · canlı konum bilinmiyor' : esc(modes[a.mode] || a.mode)}${active ? ' / ' + esc(active.task.id) : ''}</div><div class="metrics"><div><span>DERİNLİK</span><strong>${a.epoch ? num(a.depth, 1) : '—'}</strong> <small>m</small></div><div><span>BATARYA</span><strong>${a.epoch ? num(a.battery, 1) : '—'}</strong> <small>%</small></div><div><span>HIZ</span><strong>${a.epoch ? num(a.speed, 1) : '—'}</strong> <small>m/s</small></div></div><div class="battery-track"><div style="width:${Math.max(0, Math.min(100, a.battery || 0))}%"></div></div><div class="vehicle-foot"><span>TELEMETRİ YAŞI ${age === null ? '—' : num(age, 1) + ' s'}</span><span>SEQ ${a.sequence} · ${stale ? 'ESKİ / BELİRSİZ' : 'UZLAŞTIRILDI'}</span></div></article>`;
    }).join('');
    $('tasks').innerHTML = tasks.map(t => `<tr><td><strong>${esc(t.task.id)}</strong><small>${esc(t.task.name)}</small></td><td>${esc(t.owner || '—')}<br><small>NESİL ${t.generation}</small></td><td>${esc(statuses[t.status] || t.status)}</td><td>${num(percent(t.progress))}%<div class="track"><div style="width:${percent(t.progress)}%"></div></div></td></tr>`).join('');
    $('events').innerHTML = [...(s.events || [])].reverse().map(e => `<div class="event ${esc(e.kind)}"><time>${time(e.time)}</time><div><b>${esc(kinds[e.kind] || e.kind)}</b><span>${esc(e.message)}</span></div></div>`).join('');
    $('sync').textContent = `MERKEZ BAĞLI · SON GÜNCELLEME ${time(s.now)}`;
}

async function poll() {
    let received = false;
    try { const response = await fetch('/api/state', { signal: AbortSignal.timeout(4500) }); if (!response.ok) throw Error('HTTP ' + response.status); const snapshot = await response.json(); received = true; render(snapshot); }
    catch (error) { console.error(received ? 'Dashboard render failed' : 'Dashboard polling failed', error); $('sync').textContent = received ? 'GÖRÜNTÜLEME HATASI · ' + error.message : 'MERKEZE ERİŞİLEMİYOR · GÖSTERİLEN VERİ ESKİ'; $('disconnect').disabled = true; $('reconnect').disabled = true; }
    finally { setTimeout(poll, 700); }
}
async function link(connected) {
    controlling = true; if (state) render(state); $('control-status').textContent = 'Simülasyon iletişim kapısı değiştiriliyor…';
    try { const response = await fetch('/api/link', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ connected }), signal: AbortSignal.timeout(4500) }); if (!response.ok) throw Error(await response.text()); $('control-status').textContent = connected ? 'Bağlantı açıldı. Gerçek snapshot ve makbuzlar bekleniyor.' : 'Kapı kapalı. Heartbeat eşiklerinin dolması bekleniyor.'; }
    catch (error) { $('control-status').textContent = 'Kontrol başarısız: ' + error.message; }
    finally { controlling = false; }
}
$('disconnect').addEventListener('click', () => link(false));
$('reconnect').addEventListener('click', () => link(true));
poll();