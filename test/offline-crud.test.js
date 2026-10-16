const test = require('node:test');
const assert = require('node:assert/strict');
const OfflineCRUD = require('../static/offline-crud.js');

const copy = value => structuredClone(value);
const blank = () => ({ lists: [], sections: [], items: [] });
const base = () => ({
    lists: [{ id: 1, name: 'Shopping', show_completed: true }],
    sections: [{ id: 2, list_id: 1, name: 'Food', sort_order: 0, sort_mode: 'manual' }],
    items: [{ id: 3, section_id: 2, name: 'Milk', description: '', completed: false, quantity: 1, sort_order: 0 }]
});
function deferred() {
    let resolve;
    const promise = new Promise(done => { resolve = done; });
    return { promise, resolve };
}
function storageFixture() {
    let stored;
    let failWrites = false;
    return {
        async getMetadata(key) { assert.equal(key, 'offline_crud'); return stored && copy(stored); },
        async setMetadata(key, value) {
            assert.equal(key, 'offline_crud');
            if (failWrites) throw new Error('Storage quota exceeded');
            stored = copy(value);
        },
        read() { return copy(stored); },
        fail() { failWrites = true; },
        recover() { failWrites = false; }
    };
}
const ok = value => ({ ok: true, status: 200, json: async () => copy(value) });
function ack(body, snapshot, ids = {}) {
    return ok({
        snapshot,
        results: body.operations.map(operation => ({
            id: operation.id, entity: operation.entity, entity_id: operation.entity_id,
            server_id: operation.entity_id < 0 ? ids[operation.entity_id] : operation.entity_id
        }))
    });
}
async function model({ snapshot = base(), storage = storageFixture(), request = async () => ok(base()), onChange, onError } = {}) {
    const client = new OfflineCRUD({ storage, request, onChange, onError });
    await client.init();
    if (snapshot) await client.seed(snapshot);
    return { client, storage };
}

test('a rejected atomic write does not change visible state, queue or notify observers', async () => {
    let changes = 0;
    const { client, storage } = await model({ onChange() { changes++; } });
    const before = storage.read();
    const beforeChanges = changes;
    storage.fail();
    await assert.rejects(client.mutate({ entity: 'item', action: 'update', entity_id: 3, values: { completed: true } }), /quota/);
    assert.deepEqual(client.getState(), before.state);
    assert.deepEqual(storage.read(), before);
    assert.equal(client.pendingCount, 0);
    assert.equal(changes, beforeChanges);
    storage.recover();
    await client.mutate({ entity: 'item', action: 'update', entity_id: 3, values: { completed: true } });
    assert.equal(client.pendingCount, 1, 'a failed write does not poison subsequent transactions');
});

test('new parent and child chains remain usable offline and map to real IDs after sync', async () => {
    let transmitted;
    const snapshot = {
        lists: [{ id: 10, name: 'Weekend' }],
        sections: [{ id: 20, list_id: 10, name: 'Fruit' }],
        items: [{ id: 30, section_id: 20, name: 'Apple', completed: false }]
    };
    const { client } = await model({ snapshot: blank(), request: async (_url, options) => {
        transmitted = JSON.parse(options.body);
        return ack(transmitted, snapshot, { '-1': 10, '-2': 20, '-3': 30 });
    } });
    const list = await client.mutate({ entity: 'list', action: 'create', values: { name: 'Weekend' } });
    const section = await client.mutate({ entity: 'section', action: 'create', values: { name: 'Fruit', list_id: list.entity_id } });
    const item = await client.mutate({ entity: 'item', action: 'create', values: { name: 'Apple', section_id: section.entity_id } });
    assert.deepEqual([list.entity_id, section.entity_id, item.entity_id], [-1, -2, -3]);
    assert.equal(client.getState().items[0].section_id, -2);
    await client.sync();
    assert.equal(transmitted.operations[1].values.list_id, -1);
    assert.equal(transmitted.operations[2].values.section_id, -2);
    assert.deepEqual(client.getState(), snapshot);
    assert.equal(client.resolveID('item', -3), 30);
    assert.equal(client.pendingCount, 0);
});

