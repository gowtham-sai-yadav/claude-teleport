// entangle VS Code extension.
//
// This is a thin wrapper: it contributes IDE commands and UI, and shells out to
// the `entangle` binary for all the real work. Nothing about sessions,
// encryption, or bundles is reimplemented here.
//
// The one non-trivial job is making the CLI available so a user only has to
// install *one* thing (this extension). getBin() prefers a `entangle`
// already on PATH; failing that it downloads the right prebuilt binary from the
// GitHub release, verifies its SHA-256 (mirroring install.sh), and caches it in
// the extension's storage. Interactive flows run in an integrated terminal so
// the CLI's own output (the code, the progress bar) is what you see.

import * as vscode from 'vscode';
import { execFile } from 'child_process';
import { promisify } from 'util';
import * as https from 'https';
import * as crypto from 'crypto';
import * as fs from 'fs';
import * as path from 'path';

const pexecFile = promisify(execFile);

const REPO = 'gowtham-sai-yadav/entangle';
// The extension calls `sessions --json`, added in this version, so anything
// older is treated as unusable and a newer build is fetched instead.
const MIN_VERSION: [number, number, number] = [0, 5, 1];

// Session mirrors the CLI's `sessions --json` output. The CLI treats these keys
// as a stable contract and only ever adds to them, so an older extension keeps
// working against a newer CLI. `provider` says which coding tool recorded the
// session; it is optional here so this build also tolerates a CLI predating it.
interface Session {
  provider?: string;
  id: string;
  shortId: string;
  project: string;
  folder: string;
  messages: number;
  modified: string;
  sizeBytes: number;
  title: string;
}

// providerLabel is the short badge naming which coding tool recorded a session.
//
// mixed says whether the list it appears in holds more than one tool. On a
// single-tool machine the badge would say the same thing on every row, so it is
// dropped; in a mixed list every row carries it, including Claude Code's -
// badging only the others reads as "these are the unusual ones".
function providerLabel(s: Session, mixed: boolean): string {
  if (!mixed || !s.provider) {
    return '';
  }
  return ` · ${s.provider}`;
}

// isMixed reports whether a listing spans more than one coding tool.
function isMixed(sessions: Session[]): boolean {
  const seen = new Set(sessions.map((s) => s.provider || 'claude-code'));
  return seen.size > 1;
}

let extContext: vscode.ExtensionContext;
let cachedBin: string | undefined;

function config(): { bin: string; configDir: string } {
  const c = vscode.workspace.getConfiguration('entangle');
  return {
    bin: c.get<string>('path')?.trim() || 'entangle',
    configDir: c.get<string>('configDir')?.trim() || '',
  };
}

function configArgs(): string[] {
  const { configDir } = config();
  return configDir ? ['--config-dir', configDir] : [];
}

// ---- locating (or provisioning) the CLI -----------------------------------

function cmpVer(a: number[], b: number[]): number {
  for (let i = 0; i < 3; i++) {
    if ((a[i] || 0) !== (b[i] || 0)) {
      return (a[i] || 0) - (b[i] || 0);
    }
  }
  return 0;
}

// probeOK reports whether `bin version` runs and is recent enough to use.
async function probeOK(bin: string): Promise<boolean> {
  try {
    const { stdout } = await pexecFile(bin, ['version'], { timeout: 5000 });
    const m = stdout.match(/(\d+)\.(\d+)\.(\d+)/);
    if (!m) {
      return true; // runs but the version is unreadable — accept it
    }
    return cmpVer([+m[1], +m[2], +m[3]], MIN_VERSION) >= 0;
  } catch {
    return false;
  }
}

// platformAsset returns the release asset name for this machine, matching the
// GoReleaser name_template (entangle-<os>-<arch>, .exe on Windows).
function platformAsset(): string {
  const osMap: Record<string, string> = { darwin: 'darwin', linux: 'linux', win32: 'windows' };
  const goos = osMap[process.platform];
  if (!goos) {
    throw new Error(`unsupported OS ${process.platform}`);
  }
  // Only amd64 is published for Windows; it runs under emulation on ARM.
  const goarch = goos === 'windows' ? 'amd64' : process.arch === 'arm64' ? 'arm64' : 'amd64';
  return `entangle-${goos}-${goarch}${goos === 'windows' ? '.exe' : ''}`;
}

function managedBinPath(): string {
  const name = process.platform === 'win32' ? 'entangle.exe' : 'entangle';
  return path.join(extContext.globalStorageUri.fsPath, 'bin', name);
}

