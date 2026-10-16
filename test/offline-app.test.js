const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const vm = require('node:vm');
const OfflineCRUD = require('../static/offline-crud.js');

function deferred() {
    let resolve;
    const promise = new Promise(done => { resolve = done; });
    return { resolve, promise };
}

function harness() {
    const timers = new Map();
    let timerID = 0;
    const location = { pathname: '/lists/1', search: '', href: '', protocol: 'http:', host: 'localhost' };
    const window = {
        shoppingList: () => ({ init() {}, destroy() {}, request() {} }),
        homePage: () => ({}),
        addEventListener() {}, removeEventListener() {}
    };
    const document = {
        visibilityState: 'visible', querySelector: () => null,
        addEventListener() {}, removeEventListener() {}
    };
    const context = vm.createContext({
        window, document, location, navigator: { onLine: false },
        URLSearchParams, console, confirm: () => true, t: key => key,
        setTimeout(callback, delay) { const id = ++timerID; timers.set(id, { callback, delay }); return id; },
        clearTimeout(id) { timers.delete(id); }
    });
    vm.runInContext(fs.readFileSync(require.resolve('../static/offline-app.js'), 'utf8'), context);
    const app = window.shoppingList();
    app.renderCRUD = () => {};
    return { app, context, timers };
}

async function modelHarness() {
    const h = harness();
    const metadata = new Map();
    const storage = {
        async getMetadata(key) { return structuredClone(metadata.get(key)); },
        async setMetadata(key, value) { metadata.set(key, structuredClone(value)); }
    };
    h.app._crud = new OfflineCRUD({ storage, request: async () => { throw Error('Network must not be needed for local editing'); } });
    await h.app._crud.seed({
        lists: [{ id: 1, name: 'Groceries', icon: '🛒', sort_order: 0, show_completed: true }],
        sections: [{ id: 2, list_id: 1, name: 'Produce', sort_order: 0 }, { id: 3, list_id: 1, name: 'Other', sort_order: 1 }],
        items: [{ id: 7, section_id: 2, name: 'Milk', description: '', quantity: 1, sort_order: 0, completed: false, uncertain: false }]
    });
    h.app._crudReady = true;
    h.app.initCRUD = async () => {};
    return h;
}

test('shopping UI completion, editing, moving, quantity and deletion use durable offline mutations', async () => {
    const { app } = await modelHarness();
    await app.toggleItem(7);
    assert.equal(app.crudFind('item', 7).completed, true);
    app.editingItem = { id: 7 };
    app.editItemName = ' Oat milk ';
    app.editItemDescription = ' Unsweetened ';
    app.editItemQuantity = 3;
    await app.submitEditItem();
    assert.equal(app.crudFind('item', 7).name, 'Oat milk');
    assert.equal(app.crudFind('item', 7).description, 'Unsweetened');
    assert.equal(app.editingItem, null);
    await app.moveItemDesktop(7, 2, 3);
    assert.equal(app.crudFind('item', 7).section_id, 3);
    app.mobileActionItem = { id: 7, quantity: 3 };
    await app.adjustQuantity(1);
    assert.equal(app.crudFind('item', 7).quantity, 4);
    await app.toggleUncertainFetch(7);
    assert.equal(app.crudFind('item', 7).uncertain, true);
    await app.deleteItemDirect(7);
    assert.equal(app.crudFind('item', 7), undefined);
    assert.equal(app._crud.pendingCount, 6);
});

test('new temporary list, section and item remain editable before synchronization', async () => {
    const { app } = await modelHarness();
    const list = await app.crudMutate('list', 'create', undefined, { name: 'Weekend' });
    const section = await app.crudMutate('section', 'create', undefined, { list_id: list.entity_id, name: 'Produce' });
    await app.crudAddItem(section.entity_id, 'Apples');
    const item = app.crudState().items.find(value => value.name === 'Apples');
    assert.ok(list.entity_id < 0 && section.entity_id < 0 && item.id < 0);
    await app.toggleItem(item.id);
    await app.crudUpdateSection(section.entity_id, 'Fruit');
    await app.crudUpdateList(list.entity_id, 'Weekend shopping', '🍎');
    assert.equal(app.crudFind('item', item.id).completed, true);
    assert.equal(app.crudFind('section', section.entity_id).name, 'Fruit');
    assert.equal(app.crudFind('list', list.entity_id).icon, '🍎');
});