test('creating, editing and deleting an item offline replays each intent in order', async () => {
    let transmitted;
    const { client } = await model({ request: async (_url, options) => {
        transmitted = JSON.parse(options.body);
        return ack(transmitted, base(), { '-1': 8 });
    } });
    const created = await client.mutate({ entity: 'item', action: 'create', values: { name: 'Bread', section_id: 2 } });
    await client.mutate({ entity: 'item', action: 'update', entity_id: created.entity_id, values: { completed: true, name: 'Wholemeal bread', quantity: 2 } });
    await client.mutate({ entity: 'item', action: 'delete', entity_id: created.entity_id });
    assert.equal(client.getState().items.length, 1);
    await client.sync();
    assert.deepEqual(transmitted.operations.map(operation => operation.action), ['create', 'update', 'delete']);
    assert.equal(client.pendingCount, 0);
    assert.deepEqual(client.getState(), base());
});

test('section and list deletions cascade locally without deleting another list', async () => {
    const snapshot = base();
    snapshot.lists.push({ id: 4, name: 'Other' });
    snapshot.sections.push({ id: 5, list_id: 4, name: 'Other section' });
    snapshot.items.push({ id: 6, section_id: 5, name: 'Keep' });
    const { client } = await model({ snapshot });
    await client.mutate({ entity: 'section', action: 'delete', entity_id: 2 });
    assert.deepEqual(client.getState().items.map(item => item.id), [6]);
    await client.mutate({ entity: 'list', action: 'delete', entity_id: 4 });
    assert.deepEqual(client.getState().lists.map(list => list.id), [1]);
    assert.deepEqual(client.getState().sections, []);
    assert.deepEqual(client.getState().items, []);
});

test('item moves, quantity, uncertainty and section sort edits persist through reload', async () => {
    const snapshot = base();
    snapshot.sections.push({ id: 4, list_id: 1, name: 'Second' });
    const { client, storage } = await model({ snapshot });
    await client.mutate({ entity: 'item', action: 'update', entity_id: 3, values: { section_id: 4, quantity: 5, uncertain: true, completed: true, sort_order: 8 } });
    await client.mutate({ entity: 'section', action: 'update', entity_id: 4, values: { sort_mode: 'alphabetical', name: 'Produce' } });
    const reloaded = new OfflineCRUD({ storage, request: async () => ok(base()) });
    await reloaded.init();
    assert.equal(reloaded.clientId, client.clientId);
    assert.deepEqual(reloaded.getPendingOperations(), client.getPendingOperations());
    assert.deepEqual(reloaded.getState(), client.getState());
    assert.equal(reloaded.getState().items[0].section_id, 4);
});

test('lost acknowledgement retries the same client, UUIDs and exact operation payload after reload', async () => {
    const requests = [];
    const storage = storageFixture();
    const request = async (_url, options) => {
        const body = JSON.parse(options.body);
        requests.push(body);
        if (requests.length === 1) throw new TypeError('Response lost after server commit');
        return ack(body, base(), { '-1': 8 });
    };
    const { client } = await model({ storage, request });
    await client.mutate({ entity: 'item', action: 'create', values: { name: 'Bread', section_id: 2 } });
    await assert.rejects(client.sync(), /Response lost/);
    const reloaded = new OfflineCRUD({ storage, request });
    await reloaded.init();
    await reloaded.sync();
    assert.deepEqual(requests[1], requests[0]);
    assert.equal(reloaded.pendingCount, 0);
});

test('a tap can persist during a blocked network request and is replayed after its acknowledgement', async () => {
    const started = deferred();
    const release = deferred();
    const sent = [];
    const { client } = await model({ request: async (_url, options) => {
        const body = JSON.parse(options.body);
        sent.push(body);
        const snapshot = base();
        snapshot.items[0].completed = sent.length === 1;
        if (sent.length === 1) { started.resolve(); await release.promise; }
        return ack(body, snapshot);
    } });
    await client.mutate({ entity: 'item', action: 'update', entity_id: 3, values: { completed: true } });
    const sync = client.sync();
    await started.promise;
    await client.mutate({ entity: 'item', action: 'update', entity_id: 3, values: { completed: false } });
    assert.equal(client.pendingCount, 2);
    release.resolve();
    await sync;
    assert.deepEqual(sent.map(batch => batch.operations[0].values.completed), [true, false]);
    assert.equal(client.getState().items[0].completed, false);
    assert.equal(client.pendingCount, 0);
});

