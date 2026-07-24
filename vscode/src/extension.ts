// claude-teleport VS Code extension.
//
// This is a thin wrapper: it contributes IDE commands and UI, and shells out to
// the `claude-teleport` binary for all the real work. Nothing about sessions,
// encryption, or bundles is reimplemented here. Interactive flows (send shows a
// code and progress; receive imports into the workspace) run in an integrated
// terminal so the CLI's own output is what you see.

import * as vscode from 'vscode';
import { execFile } from 'child_process';
import { promisify } from 'util';

const pexecFile = promisify(execFile);

interface Session {
  id: string;
  shortId: string;
  project: string;
  folder: string;
  messages: number;
  modified: string;
  sizeBytes: number;
  title: string;
}

function config(): { bin: string; configDir: string } {
  const c = vscode.workspace.getConfiguration('claude-teleport');
  return {
    bin: c.get<string>('path')?.trim() || 'claude-teleport',
    configDir: c.get<string>('configDir')?.trim() || '',
  };
}

// configArgs appends --config-dir only when the user set one.
function configArgs(): string[] {
  const { configDir } = config();
  return configDir ? ['--config-dir', configDir] : [];
}

// listSessions asks the CLI for the machine-readable session list.
async function listSessions(): Promise<Session[]> {
  const { bin } = config();
  const { stdout } = await pexecFile(bin, ['sessions', '--json', ...configArgs()], {
    maxBuffer: 32 * 1024 * 1024,
  });
  return JSON.parse(stdout) as Session[];
}

