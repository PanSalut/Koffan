const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const vm = require('node:vm');
const view = require('../static/offline-view.js');

const sections = [
    { id: -2, list_id: -1, name: 'Produce', sort_order: 0, sort_mode: 'manual' },
    { id: 3, list_id: -1, name: 'Other', sort_order: 1, sort_mode: 'alphabetical' }
];
const base = { id: -7, section_id: -2, name: 'Milk', description: 'Whole milk', quantity: 2, completed: false, uncertain: false, sort_order: 3 };
const decode = value => value.replace(/&quot;/g, '"').replace(/&#39;/g, "'").replace(/&lt;/g, '<').replace(/&gt;/g, '>').replace(/&amp;/g, '&');

test('renderer is available through browser global and CommonJS', () => {
    const context = vm.createContext({ window: {} });
    vm.runInContext(fs.readFileSync(require.resolve('../static/offline-view.js'), 'utf8'), context);
    assert.equal(typeof context.window.OfflineView.item, 'function');
    for (const method of ['item', 'section', 'manageSections', 'lists', 'listOptions']) {
        assert.equal(typeof view[method], 'function');
    }
});

test('new local items retain the existing row contracts and all item controls', () => {
    const html = view.item(base, sections);
    for (const attribute of ['id="item--7"', 'data-item-id="-7"', 'data-section-id="-2"', 'data-completed="false"', 'data-sort-order="3"', 'data-sort-name="Milk"', 'aria-checked="false"']) {
        assert.ok(html.includes(attribute), attribute);
    }
    for (const className of ['shopping-item', 'item-content', 'item-name', 'item-quantity', 'item-description', 'item-active-actions', 'item-completed-delete', 'drag-handle']) {
        assert.ok(html.includes(className), className);
    }
    for (const method of ['toggleItem(-7,-2)', 'toggleUncertainFetch(-7)', 'moveItemDesktop(-7,-2,3)', 'deleteItemDirect(-7,-2)', 'editItem(', 'open-mobile-action']) {
        assert.ok(html.includes(method), method);
    }
    assert.doesNotMatch(html, /pending-sync|offline-sync-badge|status\.syncing|bg-rose-50/);
});

test('completed and uncertain items share row layout while exposing their semantic states', () => {
    const html = view.item({ ...base, completed: true, uncertain: true }, sections);
    assert.match(html, /data-completed="true"/);
    assert.match(html, /aria-checked="true"/);
    assert.match(html, /data-uncertain="true"/);
    assert.match(html, /bg-pink-400/);
    assert.match(html, /item-uncertain/);
    assert.match(html, /item-completed-delete/);
    assert.match(html, />2x<\/span>/);
});

test('user strings stay escaped outside Alpine source, including quotes and malicious markup', () => {
    const payload = `'"<&><img src=x onerror="globalThis.pwned=true">\n');globalThis.pwned=true;//`;
    const item = { ...base, name: payload, description: payload };
    const section = { ...sections[0], name: payload };
    const list = { id: -1, name: payload, icon: payload };
    const outputs = [
        view.item(item, [section]), view.section(section, [item], [section], true),
        view.manageSections([section]), view.lists([list], [item], [section]), view.listOptions([list])
    ];
    for (const html of outputs) {
        assert.doesNotMatch(html, /<img\b|<script\b/);
        assert.ok(html.includes('&lt;img'), 'dangerous markup remains text');
        for (const match of html.matchAll(/(?:@[^\s=]+|x-data|x-show|x-init|:[^\s=]+)="([^"]*)"/g)) {
            const source = decode(match[1]);
            assert.ok(!source.includes('globalThis.pwned'), 'user data never becomes Alpine code');
            assert.doesNotThrow(() => new Function(match[0].startsWith('@') || match[0].startsWith('x-init') ? source : `return (${source})`));
        }
    }
});

test('entity IDs reject JavaScript injection and unsafe numbers before rendering', () => {
    for (const badId of ['1);alert(1)//', Infinity, NaN, Number.MAX_SAFE_INTEGER + 1]) {
        assert.throws(() => view.item({ ...base, id: badId }), /Invalid entity ID/);
        assert.throws(() => view.listOptions([{ id: badId, name: 'List' }]), /Invalid entity ID/);
    }
});