test('unsent child references created during a blocked request remap after parent acknowledgement', async () => {
    const started = deferred();
    const release = deferred();
    const sent = [];
    const { client } = await model({ snapshot: blank(), request: async (_url, options) => {
        const body = JSON.parse(options.body);
        sent.push(body);
        const snapshot = { lists: [{ id: 10, name: 'New list' }], sections: [], items: [] };
        if (sent.length === 1) { started.resolve(); await release.promise; }
        else snapshot.sections.push({ id: 20, list_id: 10, name: 'New section' });
        return ack(body, snapshot, { '-1': 10, '-2': 20 });
    } });
    const parent = await client.mutate({ entity: 'list', action: 'create', values: { name: 'New list' } });
    const sync = client.sync();
    await started.promise;
    await client.mutate({ entity: 'section', action: 'create', values: { name: 'New section', list_id: parent.entity_id } });
    release.resolve();
    await sync;
    assert.equal(sent[1].operations[0].values.list_id, 10);
    assert.equal(client.getState().sections[0].list_id, 10);
});

test('refresh combines another shopper changes with pending local edits', async () => {
    const remote = base();
    remote.items[0].completed = true;
    remote.items[0].quantity = 4;
    const { client } = await model({ request: async () => ok(remote) });
    await client.mutate({ entity: 'item', action: 'update', entity_id: 3, values: { description: 'Unsweetened' } });
    await client.refresh();
    const item = client.getState().items[0];
    assert.equal(item.completed, true);
    assert.equal(item.quantity, 4);
    assert.equal(item.description, 'Unsweetened');
    assert.equal(client.pendingCount, 1);
});

for (const status of [401, 403, 409, 503, 'redirect']) {
    test(`sync failure ${status} preserves operations and surfaces the failure`, async () => {
        const errors = [];
        const { client } = await model({ onError: cause => errors.push(cause), request: async () => ({
            ok: status === 'redirect', status: status === 'redirect' ? 200 : status, redirected: status === 'redirect',
            json: async () => ({ error: 'Cannot apply operation', operation_id: 'failed-op' })
        }) });
        const operation = await client.mutate({ entity: 'item', action: 'update', entity_id: 3, values: { completed: true } });
        await assert.rejects(client.sync(), /Cannot apply/);
        assert.deepEqual(client.getPendingOperations(), [operation]);
        assert.equal(client.getState().items[0].completed, true);
        assert.equal(errors.at(-1).operationId, 'failed-op');
        assert.equal(errors.at(-1).status, status === 'redirect' ? 401 : status);
    });
}

test('a snapshot that removed an edited item retains the pending conflicting edit for resolution', async () => {
    const remote = base();
    remote.items = [];
    const { client } = await model({ request: async () => ok(remote) });
    await client.mutate({ entity: 'item', action: 'update', entity_id: 3, values: { name: 'Oat milk' } });
    await client.refresh();
    assert.equal(client.pendingCount, 1);
    assert.equal(client.getState().items[0].name, 'Oat milk');
});

test('invalid mutations do not enter durable storage', async () => {
    const { client, storage } = await model();
    const before = storage.read();
    for (const input of [
        { entity: 'item', action: 'create', values: { name: '', section_id: 2 } },
        { entity: 'item', action: 'create', values: { name: 'Milk', section_id: 999 } },
        { entity: 'item', action: 'update', entity_id: 3, values: { quantity: -1 } },
        { entity: 'item', action: 'update', entity_id: 3, values: { completed: 'true' } },
        { entity: 'section', action: 'update', entity_id: 2, values: { sort_mode: 'random' } }
    ]) await assert.rejects(client.mutate(input));
    assert.deepEqual(storage.read(), before);
});

test('migration deduplication survives reload and returns the originally assigned temporary ID', async () => {
    const { client, storage } = await model();
    const input = { legacy_id: 19, entity: 'item', action: 'create', values: { name: 'Bread', section_id: 2 } };
    const first = await client.mutate(input);
    const reloaded = new OfflineCRUD({ storage, request: async () => ok(base()) });
    await reloaded.init();
    const repeated = await reloaded.mutate(input);
    assert.deepEqual(repeated, first);
    assert.equal(reloaded.pendingCount, 1);
    assert.equal(reloaded.getState().items.filter(item => item.name === 'Bread').length, 1);
});

