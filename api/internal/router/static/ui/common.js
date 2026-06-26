function emailTitle(email) {
  return email.subject || 'Email #' + email.id;
}

function formatMailDate(s) {
  if (!s) return '';
  if (typeof dayjs === 'function') {
    const d = dayjs(s);
    if (d.isValid()) return d.format('YYYY-MM-DD HH:mm');
  }
  return s;
}

function sanitizeHtml(html) {
  if (typeof DOMPurify !== 'undefined') {
    return DOMPurify.sanitize(html, { WHOLE_DOCUMENT: true });
  }
  return html;
}

function renderEmail(email, container) {
  container.replaceChildren();
  container.classList.add('email-view');

  const h1 = document.createElement('h1');
  h1.textContent = emailTitle(email);
  container.appendChild(h1);

  const meta = document.createElement('dl');
  meta.className = 'meta';
  for (const [label, value] of [
    ['From', email.mail_from],
    ['To', email.rcpt_to],
    ['Date', formatMailDate(email.mail_date)],
  ]) {
    const dt = document.createElement('dt');
    dt.textContent = label;
    const dd = document.createElement('dd');
    dd.textContent = value || '';
    meta.appendChild(dt);
    meta.appendChild(dd);
  }
  container.appendChild(meta);

  if (email.html_body) {
    const iframe = document.createElement('iframe');
    iframe.sandbox = '';
    iframe.loading = 'lazy';
    iframe.srcdoc = sanitizeHtml(email.html_body);
    container.appendChild(iframe);
  } else if (email.text_body) {
    const pre = document.createElement('pre');
    pre.textContent = email.text_body;
    container.appendChild(pre);
  } else {
    const p = document.createElement('p');
    p.className = 'empty';
    p.textContent = '(no body)';
    container.appendChild(p);
  }

  if (email.attachments && email.attachments.length) {
    const box = document.createElement('div');
    box.className = 'attachments';
    const strong = document.createElement('strong');
    strong.textContent = 'Attachments:';
    box.appendChild(strong);
    const ul = document.createElement('ul');
    ul.className = 'list-unstyled mb-0';
    for (const a of email.attachments) {
      const li = document.createElement('li');
      li.textContent = a.filename + ' (' + a.content_type + ', ' + a.size + ' bytes)';
      ul.appendChild(li);
    }
    box.appendChild(ul);
    container.appendChild(box);
  }
}