test('a realtime event during a snapshot refresh triggers another refresh instead of being lost', async () => {
    const { app, context } = harness();
    context.navigator.onLine = true;
    app._crudReady = true;
    const first = deferred();
    let refreshes = 0;
    app._crud = { pendingCount: 0, async sync() { if (++refreshes === 1) await first.promise; } };
    const refreshing = app.fullRefresh();
    app.handleMessage(JSON.stringify({ type: 'item_toggled', data: { id: 7, section_id: 2, completed: true } }));
    first.resolve();
    await refreshing;
    // A follow-up may be launched in the completion callback rather than awaited
    // by the first caller, but it must start without needing another user action.
    await Promise.resolve();
    await Promise.resolve();
    assert.ok(refreshes >= 2, 'a snapshot requested before the event cannot satisfy the event');
});

test('returning online resumes the socket paused by offline mode', async () => {
    const { app, context } = harness();
    context.window.offlineStorage = { async init() {}, async getQueuedActions() { return []; } };
    context.window.OfflineCRUD = class {
        pendingCount = 0;
        async init() {}
        async seed() {}
    };
    app.fullRefresh = async () => {};
    let socketRunning = true;
    app._realtime = {
        pause() { socketRunning = false; },
        reconnect() { socketRunning = true; },
        start() { socketRunning = true; }
    };
    app.connect = () => { socketRunning = true; };
    await app.initCRUD();
    app._crudOffline();
    assert.equal(socketRunning, false);
    context.navigator.onLine = true;
    app._crudOnline();
    assert.equal(socketRunning, true, 'data refresh alone does not resume a paused realtime connection');
});

test('opening a shared list URL does not redirect using an older local snapshot', () => {
    const { app, context } = harness();
    app.renderCRUD = context.window.shoppingList().renderCRUD;
    app._crudReady = true;
    app._crud = { getState: () => ({ lists: [{ id: 1 }], sections: [], items: [] }) };
    context.location.pathname = '/lists/7';
    app.renderCRUD();
    assert.equal(context.location.href, '', 'the first server refresh must be allowed to discover list 7');
    app._crudSyncedOnce = true;
    app.renderCRUD();
    assert.equal(context.location.href, '/', 'a confirmed missing list may return to the overview');
});

test('offline list shell resolves the requested URL rather than stale document list metadata', () => {
    const { app, context } = harness();
    context.document.querySelector = () => ({ dataset: { listId: '1' } });
    context.location.pathname = '/lists/7';
    assert.equal(app.currentListId(), 7);
    context.location.pathname = '/offline/list';
    context.location.search = '?list_id=-9';
    assert.equal(app.currentListId(), -9);
    app._crud = { resolveID: (entity, id) => entity === 'list' && id === -9 ? 17 : id };
    assert.equal(app.currentListId(), 17, 'temporary list URLs follow persisted server ID aliases');
});

test('the move confirmation toast escapes list names because Toast renders HTML', async () => {
    const { app, context } = harness();
    const shown = [];
    context.window.OfflineView = require('../static/offline-view.js');
    context.window.Toast = { show: (message, type) => shown.push({ message, type }) };
    context.t = (key, params) => `${key}:${params.list}`;
    app.initCRUD = async () => {};
    app.crudState = () => ({ lists: [{ id: 2, name: '<img src=x onerror=alert(1)>' }], sections: [{ id: 5, list_id: 2 }], items: [] });
    let moved;
    app.crudMoveItem = async (id, sectionId) => { moved = [id, sectionId]; };
    await app.moveItemToList(9, 5);
    assert.deepEqual(moved, [9, 5]);
    assert.equal(shown.length, 1);
    assert.equal(shown[0].type, 'success');
    assert.ok(!shown[0].message.includes('<img'), shown[0].message);
    assert.ok(shown[0].message.includes('&lt;img src=x onerror=alert(1)&gt;'));
});