test('discarding a failed new parent removes its dependent operations but preserves independent work', async () => {
    const { client } = await model({ snapshot: blank() });
    const parent = await client.mutate({ entity: 'list', action: 'create', values: { name: 'First' } });
    const section = await client.mutate({ entity: 'section', action: 'create', values: { name: 'Food', list_id: parent.entity_id } });
    const item = await client.mutate({ entity: 'item', action: 'create', values: { name: 'Bread', section_id: section.entity_id } });
    const independent = await client.mutate({ entity: 'list', action: 'create', values: { name: 'Keep' } });
    const removed = await client.discardOperation(parent.id);
    assert.deepEqual(removed, [parent.id, section.id, item.id]);
    assert.deepEqual(client.getPendingOperations(), [independent]);
    assert.deepEqual(client.getState().lists.map(list => list.name), ['Keep']);
    assert.deepEqual(client.getState().sections, []);
});

test('bootstrap seed cannot replace established cached data or pending changes', async () => {
    const { client } = await model();
    assert.equal(await client.seed(blank()), false);
    assert.deepEqual(client.getState(), base());
    const fresh = await model({ snapshot: null });
    await fresh.client.mutate({ entity: 'list', action: 'create', values: { name: 'Offline' } });
    assert.equal(await fresh.client.seed(base()), false);
    assert.equal(fresh.client.getState().lists[0].name, 'Offline');
});

test('returned state is isolated from external mutation', async () => {
    const { client } = await model();
    client.getState().items[0].name = 'Changed outside model';
    assert.equal(client.getState().items[0].name, 'Milk');
});

test('partially acknowledged operations already sent keep their exact payload for deduplicated retry', async () => {
    const bodies = [];
    const { client } = await model({ snapshot: blank(), request: async (_url, options) => {
        const body = JSON.parse(options.body);
        bodies.push(body);
        if (bodies.length === 1) {
            const parent = body.operations[0];
            return ok({
                results: [{ id: parent.id, entity: parent.entity, entity_id: parent.entity_id, server_id: 10 }],
                snapshot: { lists: [{ id: 10, name: 'List' }], sections: [], items: [] }
            });
        }
        return ack(body, { lists: [{ id: 10, name: 'List' }], sections: [{ id: 20, list_id: 10, name: 'Section' }], items: [] }, { '-2': 20 });
    } });
    const parent = await client.mutate({ entity: 'list', action: 'create', values: { name: 'List' } });
    await client.mutate({ entity: 'section', action: 'create', values: { name: 'Section', list_id: parent.entity_id } });
    await client.sync();
    assert.deepEqual(bodies[1].operations[0], bodies[0].operations[1]);
    assert.equal(client.getState().sections[0].list_id, 10);
});

test('snapshot refresh and queue sync serialize even when Web Locks are unavailable', async () => {
    const started = deferred();
    const release = deferred();
    const methods = [];
    const { client } = await model({ request: async (_url, options) => {
        methods.push(options.method);
        if (options.method === 'GET') { started.resolve(); await release.promise; return ok(base()); }
        const snapshot = base();
        snapshot.items[0].completed = true;
        return ack(JSON.parse(options.body), snapshot);
    } });
    const refreshing = client.refresh();
    await started.promise;
    await client.mutate({ entity: 'item', action: 'update', entity_id: 3, values: { completed: true } });
    const syncing = client.sync();
    await Promise.resolve();
    assert.deepEqual(methods, ['GET']);
    release.resolve();
    await Promise.all([refreshing, syncing]);
    assert.deepEqual(methods, ['GET', 'POST']);
    assert.equal(client.getState().items[0].completed, true);
});

test('a failed acknowledgement write leaves the same operations retryable', async () => {
    const storage = storageFixture();
    const sent = [];
    const { client } = await model({ storage, request: async (_url, options) => {
        const body = JSON.parse(options.body);
        sent.push(body);
        if (sent.length === 1) storage.fail();
        return ack(body, base());
    } });
    const operation = await client.mutate({ entity: 'item', action: 'update', entity_id: 3, values: { completed: true } });
    await assert.rejects(client.sync(), /quota/);
    assert.deepEqual(client.getPendingOperations(), [operation]);
    storage.recover();
    await client.sync();
    assert.deepEqual(sent[1], sent[0]);
    assert.equal(client.pendingCount, 0);
});