// shellQuote makes an argument safe to paste into a terminal command line.
function shellQuote(s: string): string {
  return /^[\w@%+=:,./-]+$/.test(s) ? s : `'${s.replace(/'/g, `'\\''`)}'`;
}

// runInTerminal launches a CLI subcommand in a fresh integrated terminal, so the
// user sees the code, the progress bar, and can Ctrl-C exactly as on the CLI.
function runInTerminal(name: string, args: string[]): void {
  const { bin } = config();
  const term = vscode.window.createTerminal({ name });
  term.show();
  term.sendText([bin, ...args].map(shellQuote).join(' '));
}

async function handleCliError(e: unknown): Promise<void> {
  const err = e as { code?: string; stderr?: string; message?: string };
  const text = String(err.stderr || err.message || e);

  if (err.code === 'ENOENT') {
    const choice = await vscode.window.showErrorMessage(
      'claude-teleport is not installed or not on your PATH.',
      'Install',
      'Set path',
    );
    if (choice === 'Install') {
      vscode.env.openExternal(vscode.Uri.parse('https://gowthamsai.in/teleport/#install'));
    } else if (choice === 'Set path') {
      vscode.commands.executeCommand('workbench.action.openSettings', 'claude-teleport.path');
    }
    return;
  }
  if (/not defined:\s*-?json/.test(text)) {
    vscode.window.showErrorMessage(
      'Your claude-teleport is too old for this extension. Update it to v0.5.1 or newer (run: claude-teleport update).',
    );
    return;
  }
  vscode.window.showErrorMessage('claude-teleport failed: ' + text.split('\n')[0]);
}

interface SessionItem extends vscode.QuickPickItem {
  session: Session;
}

// pickSession shows the session list as a native quick pick.
async function pickSession(placeHolder: string): Promise<Session | undefined> {
  let sessions: Session[];
  try {
    sessions = await listSessions();
  } catch (e) {
    await handleCliError(e);
    return undefined;
  }
  if (sessions.length === 0) {
    vscode.window.showInformationMessage('No Claude Code sessions found on this machine.');
    return undefined;
  }
  const items: SessionItem[] = sessions.map((s) => ({
    label: s.title || '(untitled session)',
    description: `${s.shortId} · ${s.messages} msgs`,
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

async function cmdSend(): Promise<void> {
  const s = await pickSession('Pick a session to send by code');
  if (!s) {
    return;
  }
  runInTerminal('Teleport · send', ['send', s.id, ...configArgs()]);
  vscode.window.showInformationMessage('Read the code from the terminal to your teammate; they run "claude-teleport receive <code>".');
}

async function cmdShare(): Promise<void> {
  const s = await pickSession('Pick a session to share to a file');
  if (!s) {
    return;
  }
  runInTerminal('Teleport · share', ['share', s.id, ...configArgs()]);
}

async function cmdReceive(): Promise<void> {
  const code = await vscode.window.showInputBox({
    prompt: 'Enter the code your teammate read out',
    placeHolder: '7-crossover-marbles',
    ignoreFocusOut: true,
  });
  if (!code || !code.trim()) {
    return;
  }
  // The terminal opens in the workspace folder, which is where the session lands.
  runInTerminal('Teleport · receive', ['receive', code.trim(), ...configArgs()]);
}

// ---- sidebar: a clickable session list in the Activity Bar ----------------

class SessionNode extends vscode.TreeItem {
  constructor(public readonly session: Session) {
    super(session.title || '(untitled session)', vscode.TreeItemCollapsibleState.None);
    this.description = `${session.shortId} · ${session.messages} msgs`;
    this.tooltip = `${session.title || '(untitled)'}\n${session.project || '(unknown project)'}\n${session.messages} messages`;
    this.contextValue = 'session';
    this.iconPath = new vscode.ThemeIcon('comment-discussion');
    // A plain click opens the same action menu as right-click, for non-CLI users.
    this.command = { command: 'claude-teleport.itemMenu', title: 'Actions', arguments: [this] };
  }
}

class SessionsProvider implements vscode.TreeDataProvider<SessionNode> {
  private readonly changed = new vscode.EventEmitter<void>();
  readonly onDidChangeTreeData = this.changed.event;

  refresh(): void {
    this.changed.fire();
  }

  getTreeItem(node: SessionNode): vscode.TreeItem {
    return node;
  }

  async getChildren(): Promise<SessionNode[]> {
    try {
      const sessions = await listSessions();
      return sessions.map((s) => new SessionNode(s));
    } catch (e) {
      await handleCliError(e);
      return []; // the view's welcome content then offers Refresh / Install
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
  if (action.act === 'send') {
    runInTerminal('Teleport · send', ['send', s.id, ...configArgs()]);
  } else if (action.act === 'share') {
    runInTerminal('Teleport · share', ['share', s.id, ...configArgs()]);
  } else {
    await vscode.env.clipboard.writeText(s.id);
    vscode.window.showInformationMessage(`Copied session id ${s.shortId}.`);
  }
}

async function cmdMenu(): Promise<void> {
  const pick = await vscode.window.showQuickPick(
    [
      { label: '$(radio-tower) Send a session by code', cmd: 'claude-teleport.send' },
      { label: '$(inbox) Receive a session', cmd: 'claude-teleport.receive' },
      { label: '$(file) Share a session to a file', cmd: 'claude-teleport.share' },
      { label: '$(list-unordered) Browse sessions', cmd: 'claude-teleport.sessions' },
    ] as (vscode.QuickPickItem & { cmd: string })[],
    { placeHolder: 'claude-teleport' },
  );
  if (pick) {
    vscode.commands.executeCommand(pick.cmd);
  }
}

export function activate(context: vscode.ExtensionContext): void {
  const status = vscode.window.createStatusBarItem(vscode.StatusBarAlignment.Left, 0);
  status.text = '$(radio-tower) teleport';
  status.tooltip = 'claude-teleport — hand a Claude Code session to a teammate';
  status.command = 'claude-teleport.menu';
  status.show();

  const provider = new SessionsProvider();
  const view = vscode.window.createTreeView('claudeTeleport.sessions', { treeDataProvider: provider });

  // "Browse sessions" now just reveals the sidebar instead of a quick pick.
  const revealSidebar = () => vscode.commands.executeCommand('claudeTeleport.sessions.focus');

  context.subscriptions.push(
    status,
    view,
    vscode.commands.registerCommand('claude-teleport.send', cmdSend),
    vscode.commands.registerCommand('claude-teleport.receive', cmdReceive),
    vscode.commands.registerCommand('claude-teleport.share', cmdShare),
    vscode.commands.registerCommand('claude-teleport.sessions', revealSidebar),
    vscode.commands.registerCommand('claude-teleport.menu', cmdMenu),
    vscode.commands.registerCommand('claude-teleport.refresh', () => provider.refresh()),
    vscode.commands.registerCommand('claude-teleport.itemMenu', (n: SessionNode) => itemMenu(n)),
    vscode.commands.registerCommand('claude-teleport.sendItem', (n: SessionNode) =>
      runInTerminal('Teleport · send', ['send', n.session.id, ...configArgs()]),
    ),
    vscode.commands.registerCommand('claude-teleport.shareItem', (n: SessionNode) =>
      runInTerminal('Teleport · share', ['share', n.session.id, ...configArgs()]),
    ),
    vscode.commands.registerCommand('claude-teleport.copyItemId', async (n: SessionNode) => {
      await vscode.env.clipboard.writeText(n.session.id);
      vscode.window.showInformationMessage(`Copied session id ${n.session.shortId}.`);
    }),
  );
}

export function deactivate(): void {
  /* nothing to clean up */
}