test('the mobile sheet closes and moves the open item to the chosen section of another list', async () => {
    const { app, context } = harness();
    context.window.OfflineView = require('../static/offline-view.js');
    context.window.Toast = { show() {} };
    app.initCRUD = async () => {};
    app.crudState = () => ({ lists: [{ id: 2, name: 'Lidl' }], sections: [{ id: 5, list_id: 2 }], items: [] });
    const moves = [];
    app.crudMoveItem = async (id, sectionId) => { moves.push([id, sectionId]); };
    app.mobileActionItem = null;
    await app.moveToListSection(5);
    assert.deepEqual(moves, [], 'nothing happens without an open item');
    app.mobileActionItem = { id: 9, section_id: 1 };
    await app.moveToListSection(5);
    assert.deepEqual(moves, [[9, 5]]);
    assert.equal(app.mobileActionItem, null);
});

test('list move menus follow the destinations: rows and the mobile sheet are refilled and hidden when empty', () => {
    const { app, context } = harness();
    context.window.OfflineView = require('../static/offline-view.js');
    const element = (dataset = {}) => {
        const hidden = new Set();
        const menu = { innerHTML: '' };
        return { dataset, menu, hidden, classList: { toggle: (name, on) => on ? hidden.add(name) : hidden.delete(name) }, querySelector: () => menu };
    };
    const row = element({ itemId: '7' });
    const sheet = element();
    const options = { innerHTML: '' };
    context.document.querySelectorAll = selector => selector === '.item-move-list' ? [row] : [];
    context.document.getElementById = id => id === 'mobile-move-list' ? sheet : id === 'mobile-move-list-options' ? options : null;
    const destinations = [{ id: 2, name: 'Lidl', icon: '🥬', sections: [{ id: 5, name: 'Dairy' }] }];
    app.refreshListMoveMenus(destinations);
    assert.ok(!row.hidden.has('hidden') && !sheet.hidden.has('hidden'));
    assert.ok(row.menu.innerHTML.includes('moveItemToList(7,5)'));
    assert.ok(options.innerHTML.includes('moveToListSection(5)'));
    app.refreshListMoveMenus([]);
    assert.ok(row.hidden.has('hidden') && sheet.hidden.has('hidden'));
    assert.equal(row.menu.innerHTML, '');
    assert.equal(options.innerHTML, '');
});

test('a completion-only render preserves its row and applies authoritative stats exactly once', () => {
    const { app, context } = harness();
    const item = { id: 7, section_id: 2, name: 'Milk', sort_order: 0, completed: true };
    const section = { id: 2, list_id: 1, name: 'Produce', sort_mode: 'manual', sort_order: 0 };
    app._crudReady = true;
    app._crud = { getState: () => ({ lists: [{ id: 1, show_completed: true }], sections: [section], items: [item] }) };
    app._crudRenderedItems = new Map([[7, { ...item, completed: false }]]);
    const row = { dataset: { itemId: '7' }, nextElementSibling: null };
    const target = done => ({
        firstElementChild: done ? null : row,
        classList: { contains: name => name === 'completed-items' && done },
        appendChild(child) { child.parentNode = this; this.firstElementChild = child; },
        insertBefore(child) { child.parentNode = this; this.firstElementChild = child; }
    });
    const active = target(false), bought = target(true);
    row.parentNode = active;
    const sectionElement = {
        dataset: { sectionId: '2', crudSignature: JSON.stringify(['Produce', 'manual', true]) },
        querySelector(selector) {
            return selector === '.active-items' ? active : selector === '.completed-items' ? bought : null;
        },
        querySelectorAll: selector => selector === '.active-items, .completed-items' ? [active, bought] : [row]
    };
    const container = { firstElementChild: sectionElement, querySelectorAll: () => [sectionElement] };
    const elements = { 'sections-list': container, 'section-2': sectionElement, 'item-7': row };
    context.document.querySelector = () => ({ dataset: { listId: '1' }, querySelector: () => null });
    context.document.getElementById = id => elements[id] || null;
    context.document.querySelectorAll = () => [];
    context.window.OfflineView = require('../static/offline-view.js');
    context.window.checkEmptyStates = () => {};
    let toggles = 0;
    let menuUpdates = 0;
    app.stats = { total: 1, completed: 0, percentage: 0 };
    app._applyOfflineToggle = () => { toggles++; app.stats.completed++; bought.appendChild(row); };
    app.updateSectionCounter = app.updateCompletedCount = app.updateCompletedVisibility = app.updateSectionSelects = () => {};
    app.refreshAllMoveMenus = () => { menuUpdates++; };
    app.$nextTick = callback => callback();
    app.initMobileSortable = () => {};
    app.renderCRUD = context.window.shoppingList().renderCRUD;
    app.renderCRUD();
    assert.equal(toggles, 1);
    assert.equal(row.parentNode, bought);
    assert.equal(app.stats.completed, 1);
    assert.equal(app.stats.total, 1);
    assert.equal(app.stats.percentage, 100);
    assert.equal(menuUpdates, 1, 'section changes also update existing item move destinations');
});