test('independent tabs share client identity and serialize temporary IDs and mutations with Web Locks', async () => {
    const vm = require('node:vm');
    const fs = require('node:fs');
    const locks = new Map();
    const lockManager = { request(name, options, callback) {
        callback ||= options;
        const pending = (locks.get(name) || Promise.resolve()).then(callback);
        locks.set(name, pending.catch(() => {}));
        return pending;
    } };
    function loadTab() {
        const context = vm.createContext({ module: { exports: {} }, navigator: { locks: lockManager }, AbortController, setTimeout, clearTimeout, crypto: require('node:crypto').webcrypto });
        vm.runInContext(fs.readFileSync(require.resolve('../static/offline-crud.js'), 'utf8'), context);
        return context.module.exports;
    }
    const storage = storageFixture();
    const FirstTab = loadTab();
    const SecondTab = loadTab();
    const first = new FirstTab({ storage, request: async () => ok(base()) });
    const second = new SecondTab({ storage, request: async () => ok(base()) });
    await Promise.all([first.init(), second.init()]);
    assert.equal(first.clientId, second.clientId);
    await first.seed(base());
    const operations = await Promise.all([
        first.mutate({ entity: 'item', action: 'create', values: { name: 'Bread', section_id: 2 } }),
        second.mutate({ entity: 'item', action: 'create', values: { name: 'Apple', section_id: 2 } })
    ]);
    assert.equal(new Set(operations.map(operation => operation.entity_id)).size, 2);
    assert.deepEqual(storage.read().state.items.map(item => item.name), ['Milk', 'Bread', 'Apple']);
    assert.equal(storage.read().operations.length, 2);
});

test('a tab waiting for a lock that another tab never releases takes it over instead of hanging', async () => {
    const vm = require('node:vm');
    const fs = require('node:fs');
    const held = new Set();
    const lockManager = { request(name, options, callback) {
        if (options.steal) { held.delete(name); return Promise.resolve().then(callback); }
        if (!held.has(name)) { held.add(name); return Promise.resolve().then(callback).finally(() => held.delete(name)); }
        return new Promise((_, reject) => options.signal.addEventListener('abort', () => reject(Object.assign(new Error('aborted'), { name: 'AbortError' }))));
    } };
    const context = vm.createContext({ module: { exports: {} }, navigator: { locks: lockManager }, AbortController, setTimeout, clearTimeout, crypto: require('node:crypto').webcrypto });
    vm.runInContext(fs.readFileSync(require.resolve('../static/offline-crud.js'), 'utf8'), context);
    const Tab = context.module.exports;
    held.add('koffan-offline-crud-store');
    held.add('koffan-offline-crud-sync');
    const client = new Tab({ storage: storageFixture(), request: async () => ok(base()), lockPatience: { store: 10, sync: 10 } });
    await client.init();
    await client.seed(base());
    await client.mutate({ entity: 'item', action: 'update', entity_id: 3, values: { completed: true } });
    assert.equal(client.pendingCount, 1);
    await client.refresh();
});

test('a writer that was overtaken by another tab retries on fresh data instead of erasing it', async () => {
    const inner = storageFixture();
    const compareAndSet = async (key, expectedRevision, value) => {
        const current = inner.read();
        if (current?.revision !== expectedRevision) return false;
        await inner.setMetadata(key, value);
        return true;
    };
    let hold = null;
    const slow = {
        getMetadata: async key => {
            const value = await inner.getMetadata(key);
            if (hold) { const gate = hold; hold = null; await gate.promise; }
            return value;
        },
        setMetadata: inner.setMetadata,
        compareAndSetMetadata: compareAndSet
    };
    const fast = { ...slow, getMetadata: inner.getMetadata };
    const stale = new OfflineCRUD({ storage: slow, request: async () => ok(base()), lockPatience: { store: 20 } });
    const other = new OfflineCRUD({ storage: fast, request: async () => ok(base()), lockPatience: { store: 20 } });
    await stale.init();
    await stale.seed(base());
    await other.init();
    const gate = deferred();
    hold = gate;
    // The first tab has read the record and stalls (frozen tab, lock taken over).
    const late = stale.mutate({ entity: 'item', action: 'update', entity_id: 3, values: { name: 'From the frozen tab' } });
    await new Promise(resolve => setTimeout(resolve, 5));
    await other.mutate({ entity: 'item', action: 'update', entity_id: 3, values: { quantity: 4 } });
    gate.resolve();
    await late;
    const saved = inner.read();
    assert.equal(saved.operations.length, 2, 'both tabs\' operations survive');
    const item = saved.state.items.find(entry => entry.id === 3);
    assert.equal(item.name, 'From the frozen tab');
    assert.equal(item.quantity, 4);
});

