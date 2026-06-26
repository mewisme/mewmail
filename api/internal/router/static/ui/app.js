(function () {
  const KEY = 'mewmail_api_key';

  const gate = document.getElementById('gate');
  const app = document.getElementById('app');
  const keyInput = document.getElementById('api-key');
  const gateErr = document.getElementById('gate-err');
  const statusEl = document.getElementById('status');
  const listEl = document.getElementById('list');
  const detailEl = document.getElementById('detail');
  const filterTo = document.getElementById('filter-to');
  const autoWait = document.getElementById('auto-wait');

  let emails = [];
  let selectedId = null;
  let selectedEmail = null;
  let waitAbort = null;
  let busy = false;

  function setStatus(msg, isErr) {
    statusEl.textContent = msg || '';
    statusEl.classList.toggle('err', !!isErr);
  }

  function showGate(err) {
    app.classList.add('hidden');
    gate.classList.remove('hidden');
    gateErr.textContent = err || '';
    gateErr.classList.toggle('hidden', !err);
    stopWait();
  }

  function showApp() {
    gate.classList.add('hidden');
    app.classList.remove('hidden');
  }

  async function api(path, opts) {
    const key = localStorage.getItem(KEY);
    const res = await fetch(path, {
      ...opts,
      headers: {
        Authorization: 'Bearer ' + key,
        ...(opts && opts.headers),
      },
    });
    if (res.status === 401) {
      localStorage.removeItem(KEY);
      showGate('Invalid API key');
      throw new Error('unauthorized');
    }
    const ct = res.headers.get('content-type') || '';
    if (!ct.includes('application/json')) {
      if (!res.ok) throw new Error(res.statusText);
      return res;
    }
    const json = await res.json();
    if (!res.ok || !json.success) {
      throw new Error(json.error || res.statusText);
    }
    return json.data;
  }

  async function validateKey() {
    await api('/emails/stats');
  }

  function setActiveRow(id) {
    const prev = listEl.querySelector('.list-item.active');
    if (prev) prev.classList.remove('active');
    if (id == null) return;
    const next = listEl.querySelector('.list-item[data-id="' + id + '"]');
    if (next) next.classList.add('active');
  }

  function renderList() {
    const frag = document.createDocumentFragment();
    if (!emails.length) {
      const p = document.createElement('p');
      p.className = 'list-empty';
      p.textContent = 'No emails';
      frag.appendChild(p);
      listEl.replaceChildren(frag);
      return;
    }
    for (const e of emails) {
      const row = document.createElement('div');
      row.className = 'list-item' + (e.id === selectedId ? ' active' : '');
      row.dataset.id = String(e.id);

      const subj = document.createElement('div');
      subj.className = 'subj';
      subj.textContent = e.subject || '(no subject)';
      if (e.kept) {
        const b = document.createElement('span');
        b.className = 'badge kept';
        b.textContent = 'kept';
        subj.appendChild(b);
      }
      if (e.opened_at) {
        const b = document.createElement('span');
        b.className = 'badge opened';
        b.textContent = 'opened';
        subj.appendChild(b);
      }

      const meta = document.createElement('div');
      meta.className = 'row-meta';
      meta.textContent = (e.mail_from || '?') + ' → ' + (e.rcpt_to || '?');

      row.appendChild(subj);
      row.appendChild(meta);
      frag.appendChild(row);
    }
    listEl.replaceChildren(frag);
  }

  function renderDetailEmpty() {
    detailEl.replaceChildren();
    const p = document.createElement('p');
    p.className = 'detail-empty';
    p.textContent = 'Select an email';
    detailEl.appendChild(p);
    selectedEmail = null;
  }

  function renderDetailActions(email) {
    const actions = document.createElement('div');
    actions.className = 'actions';

    const keepBtn = document.createElement('button');
    keepBtn.type = 'button';
    keepBtn.textContent = email.kept ? 'Unkeep' : 'Keep';
    keepBtn.disabled = busy;
    keepBtn.addEventListener('click', () => toggleKeep(email));

    const rawBtn = document.createElement('button');
    rawBtn.type = 'button';
    rawBtn.textContent = 'Download raw';
    rawBtn.disabled = busy;
    rawBtn.addEventListener('click', () => downloadRaw(email.id));

    const delBtn = document.createElement('button');
    delBtn.type = 'button';
    delBtn.className = 'danger';
    delBtn.textContent = 'Delete';
    delBtn.disabled = busy;
    delBtn.addEventListener('click', () => deleteEmail(email.id));

    actions.appendChild(keepBtn);
    actions.appendChild(rawBtn);
    actions.appendChild(delBtn);
    return actions;
  }

  async function selectEmail(id) {
    selectedId = id;
    setActiveRow(id);
    setStatus('Loading email…');
    try {
      const email = await api('/emails/' + id + '?track_open=false');
      selectedEmail = email;
      detailEl.replaceChildren();
      detailEl.appendChild(renderDetailActions(email));
      const body = document.createElement('div');
      renderEmail(email, body);
      detailEl.appendChild(body);
      setStatus('');
    } catch (err) {
      if (err.message !== 'unauthorized') setStatus(err.message, true);
    }
  }

  async function loadInbox() {
    if (busy) return;
    busy = true;
    setStatus('Loading…');
    try {
      const to = filterTo.value.trim();
      const q = new URLSearchParams({ limit: '50' });
      if (to) q.set('to', to);
      const data = await api('/emails?' + q);
      emails = data.emails || [];
      if (selectedId && !emails.find((e) => e.id === selectedId)) {
        selectedId = null;
        renderDetailEmpty();
      }
      renderList();
      setStatus(emails.length ? emails.length + ' of ' + data.total + ' shown' : 'Inbox empty');
      maybeStartWait();
    } catch (err) {
      if (err.message !== 'unauthorized') setStatus(err.message, true);
    } finally {
      busy = false;
    }
  }

  function newestId() {
    if (!emails.length) return 0;
    return Math.max(...emails.map((e) => e.id));
  }

  function stopWait() {
    if (waitAbort) {
      waitAbort.abort();
      waitAbort = null;
    }
  }

  async function waitLoop(signal) {
    while (!signal.aborted && autoWait.checked) {
      try {
        const to = filterTo.value.trim();
        const q = new URLSearchParams({
          since_id: String(newestId()),
          timeout: '25',
        });
        if (to) q.set('to', to);
        setStatus('Waiting for new mail…');
        const email = await api('/emails/wait?' + q);
        if (signal.aborted) return;
        const exists = emails.find((e) => e.id === email.id);
        if (!exists) {
          emails.unshift(email);
          renderList();
        }
        setStatus('New mail: ' + (email.subject || '#' + email.id));
        if (autoWait.checked) continue;
      } catch (err) {
        if (signal.aborted) return;
        if (err.message === 'unauthorized') return;
        if (err.message.includes('timeout')) {
          if (autoWait.checked) continue;
          setStatus('Wait timed out');
          return;
        }
        setStatus(err.message, true);
        return;
      }
    }
  }

  function maybeStartWait() {
    stopWait();
    if (!autoWait.checked) return;
    waitAbort = new AbortController();
    waitLoop(waitAbort.signal);
  }

  async function toggleKeep(email) {
    busy = true;
    renderDetailIfSelected();
    try {
      const path = '/emails/' + email.id + '/keep';
      const data = email.kept
        ? await api(path, { method: 'DELETE' })
        : await api(path, { method: 'POST' });
      const updated = data.email;
      const idx = emails.findIndex((e) => e.id === updated.id);
      if (idx >= 0) emails[idx] = { ...emails[idx], kept: updated.kept };
      selectedEmail = updated;
      renderList();
      detailEl.replaceChildren();
      detailEl.appendChild(renderDetailActions(updated));
      const body = document.createElement('div');
      renderEmail(updated, body);
      detailEl.appendChild(body);
      setStatus(updated.kept ? 'Email kept' : 'Keep removed');
    } catch (err) {
      if (err.message !== 'unauthorized') setStatus(err.message, true);
    } finally {
      busy = false;
    }
  }

  function renderDetailIfSelected() {
    if (selectedEmail) {
      detailEl.replaceChildren();
      detailEl.appendChild(renderDetailActions(selectedEmail));
      const body = document.createElement('div');
      renderEmail(selectedEmail, body);
      detailEl.appendChild(body);
    }
  }

  async function downloadRaw(id) {
    busy = true;
    renderDetailIfSelected();
    try {
      const key = localStorage.getItem(KEY);
      const res = await fetch('/emails/' + id + '/raw?track_open=false', {
        headers: { Authorization: 'Bearer ' + key },
      });
      if (res.status === 401) {
        localStorage.removeItem(KEY);
        showGate('Invalid API key');
        return;
      }
      if (!res.ok) throw new Error(res.statusText);
      const blob = await res.blob();
      const a = document.createElement('a');
      a.href = URL.createObjectURL(blob);
      a.download = 'email-' + id + '.eml';
      a.click();
      URL.revokeObjectURL(a.href);
      setStatus('Downloaded raw email');
    } catch (err) {
      setStatus(err.message, true);
    } finally {
      busy = false;
      renderDetailIfSelected();
    }
  }

  async function deleteEmail(id) {
    if (!confirm('Delete email #' + id + '?')) return;
    busy = true;
    renderDetailIfSelected();
    try {
      await api('/emails/' + id, { method: 'DELETE' });
      emails = emails.filter((e) => e.id !== id);
      if (selectedId === id) {
        selectedId = null;
        renderDetailEmpty();
      }
      renderList();
      setStatus('Email deleted');
      maybeStartWait();
    } catch (err) {
      if (err.message !== 'unauthorized') setStatus(err.message, true);
    } finally {
      busy = false;
    }
  }

  listEl.addEventListener('click', (e) => {
    const row = e.target.closest('.list-item');
    if (!row) return;
    selectEmail(Number(row.dataset.id));
  });

  document.getElementById('save-key').addEventListener('click', async () => {
    const key = keyInput.value.trim();
    if (!key) {
      gateErr.textContent = 'Enter an API key';
      gateErr.classList.remove('hidden');
      return;
    }
    localStorage.setItem(KEY, key);
    try {
      await validateKey();
      showApp();
      await loadInbox();
    } catch (err) {
      if (err.message !== 'unauthorized') {
        gateErr.textContent = err.message;
        gateErr.classList.remove('hidden');
      }
    }
  });

  keyInput.addEventListener('keydown', (e) => {
    if (e.key === 'Enter') document.getElementById('save-key').click();
  });

  document.getElementById('clear-key').addEventListener('click', () => {
    localStorage.removeItem(KEY);
    showGate();
    keyInput.value = '';
  });

  document.getElementById('refresh').addEventListener('click', () => loadInbox());
  document.getElementById('apply-filter').addEventListener('click', () => loadInbox());
  filterTo.addEventListener('keydown', (e) => {
    if (e.key === 'Enter') loadInbox();
  });

  autoWait.addEventListener('change', () => {
    if (autoWait.checked) maybeStartWait();
    else {
      stopWait();
      setStatus('');
    }
  });

  (async function init() {
    const saved = localStorage.getItem(KEY);
    if (!saved) return;
    keyInput.value = saved;
    try {
      await validateKey();
      showApp();
      await loadInbox();
    } catch {
      /* showGate handled in api() */
    }
  })();
})();