test('manual and alphabetical item order matches SQLite NOCASE with ascending ID ties', () => {
    const items = [
        { ...base, id: 8, name: 'apple', sort_order: 2 },
        { ...base, id: 4, name: 'Apple', sort_order: 2 },
        { ...base, id: 5, name: 'Zebra', sort_order: 1 },
        { ...base, id: 6, name: 'Ą', sort_order: 0 },
        { ...base, id: 7, name: 'ą', sort_order: 0 },
        { ...base, id: -9, name: 'A', sort_order: -1, completed: true }
    ];
    assert.deepEqual(view.sortItems(items, 'manual').map(item => item.id), [6, 7, 5, 4, 8, -9]);
    assert.deepEqual(view.sortItems(items, 'alphabetical').map(item => item.id), [4, 8, 5, 6, 7, -9]);
    assert.deepEqual(view.sortItems(items, 'alphabetical_desc').map(item => item.id), [7, 6, 5, 4, 8, -9]);
    assert.deepEqual(items.map(item => item.id), [8, 4, 5, 6, 7, -9], 'rendering never sorts the source model in place');
    assert.deepEqual(view.sortItems([{ ...base, id: 1, name: '𐀀' }, { ...base, id: 2, name: '\uE000' }], 'alphabetical').map(item => item.id), [2, 1]);
});

test('tied temporary entities follow existing IDs in their eventual server creation order', () => {
    const items = [-4, 9, -1, 3, -2].map(id => ({ ...base, id }));
    for (const mode of ['manual', 'alphabetical', 'alphabetical_desc']) {
        const beforeSync = view.sortItems(items, mode).map(item => item.id);
        assert.deepEqual(beforeSync, [3, 9, -1, -2, -4]);
        const aliases = new Map([[-1, 10], [-2, 11], [-4, 12]]);
        const afterSync = view.sortItems(items.map(item => ({ ...item, id: aliases.get(item.id) ?? item.id })), mode).map(item => item.id);
        assert.deepEqual(beforeSync.map(id => aliases.get(id) ?? id), afterSync);
    }
    assert.deepEqual(view.sortEntities(items).map(item => item.id), [3, 9, -1, -2, -4]);
});