test('bulk completion and reordering publish only their final durable state', async () => {
    const { app } = await modelHarness();
    await app.crudAddItem(2, 'Bread');
    const notifications = [];
    app._crud.onChange = () => notifications.push(app.crudState());
    await app.toggleAllItems(2);
    assert.equal(notifications.length, 1);
    assert.ok(notifications[0].items.every(item => item.completed));
    notifications.length = 0;
    await app.crudMoveSection(3, 'up');
    assert.equal(notifications.length, 1);
    assert.equal(app.crudFind('section', 3).sort_order, 0);
    assert.equal(app.crudFind('section', 2).sort_order, 1);
});

test('legacy item creation ignores transport-only fields and clears only durable imports', async () => {
    const { app, context } = await modelHarness();
    const removed = [];
    context.window.offlineStorage = {
        getQueuedActions: async () => [{id:12,type:'create_item',method:'POST',url:'/items',body:'section_id=2&name=Legacy+bread&quantity=2&quick_add=true'}],
        clearAction: async id => removed.push(id)
    };
    await app.migrateLegacyCRUD();
    assert.equal(app.crudState().items.find(item => item.name === 'Legacy bread').quantity, 2);
    assert.deepEqual(removed, [12]);
    await app.migrateLegacyCRUD();
    assert.equal(app.crudState().items.filter(item => item.name === 'Legacy bread').length, 1);
});

test('an unsupported legacy action remains queued and does not leave the page hidden', async () => {
    const { app, context } = harness();
    context.window.offlineStorage = {async init() {},async getQueuedActions() {return [{id:12,type:'unknown_legacy_action'}];}};
    context.window.OfflineCRUD = class {
        pendingCount = 0;
        async init() {}
        async seed() {}
        getState() {return {lists:[],sections:[],items:[]};}
    };
    app.fullRefresh = async () => {};
    await app.initCRUD();
    assert.equal(app._crudReady, true);
    assert.equal(app._legacyMigrationBlocked, true);
    assert.equal(app.crudError, 'offline.sync_failed');
});

function earlyRenderHarness({ model = { pending: 0, snapshot_at: 1000 }, generatedAt = 2000 } = {}) {
    const h = harness();
    const drawn = [];
    const stored = new Map(model === null ? [] : [['koffan-offline-model', JSON.stringify(model)]]);
    h.context.localStorage = { getItem: key => stored.get(key) ?? null, setItem: (key, value) => stored.set(key, value) };
    h.context.window.OfflineCRUD = OfflineCRUD;
    h.context.window.offlineStorage = { init: () => new Promise(() => {}) };
    h.context.window.offlineBootstrap = { lists: [{ id: 1, name: 'Groceries' }], sections: [], items: [], generated_at: generatedAt };
    h.app.renderCRUD = state => drawn.push(state);
    h.app.initCRUD();
    return drawn;
}

test('a page snapshot at least as new as the stored model is drawn before the offline store opens', () => {
    const drawn = earlyRenderHarness();
    assert.equal(drawn.length, 1);
    assert.deepEqual(drawn[0].lists.map(list => list.name), ['Groceries']);
    assert.equal(earlyRenderHarness({ generatedAt: 1000 }).length, 1, 'the same snapshot as the stored model');
});

test('the page snapshot waits for the offline store when it may be outdated', () => {
    assert.equal(earlyRenderHarness({ model: null }).length, 0, 'unknown local state');
    assert.equal(earlyRenderHarness({ model: { pending: 2, snapshot_at: 1000 } }).length, 0, 'unsynced local changes');
    assert.equal(earlyRenderHarness({ generatedAt: 500 }).length, 0, 'an older copy, such as a cached offline page');
    assert.equal(earlyRenderHarness({ generatedAt: null }).length, 0, 'a page cached before snapshots carried their time');
});