test('a synchronization that lost its lock cannot apply its obsolete response', async () => {
    const vm = require('node:vm');
    const fs = require('node:fs');
    const held = new Set();
    const lockManager = { request(name, options, callback) {
        if (options.steal) { held.delete(name); return Promise.resolve().then(callback); }
        if (!held.has(name)) { held.add(name); return Promise.resolve().then(callback).finally(() => held.delete(name)); }
        return new Promise((_, reject) => options.signal.addEventListener('abort', () => reject(Object.assign(new Error('aborted'), { name: 'AbortError' }))));
    } };
    function loadTab() {
        const context = vm.createContext({ module: { exports: {} }, navigator: { locks: lockManager }, AbortController, setTimeout, clearTimeout, crypto: require('node:crypto').webcrypto });
        vm.runInContext(fs.readFileSync(require.resolve('../static/offline-crud.js'), 'utf8'), context);
        return context.module.exports;
    }
    const storage = storageFixture();
    const release = deferred();
    const older = base();
    older.items[0].name = 'Older snapshot';
    const newer = base();
    newer.items[0].name = 'Newer snapshot';
    const FirstTab = loadTab();
    const SecondTab = loadTab();
    const first = new FirstTab({ storage, request: async () => { await release.promise; return ok(older); }, lockPatience: { store: 10, sync: 10 } });
    const second = new SecondTab({ storage, request: async () => ok(newer), lockPatience: { store: 10, sync: 10 } });
    await first.init();
    await first.seed(base());
    await second.init();
    const stalled = first.refresh();
    await new Promise(resolve => setTimeout(resolve, 5));
    await second.refresh();
    assert.equal(storage.read().state.items[0].name, 'Newer snapshot');
    release.resolve();
    await stalled;
    assert.equal(storage.read().state.items[0].name, 'Newer snapshot', 'the obsolete response was dropped');
});

test('large queues drain in FIFO batches of at most one hundred operations', async () => {
    const sizes = [];
    const names = [];
    const { client } = await model({ request: async (_url, options) => {
        const body = JSON.parse(options.body);
        sizes.push(body.operations.length);
        names.push(...body.operations.map(operation => operation.values.name));
        const snapshot = base();
        snapshot.items[0].name = body.operations.at(-1).values.name;
        return ack(body, snapshot);
    } });
    for (let index = 0; index < 101; index++) {
        await client.mutate({ entity: 'item', action: 'update', entity_id: 3, values: { name: `Item ${index}` } });
    }
    await client.sync();
    assert.deepEqual(sizes, [100, 1]);
    assert.deepEqual(names, Array.from({ length: 101 }, (_, index) => `Item ${index}`));
    assert.equal(client.getState().items[0].name, 'Item 100');
    assert.equal(client.pendingCount, 0);
});

test('discarding a conflicting update removes its restored row but preserves another pending edit', async () => {
    const remote = base();
    remote.items = [];
    const { client } = await model({ request: async () => ok(remote) });
    const failed = await client.mutate({ entity: 'item', action: 'update', entity_id: 3, values: { name: 'Oat milk' } });
    const keep = await client.mutate({ entity: 'section', action: 'update', entity_id: 2, values: { name: 'Produce' } });
    await client.refresh();
    assert.equal(client.getState().items.length, 1);
    await client.discardOperation(failed.id);
    assert.deepEqual(client.getState().items, []);
    assert.deepEqual(client.getPendingOperations(), [keep]);
    assert.equal(client.getState().sections[0].name, 'Produce');
});