test('section filters its own items, separates completion states and retains hidden completed container', () => {
    const html = view.section(sections[0], [base, { ...base, id: 8, completed: true }, { ...base, id: 9, section_id: 3 }], sections, false);
    assert.match(html, /id="section--2"/);
    assert.match(html, /data-sort-mode="manual"/);
    assert.match(html, /section-counter[^>]*>1\/2<\/span>/);
    assert.match(html, /completed-count">1<\/span>/);
    assert.match(html, /completed-wrapper hidden/);
    assert.ok(html.indexOf('id="item--7"') < html.indexOf('completed-wrapper'));
    assert.ok(html.indexOf('id="item-8"') > html.indexOf('completed-wrapper'));
    assert.doesNotMatch(html, /id="item-9"/);
    assert.match(html, /crudUpdateSection\(-2,editName\)/);
    assert.match(html, /submitQuickAdd\(-2\)/);
    assert.match(html, /toggleAllItems\(-2\)/);
    const empty = view.section(sections[0], [], sections, true);
    assert.match(empty, /mb-4 hidden/);
    assert.match(empty, /data-crud-toggle-all/);
    assert.match(empty, /toggleAllItems\(-2\)/);
    const button = empty.match(/<button\b[^>]*data-crud-toggle-all[^>]*>/)?.[0];
    assert.ok(button);
    assert.doesNotMatch(button, /:title=|:aria-label=|x-bind:/, 'Alpine must not overwrite labels computed after actual items are inserted');
});

test('section management uses offline CRUD handlers and disables unavailable moves', () => {
    const html = view.manageSections([...sections].reverse());
    assert.ok(html.indexOf('id="manage-section--2"') < html.indexOf('id="manage-section-3"'));
    assert.match(decode(html), /crudMoveSection\(-2,'up'\)[^>]+disabled/);
    assert.match(decode(html), /crudMoveSection\(3,'down'\)[^>]+disabled/);
    for (const method of ['crudUpdateSection(-2,editName)', 'crudDeleteSection(-2)', 'toggleSection(-2)']) assert.ok(html.includes(method));
    assert.doesNotMatch(html, /fetch\(|htmx\.|hx-(get|post|put|delete)|confirm\(/);
});

test('home list cards derive counts through section ownership and offer offline navigation and editing', () => {
    const lists = [ { id: -1, name: 'Groceries', icon: '🛒', sort_order: 2 }, { id: 2, name: 'Hardware', sort_order: 0 } ];
    const html = view.lists(lists, [base, { ...base, id: 8, completed: true }, { ...base, id: 9, section_id: 999 }], sections);
    assert.ok(html.indexOf('id="home-list-2"') < html.indexOf('id="home-list--1"'));
    assert.match(html, /data-stats-count>1\/2/);
    assert.match(html, /style="width:50%"/);
    for (const expression of ['crudNavigateList(-1)', 'crudUpdateList(-1,editName,editIcon)', 'crudDeleteList(-1)', 'crudMoveList(-1,']) assert.ok(html.includes(expression));
    assert.doesNotMatch(html, /pending-sync|status\.syncing|fetch\(|htmx\.|confirm\(/);
    const options = view.listOptions(lists);
    assert.match(options, /href="\/offline\/list\?list_id=-1"/);
    assert.match(options, /crudNavigateList\(-1\)/);
    assert.match(options, />Groceries<\/span>/);
});

test('all list links work when opened in another tab without the Alpine click handler', () => {
    const lists = [{ id: -12, name: 'Local list' }, { id: 7, name: 'Saved list' }];
    const cards = view.lists(lists);
    const switcher = view.listOptions(lists);
    for (const html of [cards, switcher]) {
        const links = [...html.matchAll(/href="([^"]+)"/g)].map(match => match[1]);
        assert.ok(links.includes('/offline/list?list_id=-12'));
        assert.ok(links.includes('/lists/7'));
        assert.ok(links.every(href => href === '/offline/list?list_id=-12' || href === '/lists/7'));
        assert.doesNotMatch(html, /href="\/lists\/-/);
    }
});

const destinations = [
    { id: 9, name: 'Lidl <b>', icon: '🛒', sections: [{ id: 21, name: 'Dairy' }, { id: -4, name: 'Fridge "cold"' }] },
    { id: 10, name: 'Rewe', icon: '', sections: [{ id: 30, name: 'Bakery' }] }
];

test('items offer other lists grouped by list and hide the control when there is none', () => {
    const html = view.item(base, sections, destinations);
    assert.ok(html.includes('class="item-move-list relative"'));
    assert.ok(html.includes('data-list-moves'));
    for (const call of ['moveItemToList(-7,21)', 'moveItemToList(-7,-4)', 'moveItemToList(-7,30)']) assert.ok(decode(html).includes(call), call);
    assert.ok(html.indexOf('Lidl &lt;b&gt;') < html.indexOf('moveItemToList(-7,21)'));
    assert.ok(html.indexOf('moveItemToList(-7,-4)') < html.indexOf('Rewe'));
    assert.ok(html.includes('🛒 Rewe'), 'a list without an icon falls back to the cart');
    assert.doesNotMatch(html, /Lidl <b>|Fridge "cold"/);
    assert.match(view.item(base, sections), /item-move-list relative hidden/);
    assert.equal(view.listMoveMenu(-7, []), '');
});

test('mobile list menu lists every destination section and escapes names', () => {
    const html = view.mobileListMoveMenu(destinations);
    assert.ok(decode(html).includes('moveToListSection(21)') && decode(html).includes('moveToListSection(-4)'));
    assert.ok(html.includes('Lidl &lt;b&gt;'));
    assert.doesNotMatch(html, /Lidl <b>/);
    assert.equal(view.mobileListMoveMenu([]), '');
});

test('all rendered action labels use existing translation keys', () => {
    const translations = JSON.parse(fs.readFileSync(require.resolve('../i18n/en.json'), 'utf8'));
    const html = decode([
        view.section(sections[0], [base, { ...base, id: 8, completed: true, uncertain: true }], sections),
        view.item(base, sections, destinations),
        view.manageSections(sections), view.lists([{ id: -1, name: 'List' }], [base], sections)
    ].join(''));
    const keys = new Set([...html.matchAll(/\bt\('([^']+)'/g)].map(match => match[1]));
    for (const key of keys) {
        const translation = key.split('.').reduce((value, component) => value?.[component], translations);
        assert.equal(typeof translation, 'string', `missing translation for ${key}`);
    }
});
