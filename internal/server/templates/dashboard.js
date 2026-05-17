function formatAgo(unix) {
  const d = (Date.now() / 1000 - unix);
  if (d < 60) return Math.floor(d) + 's';
  if (d < 3600) return Math.floor(d / 60) + 'm';
  return Math.floor(d / 3600) + 'h';
}

function escapeHtml(s) {
  return String(s).replace(/[&<>"']/g, c => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' })[c]);
}

function showToast(msg, isErr) {
  const t = document.getElementById('toast');
  t.textContent = msg;
  t.style.borderColor = isErr ? 'var(--red)' : 'var(--green)';
  t.style.color = isErr ? 'var(--red)' : 'var(--green)';
  t.style.background = isErr ? 'rgba(255,122,122,0.15)' : 'rgba(127,255,170,0.15)';
  t.style.display = 'block';
  setTimeout(() => t.style.display = 'none', 2500);
}

async function api(method, path, body) {
  const opts = { method, headers: { 'Content-Type': 'application/json' } };
  if (body !== undefined) opts.body = JSON.stringify(body);
  const r = await fetch(path, opts);
  if (!r.ok) throw new Error('HTTP ' + r.status);
  return r.json();
}

async function killSession(id) {
  if (!confirm('¿Matar la sesión ' + id.slice(0, 8) + '? Sus clientes se desconectarán.')) return;
  try { await api('POST', '/admin/api/sessions/' + id + '/kill'); showToast('Sesión eliminada'); refresh(); }
  catch (e) { showToast('Error: ' + e.message, true); }
}

async function editLabel(id, current) {
  const label = prompt('Nuevo alias para la sesión:', current || '');
  if (label === null) return;
  try { await api('POST', '/admin/api/sessions/' + id + '/label', { label }); showToast('Alias actualizado'); refresh(); }
  catch (e) { showToast('Error: ' + e.message, true); }
}

async function togglePublic(id, makePublic) {
  try { await api('POST', '/admin/api/sessions/' + id + '/public', { public: makePublic }); showToast(makePublic ? 'Marcada pública' : 'Marcada privada'); refresh(); }
  catch (e) { showToast('Error: ' + e.message, true); }
}

async function createInvite() {
  const label = document.getElementById('inv-label').value.trim();
  try {
    const inv = await api('POST', '/admin/api/invites', { label });
    document.getElementById('inv-label').value = '';
    try { await navigator.clipboard.writeText(inv.url); showToast('Invite generado + URL copiada al clipboard'); }
    catch { showToast('Invite generado: ' + inv.url); }
    refresh();
  } catch (e) { showToast('Error: ' + e.message, true); }
}

async function copyInvite(url) {
  try { await navigator.clipboard.writeText(url); showToast('URL copiada'); }
  catch { showToast('No se pudo copiar — ' + url, true); }
}

async function revokeInvite(token) {
  if (!confirm('¿Revocar este invite? No podrá usarse.')) return;
  try { await api('DELETE', '/admin/api/invites/' + token); showToast('Invite revocado'); refresh(); }
  catch (e) { showToast('Error: ' + e.message, true); }
}

async function refresh() {
  try {
    const [list, invs] = await Promise.all([
      api('GET', '/admin/api/sessions'),
      api('GET', '/admin/api/invites'),
    ]);
    const rows = document.getElementById('rows');
    let full = 0, priv = 0;
    if (list.length === 0) {
      rows.innerHTML = '<tr><td colspan="7" class="empty">No hay sesiones activas</td></tr>';
    } else {
      rows.innerHTML = list.map(s => {
        if (s.mode === 'full') full++; else if (s.mode === 'privileged') priv++;
        const labelCell = s.label
          ? '<span class="label-cell">' + escapeHtml(s.label) + '</span>'
          : '<span class="label-cell empty-label">(sin alias)</span>';
        const shareCell = s.has_share
          ? '<span class="pill ' + (s.public ? 'share' : 'private') + '">' + (s.public ? '◆ PÚBLICA' : '◆ PRIVADA') + '</span>'
          : '<span class="pill no-share">— sin share</span>';
        const pubBtn = s.has_share
          ? '<button class="act" onclick="togglePublic(\'' + s.id + '\', ' + (!s.public) + ')">' + (s.public ? 'Hacer privada' : 'Hacer pública') + '</button>'
          : '';
        return '<tr>' +
          '<td>' + s.id.slice(0, 8) + '…</td>' +
          '<td class="mode-' + s.mode + '">' + s.mode.toUpperCase() + '</td>' +
          '<td>' + labelCell + '</td>' +
          '<td>' + shareCell + '</td>' +
          '<td class="ago">hace ' + formatAgo(s.created_at) + '</td>' +
          '<td class="ago">hace ' + formatAgo(s.last_active) + '</td>' +
          '<td>' +
            '<button class="act" onclick="editLabel(\'' + s.id + '\', ' + JSON.stringify(s.label).replace(/"/g, '&quot;') + ')">alias</button>' +
            pubBtn +
            '<button class="act danger" onclick="killSession(\'' + s.id + '\')">kill</button>' +
          '</td>' +
        '</tr>';
      }).join('');
    }
    document.getElementById('cnt-full').textContent = full;
    document.getElementById('cnt-priv').textContent = priv;
    document.getElementById('cnt-inv').textContent = invs.length;

    const invsT = document.getElementById('invs');
    if (invs.length === 0) {
      invsT.innerHTML = '<tr><td colspan="4" class="empty">No hay invites pendientes</td></tr>';
    } else {
      invsT.innerHTML = invs.map(i =>
        '<tr>' +
          '<td><span class="invite-token">' + i.token.slice(0, 12) + '…</span></td>' +
          '<td>' + (i.label ? escapeHtml(i.label) : '<span class="empty-label">(sin alias)</span>') + '</td>' +
          '<td class="ago">hace ' + formatAgo(i.created_at) + '</td>' +
          '<td>' +
            '<button class="act copy" onclick="copyInvite(\'' + i.url + '\')">⎘ copiar</button>' +
            '<button class="act danger" onclick="revokeInvite(\'' + i.token + '\')">revocar</button>' +
          '</td>' +
        '</tr>'
      ).join('');
    }
  } catch (e) { console.error('refresh failed', e); }
}

document.getElementById('inv-create').addEventListener('click', createInvite);
document.getElementById('inv-label').addEventListener('keydown', (e) => { if (e.key === 'Enter') createInvite(); });

refresh();
setInterval(refresh, 5000);