test('mutateMany persists and notifies once with the complete batch, which survives reload and sync', async () => {
    const storage = storageFixture();
    let writes = 0;
    const originalWrite = storage.setMetadata;
    storage.setMetadata = async (...args) => { writes++; return originalWrite(...args); };
    const changes = [];
    let transmitted;
    const request = async (_url, options) => {
        transmitted = JSON.parse(options.body);
        const snapshot = base();
        snapshot.items[0].completed = true;
        snapshot.items[0].quantity = 6;
        snapshot.sections[0].sort_mode = 'alphabetical';
        return ack(transmitted, snapshot);
    };
    const { client } = await model({ storage, request, onChange: state => changes.push(state) });
    writes = 0;
    changes.length = 0;
    const operations = await client.mutateMany([
        { entity: 'item', action: 'update', entity_id: 3, values: { completed: true } },
        { entity: 'item', action: 'update', entity_id: 3, values: { quantity: 6 } },
        { entity: 'section', action: 'update', entity_id: 2, values: { sort_mode: 'alphabetical' } }
    ]);
    assert.equal(writes, 1);
    assert.equal(changes.length, 1);
    assert.equal(changes[0].items[0].completed, true);
    assert.equal(changes[0].items[0].quantity, 6);
    assert.equal(changes[0].sections[0].sort_mode, 'alphabetical');
    assert.equal(operations.length, 3);
    const reloaded = new OfflineCRUD({ storage, request });
    await reloaded.init();
    assert.deepEqual(reloaded.getPendingOperations(), operations);
    await reloaded.sync();
    assert.deepEqual(transmitted.operations, operations);
    assert.equal(reloaded.pendingCount, 0);
});

test('an invalid later batch operation rolls back every earlier operation and temporary ID allocation', async () => {
    const { client, storage } = await model();
    const before = storage.read();
    await assert.rejects(client.mutateMany([
        { entity: 'item', action: 'create', values: { name: 'Bread', section_id: 2 } },
        { entity: 'item', action: 'update', entity_id: 3, values: { quantity: -1 } }
    ]), /nonnegative/);
    assert.deepEqual(storage.read(), before);
    assert.deepEqual(client.getState(), before.state);
    assert.equal(client.pendingCount, 0);
    const created = await client.mutate({ entity: 'item', action: 'create', values: { name: 'Apple', section_id: 2 } });
    assert.equal(created.entity_id, -1);
});

test('batch legacy imports deduplicate within the batch and after reload without another write', async () => {
    const { client, storage } = await model();
    const input = { legacy_id: 91, entity: 'item', action: 'create', values: { name: 'Bread', section_id: 2 } };
    const operations = await client.mutateMany([input, input]);
    assert.deepEqual(operations[1], operations[0]);
    assert.equal(client.pendingCount, 1);
    const reloaded = new OfflineCRUD({ storage, request: async () => ok(base()) });
    await reloaded.init();
    const before = storage.read();
    assert.deepEqual(await reloaded.mutateMany([input, input]), operations);
    assert.deepEqual(storage.read(), before);
    assert.deepEqual(await reloaded.mutateMany([]), []);
    assert.deepEqual(storage.read(), before);
});

test('a batch persistence failure does not publish intermediate changes', async () => {
    const { client, storage } = await model();
    const before = client.getState();
    storage.fail();
    await assert.rejects(client.mutateMany([
        { entity: 'item', action: 'update', entity_id: 3, values: { completed: true } },
        { entity: 'item', action: 'update', entity_id: 3, values: { quantity: 9 } }
    ]), /quota/);
    assert.deepEqual(client.getState(), before);
    assert.equal(client.pendingCount, 0);
});

test('a newer page snapshot replaces an older stored copy and keeps pending changes', async () => {
    const { client } = await model({ snapshot: { ...base(), generated_at: 1000 } });
    await client.mutate({ entity: 'item', action: 'update', entity_id: 3, values: { completed: true } });
    const newer = base();
    newer.items.push({ id: 4, section_id: 2, name: 'Bread', description: '', completed: false, quantity: 0, sort_order: 1 });
    newer.generated_at = 2000;
    assert.equal(await client.seed(newer), true);
    const state = client.getState();
    assert.deepEqual(state.items.map(item => [item.id, item.completed]), [[3, true], [4, false]], 'pending edits are replayed on top');
    assert.equal(client.pendingCount, 1);
    assert.equal('generated_at' in state, false, 'the generation time is not part of the visible model');
    assert.equal(await client.seed({ ...blank(), generated_at: 1500 }), false, 'an older copy, such as a cached offline page, is ignored');
    assert.equal(await client.seed(blank()), false, 'a snapshot without a generation time never replaces stored data');
    assert.equal(client.getState().items.length, 2);
});
