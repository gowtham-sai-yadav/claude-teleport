// Dependency-free checks for the extension. Runs without a VS Code runtime by
// stubbing the `vscode` module, so it can run in CI or a plain terminal:
//
//   npm test
//
// It verifies two things that break extensions in practice:
//   1) manifest wiring  — every declared command has a handler; every menu and
//      TreeItem command points at a real, registered command.
//   2) activation       — activate() runs without throwing and registers the
//      status bar, the sessions view, and all commands.

const assert = require('assert');
const path = require('path');
const Module = require('module');

// ---- record what the extension does against a stubbed vscode API ----------
const calls = { commands: [], statusBars: 0, treeViews: [] };

const disposable = { dispose() {} };
const vscodeStub = {
  StatusBarAlignment: { Left: 1, Right: 2 },
  TreeItemCollapsibleState: { None: 0, Collapsed: 1, Expanded: 2 },
  ProgressLocation: { Notification: 15, Window: 10 },
  ThemeIcon: class { constructor(id) { this.id = id; } },
  TreeItem: class { constructor(label, state) { this.label = label; this.collapsibleState = state; } },
  EventEmitter: class { constructor() { this.event = () => disposable; } fire() {} },
  Uri: { parse: (s) => ({ toString: () => s }) },
  env: { openExternal() {}, clipboard: { writeText: async () => {} } },
  commands: {
    registerCommand(id) { calls.commands.push(id); return disposable; },
    executeCommand: async () => {},
  },
  window: {
    createStatusBarItem() { calls.statusBars++; return { text: '', tooltip: '', command: '', show() {}, dispose() {} }; },
    createTreeView(id) { calls.treeViews.push(id); return { dispose() {} }; },
    showQuickPick: async () => undefined,
    showInputBox: async () => undefined,
    showInformationMessage: async () => undefined,
    showErrorMessage: async () => undefined,
    withProgress: async (_opts, task) => task({ report() {} }),
    createTerminal() { return { show() {}, sendText() {}, dispose() {} }; },
  },
  workspace: {
    getConfiguration() { return { get: (k) => (k === 'path' ? 'claude-teleport' : '') }; },
  },
};

const origLoad = Module._load;
Module._load = function (request) {
  if (request === 'vscode') return vscodeStub;
  return origLoad.apply(this, arguments);
};

// ---- load manifest + compiled extension -----------------------------------
const pkg = require(path.join(__dirname, '..', 'package.json'));
const ext = require(path.join(__dirname, '..', 'out', 'extension.js'));

// ---- 1) activation ---------------------------------------------------------
const subscriptions = [];
const fakeContext = { subscriptions, globalStorageUri: { fsPath: require('os').tmpdir() + '/ct-ext-test' } };
assert.doesNotThrow(() => ext.activate(fakeContext), 'activate() threw');
assert.doesNotThrow(() => ext.deactivate(), 'deactivate() threw');

const registered = new Set(calls.commands);
assert.strictEqual(calls.statusBars, 1, 'expected exactly one status bar item');
assert.ok(calls.treeViews.includes('claudeTeleport.sessions'), 'sessions tree view was not created');
assert.ok(subscriptions.length >= registered.size + 2, 'not everything was pushed to context.subscriptions');

// ---- 2) manifest wiring ----------------------------------------------------
const declared = new Set(pkg.contributes.commands.map((c) => c.command));

// every declared command must have a registered handler
for (const id of declared) {
  assert.ok(registered.has(id), `command "${id}" is declared in package.json but never registered`);
}

// every command referenced in any menu must be declared (so it has a title)
const menus = pkg.contributes.menus || {};
for (const [menu, items] of Object.entries(menus)) {
  for (const item of items) {
    assert.ok(declared.has(item.command), `menu "${menu}" references undeclared command "${item.command}"`);
  }
}

// config keys the code reads must exist
const props = pkg.contributes.configuration.properties;
for (const key of ['claude-teleport.path', 'claude-teleport.configDir']) {
  assert.ok(props[key], `config property "${key}" is used but not declared`);
}

// registered-but-undeclared commands are only allowed for known internal ones
const internalOK = new Set(['claude-teleport.itemMenu']);
for (const id of registered) {
  if (!declared.has(id)) {
    assert.ok(internalOK.has(id), `command "${id}" is registered but not declared (and not a known internal)`);
  }
}

console.log(`OK  activate registered ${registered.size} commands, 1 status bar, view [${calls.treeViews.join(', ')}]`);
console.log(`OK  ${declared.size} declared commands all have handlers; all menu refs resolve`);
console.log('PASS');