// httpGet fetches a URL into a Buffer, following redirects (GitHub release
// downloads redirect to a signed asset URL).
function httpGet(url: string, redirects = 5): Promise<Buffer> {
  return new Promise((resolve, reject) => {
    https
      .get(url, { headers: { 'User-Agent': 'entangle-vscode' } }, (res) => {
        const status = res.statusCode || 0;
        if (status >= 300 && status < 400 && res.headers.location) {
          res.resume();
          if (redirects <= 0) {
            reject(new Error('too many redirects'));
            return;
          }
          resolve(httpGet(new URL(res.headers.location, url).toString(), redirects - 1));
          return;
        }
        if (status !== 200) {
          res.resume();
          reject(new Error(`HTTP ${status} for ${url}`));
          return;
        }
        const chunks: Buffer[] = [];
        res.on('data', (c) => chunks.push(c));
        res.on('end', () => resolve(Buffer.concat(chunks)));
      })
      .on('error', reject);
  });
}

// parseSum finds the checksum for one asset in a SHA256SUMS.txt body.
function parseSum(sums: string, asset: string): string | undefined {
  for (const line of sums.split('\n')) {
    const parts = line.trim().split(/\s+/);
    if (parts.length >= 2 && parts[parts.length - 1] === asset) {
      return parts[0];
    }
  }
  return undefined;
}

async function downloadCli(progress: vscode.Progress<{ message?: string }>): Promise<string> {
  const asset = platformAsset();
  const base = `https://github.com/${REPO}/releases/latest/download`;

  progress.report({ message: 'fetching checksums…' });
  const want = parseSum((await httpGet(`${base}/SHA256SUMS.txt`)).toString('utf8'), asset);
  if (!want) {
    throw new Error(`the latest release has no checksum for ${asset}`);
  }

  progress.report({ message: `downloading ${asset}…` });
  const buf = await httpGet(`${base}/${asset}`);
  const got = crypto.createHash('sha256').update(buf).digest('hex');
  if (got.toLowerCase() !== want.toLowerCase()) {
    throw new Error('checksum mismatch — refusing to install');
  }

  const dest = managedBinPath();
  await fs.promises.mkdir(path.dirname(dest), { recursive: true });
  await fs.promises.writeFile(dest, buf, { mode: 0o755 });
  return dest;
}

// getBin resolves a usable CLI, provisioning one if needed. Returns undefined
// if the user declines (a message has already been shown).
async function getBin(): Promise<string | undefined> {
  if (cachedBin) {
    return cachedBin;
  }
  const { bin } = config();

  // An explicit path the user set is authoritative — trust it, don't override.
  if (bin !== 'entangle' && bin !== 'claude-teleport') {
    cachedBin = bin;
    return bin;
  }
  // Otherwise prefer one on PATH, then a previously downloaded copy.
  if (await probeOK('entangle')) {
    cachedBin = 'entangle';
    return cachedBin;
  }
  // Someone who installed before the rename still has the old command on PATH.
  // Use it rather than offering to download a second copy of the same program.
  if (await probeOK('claude-teleport')) {
    cachedBin = 'claude-teleport';
    return cachedBin;
  }
  const managed = managedBinPath();
  if (fs.existsSync(managed) && (await probeOK(managed))) {
    cachedBin = managed;
    return managed;
  }

  const choice = await vscode.window.showInformationMessage(
    'entangle (the CLI this extension drives) was not found. Download it now? It is fetched from the GitHub release and verified by checksum.',
    'Download',
    'Set path',
    'Cancel',
  );
  if (choice === 'Set path') {
    vscode.commands.executeCommand('workbench.action.openSettings', 'entangle.path');
    return undefined;
  }
  if (choice !== 'Download') {
    return undefined;
  }
  try {
    const dest = await vscode.window.withProgress(
      { location: vscode.ProgressLocation.Notification, title: 'Installing entangle', cancellable: false },
      (progress) => downloadCli(progress),
    );
    cachedBin = dest;
    vscode.window.showInformationMessage('entangle is ready.');
    return dest;
  } catch (e) {
    vscode.window.showErrorMessage('Could not download entangle: ' + (e as Error).message);
    return undefined;
  }
}

// ---- talking to the CLI ----------------------------------------------------

async function listSessions(bin: string): Promise<Session[]> {
  const big = { maxBuffer: 32 * 1024 * 1024 };
  const cfg = configArgs();

  // Prefer every coding tool on the machine. Two reasons this is not simply the
  // one call: a CLI older than --tool rejects the flag outright, and --config-dir
  // names a single tool's directory so it cannot be combined with "all".
  if (cfg.length === 0) {
    try {
      const { stdout } = await pexecFile(bin, ['sessions', '--json', '--tool', 'all'], big);
      return JSON.parse(stdout) as Session[];
    } catch {
      // Fall through to the single-tool form rather than showing an empty list.
    }
  }
  const { stdout } = await pexecFile(bin, ['sessions', '--json', ...cfg], big);
  return JSON.parse(stdout) as Session[];
}

// toolArgs tells the CLI which tool a session belongs to. Older CLIs do not know
// --tool, so it is only added for a non-Claude session: a Claude session (or one
// from a CLI too old to report a provider) keeps working against any version.
function toolArgs(s: Session): string[] {
  if (!s.provider || s.provider === 'claude-code') {
    return [];
  }
  return ['--tool', s.provider];
}

function shellQuote(s: string): string {
  return /^[\w@%+=:,./-]+$/.test(s) ? s : `'${s.replace(/'/g, `'\\''`)}'`;
}

function runInTerminal(name: string, bin: string, args: string[]): void {
  const term = vscode.window.createTerminal({ name });
  term.show();
  term.sendText([bin, ...args].map(shellQuote).join(' '));
}

async function handleCliError(e: unknown): Promise<void> {
  const err = e as { code?: string; stderr?: string; message?: string };
  const text = String(err.stderr || err.message || e);
  if (/not defined:\s*-?json/.test(text)) {
    vscode.window.showErrorMessage(
      'Your entangle is too old for this extension. Update it (run: entangle update) to v0.5.1 or newer.',
    );
    return;
  }
  vscode.window.showErrorMessage('entangle failed: ' + text.split('\n')[0]);
}

// ---- quick pick (palette flows) -------------------------------------------

interface SessionItem extends vscode.QuickPickItem {
  session: Session;
}

async function pickSession(placeHolder: string): Promise<Session | undefined> {
  const bin = await getBin();
  if (!bin) {
    return undefined;
  }
  let sessions: Session[];
  try {
    sessions = await listSessions(bin);
  } catch (e) {
    await handleCliError(e);
    return undefined;
  }
  if (sessions.length === 0) {
    vscode.window.showInformationMessage('No coding sessions found on this machine.');
    return undefined;
  }
  const mixed = isMixed(sessions);
  const items: SessionItem[] = sessions.map((s) => ({
    label: s.title || '(untitled session)',
    description: `${s.shortId} · ${s.messages} msgs${providerLabel(s, mixed)}`,
    detail: s.project || '(unknown project)',
    session: s,
  }));
  const pick = await vscode.window.showQuickPick(items, {
    placeHolder,
    matchOnDescription: true,
    matchOnDetail: true,
  });
  return pick?.session;
}

// ---- sidebar: a clickable session list in the Activity Bar ----------------

class SessionNode extends vscode.TreeItem {
  constructor(public readonly session: Session, mixed: boolean) {
    super(session.title || '(untitled session)', vscode.TreeItemCollapsibleState.None);
    this.description = `${session.shortId} · ${session.messages} msgs${providerLabel(session, mixed)}`;
    this.tooltip = `${session.title || '(untitled)'}\n${session.project || '(unknown project)'}\n${session.messages} messages`;
    this.contextValue = 'session';
    this.iconPath = new vscode.ThemeIcon('comment-discussion');
    this.command = { command: 'entangle.itemMenu', title: 'Actions', arguments: [this] };
  }
}

class SessionsProvider implements vscode.TreeDataProvider<SessionNode> {
  private readonly changed = new vscode.EventEmitter<void>();
  readonly onDidChangeTreeData = this.changed.event;

  refresh(): void {
    cachedBin = undefined; // re-resolve after an install / path change
    this.changed.fire();
  }

  getTreeItem(node: SessionNode): vscode.TreeItem {
    return node;
  }

  async getChildren(): Promise<SessionNode[]> {
    const bin = await getBin();
    if (!bin) {
      return [];
    }
    try {
      const sessions = await listSessions(bin);
      const mixed = isMixed(sessions);
      return sessions.map((s) => new SessionNode(s, mixed));
    } catch (e) {
      await handleCliError(e);
      return [];
    }
  }
}

async function itemMenu(node: SessionNode): Promise<void> {
  const s = node.session;
  const action = await vscode.window.showQuickPick(
    [
      { label: '$(radio-tower) Send by code', act: 'send' },
      { label: '$(file) Share to a file', act: 'share' },
      { label: '$(clippy) Copy session id', act: 'copy' },
    ] as (vscode.QuickPickItem & { act: string })[],
    { placeHolder: `${s.title || '(untitled)'} · ${s.shortId}` },
  );
  if (!action) {
    return;
  }
  if (action.act === 'copy') {
    await vscode.env.clipboard.writeText(s.id);
    vscode.window.showInformationMessage(`Copied session id ${s.shortId}.`);
    return;
  }
  const bin = await getBin();
  if (!bin) {
    return;
  }
  if (action.act === 'send') {
    runInTerminal('Teleport · send', bin, ['send', s.id, ...toolArgs(s), ...configArgs()]);
  } else {
    runInTerminal('Teleport · share', bin, ['share', s.id, ...toolArgs(s), ...configArgs()]);
  }
}

// ---- command handlers ------------------------------------------------------

async function cmdSend(): Promise<void> {
  const s = await pickSession('Pick a session to send by code');
  if (!s) {
    return;
  }
  const bin = await getBin();
  if (!bin) {
    return;
  }
  runInTerminal('Teleport · send', bin, ['send', s.id, ...toolArgs(s), ...configArgs()]);
  vscode.window.showInformationMessage('Read the code from the terminal to your teammate; they run "entangle receive <code>".');
}

async function cmdShare(): Promise<void> {
  const s = await pickSession('Pick a session to share to a file');
  if (!s) {
    return;
  }
  const bin = await getBin();
  if (!bin) {
    return;
  }
  runInTerminal('Teleport · share', bin, ['share', s.id, ...toolArgs(s), ...configArgs()]);
}

async function cmdReceive(): Promise<void> {
  const bin = await getBin();
  if (!bin) {
    return;
  }
  const code = await vscode.window.showInputBox({
    prompt: 'Enter the code your teammate read out',
    placeHolder: '7-crossover-marbles',
    ignoreFocusOut: true,
  });
  if (!code || !code.trim()) {
    return;
  }
  runInTerminal('Teleport · receive', bin, ['receive', code.trim(), ...configArgs()]);
}

async function cmdMenu(): Promise<void> {
  const pick = await vscode.window.showQuickPick(
    [
      { label: '$(radio-tower) Send a session by code', cmd: 'entangle.send' },
      { label: '$(inbox) Receive a session', cmd: 'entangle.receive' },
      { label: '$(file) Share a session to a file', cmd: 'entangle.share' },
      { label: '$(list-unordered) Browse sessions', cmd: 'entangle.sessions' },
    ] as (vscode.QuickPickItem & { cmd: string })[],
    { placeHolder: 'entangle' },
  );
  if (pick) {
    vscode.commands.executeCommand(pick.cmd);
  }
}

export function activate(context: vscode.ExtensionContext): void {
  extContext = context;

  const status = vscode.window.createStatusBarItem(vscode.StatusBarAlignment.Left, 0);
  status.text = '$(radio-tower) entangle';
  status.tooltip = 'entangle — hand a live coding session to a teammate';
  status.command = 'entangle.menu';
  status.show();

  const provider = new SessionsProvider();
  const view = vscode.window.createTreeView('entangle.sessions', { treeDataProvider: provider });
  const revealSidebar = () => vscode.commands.executeCommand('entangle.sessions.focus');

  context.subscriptions.push(
    status,
    view,
    vscode.commands.registerCommand('entangle.send', cmdSend),
    vscode.commands.registerCommand('entangle.receive', cmdReceive),
    vscode.commands.registerCommand('entangle.share', cmdShare),
    vscode.commands.registerCommand('entangle.sessions', revealSidebar),
    vscode.commands.registerCommand('entangle.menu', cmdMenu),
    vscode.commands.registerCommand('entangle.refresh', () => provider.refresh()),
    vscode.commands.registerCommand('entangle.itemMenu', (n: SessionNode) => itemMenu(n)),
    vscode.commands.registerCommand('entangle.sendItem', async (n: SessionNode) => {
      const bin = await getBin();
      if (bin) {
        runInTerminal('Teleport · send', bin, ['send', n.session.id, ...toolArgs(n.session), ...configArgs()]);
      }
    }),
    vscode.commands.registerCommand('entangle.shareItem', async (n: SessionNode) => {
      const bin = await getBin();
      if (bin) {
        runInTerminal('Teleport · share', bin, ['share', n.session.id, ...toolArgs(n.session), ...configArgs()]);
      }
    }),
    vscode.commands.registerCommand('entangle.copyItemId', async (n: SessionNode) => {
      await vscode.env.clipboard.writeText(n.session.id);
      vscode.window.showInformationMessage(`Copied session id ${n.session.shortId}.`);
    }),
  );
}

export function deactivate(): void {
  /* nothing to clean up */
}
