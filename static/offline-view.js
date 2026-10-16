(function (root, factory) {
    const view = factory();
    if (typeof module === 'object' && module.exports) module.exports = view;
    if (root) root.OfflineView = view;
})(typeof window !== 'undefined' ? window : null, function () {
    'use strict';

    // Model values only enter escaped text/attributes. Alpine expressions read
    // user strings from data attributes instead of treating them as JavaScript.
    const escape = value => String(value ?? '').replace(/[&<>"']/g, character => ({
        '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;'
    })[character]);
    const id = value => {
        const result = Number(value);
        if (!Number.isSafeInteger(result)) throw new TypeError('Invalid entity ID');
        return result;
    };
    const numeric = value => Number.isFinite(Number(value)) ? Number(value) : 0;
    const quantity = value => Math.max(0, Math.trunc(numeric(value)));
    const completed = value => value === true || value === 1 || value === 'true';
    const sortMode = mode => ['alphabetical', 'alphabetical_desc'].includes(mode) ? mode : 'manual';
    const paths = {
        check: 'M5 13l4 4L19 7', close: 'M6 18L18 6M6 6l12 12',
        plus: 'M12 4v16m8-8H4', up: 'M5 15l7-7 7 7', down: 'M19 9l-7 7-7-7',
        right: 'M9 5l7 7-7 7', menu: 'M4 6h16M4 10h16M4 14h16M4 18h16',
        edit: 'M11 5H6a2 2 0 00-2 2v11a2 2 0 002 2h11a2 2 0 002-2v-5m-1.414-9.414a2 2 0 112.828 2.828L11.828 15H9v-2.828l8.586-8.586z',
        move: 'M8 7h12m0 0l-4-4m4 4l-4 4m0 6H4m0 0l4 4m-4-4l4-4',
        'move-list': 'M3 10h10a8 8 0 018 8v2M3 10l6 6m-6-6l6-6',
        uncertain: 'M8.228 9c.549-1.165 2.03-2 3.772-2 2.21 0 4 1.343 4 3 0 1.4-1.278 2.575-3.006 2.907-.542.104-.994.54-.994 1.093m0 3h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z',
        dots: 'M12 5v.01M12 12v.01M12 19v.01M12 6a1 1 0 110-2 1 1 0 010 2zm0 7a1 1 0 110-2 1 1 0 010 2zm0 7a1 1 0 110-2 1 1 0 010 2z'
    };
    const svg = (name, classes = 'w-4 h-4') => `<svg class="${classes}" fill="none" stroke="currentColor" viewBox="0 0 24 24" aria-hidden="true"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="${paths[name]}"></path></svg>`;
    const actionClass = 'p-1.5 rounded-md hover:bg-stone-100 dark:hover:bg-stone-700 text-stone-400 dark:text-stone-500 transition-colors';
    const fieldClass = 'w-full border border-stone-200 dark:border-stone-600 rounded-lg px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-pink-400 bg-white dark:bg-stone-700 text-stone-800 dark:text-stone-100';
    const button = (expression, title, icon, extra = '', bindLabel = true) => `<button type="button" @click="${escape(expression)}" ${bindLabel ? `:title="t('${title}')" :aria-label="t('${title}')"` : ''} class="${actionClass}" ${extra}>${svg(icon)}</button>`;
    const menuButton = (expression, title, icon, extra = '') => `<button type="button" @click="${escape(expression)}" class="w-full px-3 py-2.5 text-left text-sm text-stone-700 dark:text-stone-200 hover:bg-stone-50 dark:hover:bg-stone-700 flex items-center gap-3 disabled:opacity-30" ${extra}>${svg(icon)}<span x-text="t('${title}')"></span></button>`;
    const listURL = value => id(value) < 0 ? `/offline/list?list_id=${id(value)}` : `/lists/${id(value)}`;

    // New server IDs will follow existing IDs in creation order. Temporary IDs
    // count downward, so use their creation order for ties before synchronization.
    function compareIDs(left, right) {
        const a = id(left), b = id(right);
        if ((a < 0) !== (b < 0)) return a < 0 ? 1 : -1;
        return a < 0 ? b - a : a - b;
    }

    // SQLite NOCASE folds only ASCII letters. localeCompare would reorder Polish
    // names differently and make rows jump again when server snapshots arrive.
    function compareNames(left, right) {
        const fold = text => String(text ?? '').replace(/[A-Z]/g, letter => letter.toLowerCase());
        const a = Array.from(fold(left));
        const b = Array.from(fold(right));
        for (let index = 0; index < Math.min(a.length, b.length); index++) {
            const difference = a[index].codePointAt(0) - b[index].codePointAt(0);
            if (difference) return difference;
        }
        return a.length - b.length;
    }

    function sortItems(items = [], mode = 'manual') {
        const order = sortMode(mode);
        return [...items].sort((left, right) => {
            const state = Number(completed(left.completed)) - Number(completed(right.completed));
            if (state) return state;
            const comparison = order === 'manual'
                ? numeric(left.sort_order) - numeric(right.sort_order)
                : compareNames(left.name, right.name) * (order === 'alphabetical_desc' ? -1 : 1);
            return comparison || compareIDs(left.id, right.id);
        });
    }

    function sortEntities(entities = []) {
        return [...entities].sort((a, b) => numeric(a.sort_order) - numeric(b.sort_order) || compareIDs(a.id, b.id));
    }

    // Sections of every other list, offered by "Move to another list".
    // destinations: [{ id, name, icon, sections: [{ id, name }] }]
    const destinationHeader = destination => `<div class="px-3 pt-1.5 pb-0.5 text-xs font-medium text-stone-400 dark:text-stone-500 truncate">${escape(destination.icon || '🛒')} ${escape(destination.name)}</div>`;

    function listMoveMenu(itemId, destinations = []) {
        return destinations.map(destination => destinationHeader(destination) + destination.sections.map(section => `<button type="button" @click="open=false; $data.moveItemToList(${id(itemId)},${id(section.id)})" class="block w-full text-left pl-6 pr-3 py-1.5 text-sm text-stone-600 dark:text-stone-300 hover:bg-stone-50 dark:hover:bg-stone-700 truncate">${escape(section.name)}</button>`).join('')).join('');
    }

    function mobileListMoveMenu(destinations = []) {
        return destinations.map(destination => `<div class="pt-1 text-xs font-medium text-stone-400 dark:text-stone-500 truncate">${escape(destination.icon || '🛒')} ${escape(destination.name)}</div>` + destination.sections.map(section => `<button type="button" @click="moveToListSection(${id(section.id)})" class="w-full text-left p-2 rounded-lg text-sm text-stone-600 dark:text-stone-300 hover:bg-stone-100 dark:hover:bg-stone-700 truncate">${escape(section.name)}</button>`).join('')).join('');
    }

    function item(value, sections = [], destinations = []) {
        const itemId = id(value.id), sectionId = id(value.section_id);
        const done = completed(value.completed), uncertain = completed(value.uncertain), count = quantity(value.quantity);
        const attributes = `data-item-id="${itemId}" data-item-name="${escape(value.name)}" data-item-description="${escape(value.description)}" data-item-quantity="${count}" data-section-id="${sectionId}" data-uncertain="${uncertain}"`;
        const edit = `$data.editItem({id:Number($el.dataset.itemId),name:$el.dataset.itemName,description:$el.dataset.itemDescription,quantity:Number($el.dataset.itemQuantity)})`;
        const remove = `if(confirm(t('confirm.delete_item',{name:$el.dataset.itemName}))) $data.deleteItemDirect(${itemId},${sectionId})`;
        const moves = sortEntities(sections).filter(section => id(section.id) !== sectionId).map(section => `<button type="button" @click="open=false; $data.moveItemDesktop(${itemId},${sectionId},${id(section.id)})" class="block w-full text-left px-3 py-1.5 text-sm text-stone-600 dark:text-stone-300 hover:bg-stone-50 dark:hover:bg-stone-700">${escape(section.name)}</button>`).join('');
        return `<div id="item-${itemId}" ${attributes} data-completed="${done}" data-sort-order="${numeric(value.sort_order)}" data-sort-name="${escape(value.name)}" class="shopping-item px-4 py-3 flex items-center gap-0.5 hover:bg-stone-50 dark:hover:bg-stone-700 active:bg-stone-100 dark:active:bg-stone-700 transition-colors group select-none${uncertain ? ' bg-amber-50/50 dark:bg-amber-900/30' : ''}" x-show="isItemVisible(${itemId})">
    <div class="drag-handle flex-shrink-0 w-5 h-10 flex items-center justify-center -ml-2 touch-none cursor-grab active:cursor-grabbing text-stone-300 dark:text-stone-600">${svg('menu')}</div>
    <button type="button" role="checkbox" aria-label="${escape(value.name)}" aria-checked="${done}" @click="$data.toggleItem(${itemId},${sectionId})" class="flex-shrink-0 w-11 h-11 flex items-center justify-center -m-3 active:scale-90 transition-transform"><span class="w-5 h-5 rounded-full ${done ? 'bg-pink-400 flex items-center justify-center' : 'border-2 border-stone-300 dark:border-stone-500 hover:border-pink-400 transition-transform hover:scale-110'}">${done ? svg('check', 'w-3 h-3 text-white') : ''}</span></button>
    <div class="item-content flex-1 min-w-0 cursor-pointer ml-2" @click="$data.toggleItem(${itemId},${sectionId})"><div class="flex items-center gap-2">${uncertain ? '<span class="item-uncertain text-amber-500 dark:text-amber-400 text-xs">?</span>' : ''}<p class="item-name text-sm text-stone-700 dark:text-stone-200 line-clamp-2 break-words">${escape(value.name)}</p>${count ? `<span class="item-quantity px-1.5 py-0.5 text-xs font-medium bg-stone-100 dark:bg-stone-700 text-stone-600 dark:text-stone-300 rounded-full flex-shrink-0">${count}x</span>` : ''}</div>${value.description ? `<p class="item-description text-xs text-stone-400 dark:text-stone-500 line-clamp-2 break-words mt-0.5">${escape(value.description)}</p>` : ''}</div>
    <div class="item-active-actions hidden md:flex items-center gap-0.5 opacity-0 group-hover:opacity-100 transition-opacity">
        ${button(`$data.toggleUncertainFetch(${itemId})`, uncertain ? 'actions.remove_mark' : 'actions.mark_uncertain', 'uncertain')}
        ${button(edit, 'common.edit', 'edit', attributes)}
        <div x-data="{open:false}" class="item-move relative">${button('open=!open', 'actions.move', 'move')}<div x-show="open" @click.outside="open=false" x-cloak class="absolute right-0 mt-1 w-40 bg-white dark:bg-stone-800 rounded-lg shadow-xl border border-stone-200 dark:border-stone-700 z-50 py-1">${moves}</div></div>
        <div x-data="{open:false}" data-item-id="${itemId}" class="item-move-list relative${destinations.length ? '' : ' hidden'}">${button('open=!open', 'actions.move_to_list', 'move-list')}<div x-show="open" @click.outside="open=false" x-cloak data-list-moves class="absolute right-0 mt-1 w-56 max-h-64 overflow-y-auto bg-white dark:bg-stone-800 rounded-lg shadow-xl border border-stone-200 dark:border-stone-700 z-50 py-1">${listMoveMenu(itemId, destinations)}</div></div>
        ${button(remove, 'common.delete', 'close', `data-item-name="${escape(value.name)}"`)}
    </div>
    <button type="button" ${attributes} @click="$dispatch('open-mobile-action',{id:Number($el.dataset.itemId),name:$el.dataset.itemName,description:$el.dataset.itemDescription,quantity:Number($el.dataset.itemQuantity),section_id:Number($el.dataset.sectionId),uncertain:$el.dataset.uncertain==='true'})" :aria-label="t('items.more_options')" class="item-active-actions md:hidden p-2 rounded-lg text-stone-400 dark:text-stone-500 hover:bg-stone-100 dark:hover:bg-stone-700">${svg('dots', 'w-5 h-5')}</button>
    <button type="button" data-item-name="${escape(value.name)}" @click="${escape(remove)}" :aria-label="t('common.delete')" class="item-completed-delete hidden p-1.5 rounded-md text-stone-300 dark:text-stone-500 hover:text-red-500 dark:hover:text-red-400 hover:bg-red-50 dark:hover:bg-red-900/30 opacity-0 group-hover:opacity-100 transition-opacity">${svg('close')}</button>
</div>`;
    }

    function section(value, items = [], sections = [], showCompleted = true) {
        const sectionId = id(value.id), mode = sortMode(value.sort_mode);
        const ordered = sortItems(items.filter(entry => id(entry.section_id) === sectionId), mode);
        const active = ordered.filter(entry => !completed(entry.completed));
        const done = ordered.filter(entry => completed(entry.completed));
        return `<div id="section-${sectionId}" data-section-id="${sectionId}" data-sort-mode="${mode}" class="bg-white dark:bg-stone-800 rounded-xl border border-stone-200 dark:border-stone-700 mb-4${ordered.length ? '' : ' hidden'}" x-show="isSectionVisible(${sectionId})">
    <div data-name="${escape(value.name)}" x-data="{editing:false,editName:$el.dataset.name}" class="px-4 py-3 border-b border-stone-100 dark:border-stone-700 flex items-center justify-between">
        <div class="flex items-center gap-3 flex-1 min-w-0"><h3 x-show="!editing" @dblclick="editing=true; $nextTick(()=>$refs.sectionName.focus())" class="font-medium text-stone-800 dark:text-stone-100 truncate">${escape(value.name)}</h3>
        <form x-show="editing" x-cloak @submit.prevent="crudUpdateSection(${sectionId},editName); editing=false" class="flex-1 flex items-center gap-1"><input x-ref="sectionName" x-model="editName" name="name" required maxlength="100" @keydown.escape="editing=false" class="${fieldClass}"><button type="submit" class="${actionClass}" :aria-label="t('common.save')">${svg('check')}</button></form>
        <span x-show="!editing" class="text-xs text-stone-400 dark:text-stone-500 section-counter flex-shrink-0">${done.length}/${ordered.length}</span></div>
        <div class="flex items-center gap-2 flex-shrink-0">${button(`toggleAllItems(${sectionId})`, active.length ? 'items.check_all' : 'items.uncheck_all', 'check', 'data-crud-toggle-all', false)}${button(`cycleSortMode(${sectionId},'${mode}')`, 'items.sort_mode', mode === 'manual' ? 'menu' : mode === 'alphabetical' ? 'down' : 'up')}${button('editing=true; $nextTick(()=>$refs.sectionName.focus())', 'common.edit', 'edit')}${button(`quickAddSectionId===${sectionId}?closeQuickAdd():openQuickAdd(${sectionId})`, 'items.quick_add', 'plus')}</div>
    </div>
    <div x-show="quickAddSectionId===${sectionId}" x-cloak class="px-4 py-3 border-b border-stone-100 dark:border-stone-700 bg-stone-50/50 dark:bg-stone-900/30"><form @submit.prevent="submitQuickAdd(${sectionId})" class="relative flex items-center gap-2"><div class="flex-1 relative"><input type="text" id="quick-add-input-${sectionId}" x-model="quickAddName" maxlength="200" autocomplete="off" :placeholder="t('items.what_to_buy')" @input.debounce.150ms="fetchQuickAddSuggestions($event.target.value)" @keydown="handleQuickAddKeydown($event,${sectionId})" @blur="hideQuickAddSuggestionsDelayed()" class="${fieldClass}"><div x-show="showQuickAddSuggestions && quickAddSuggestions.length" x-cloak class="absolute top-full left-0 right-0 mt-1 bg-white dark:bg-stone-800 border border-stone-200 dark:border-stone-700 rounded-lg shadow-lg z-50 max-h-48 overflow-y-auto"><template x-for="(suggestion,index) in quickAddSuggestions" :key="suggestion.name"><button type="button" @mousedown.prevent="selectQuickAddSuggestion(suggestion)" class="w-full text-left px-3 py-2 text-sm text-stone-700 dark:text-stone-200 hover:bg-stone-50 dark:hover:bg-stone-700" :class="index===selectedQuickAddSuggestionIndex && 'bg-pink-50'"><span x-text="suggestion.name"></span></button></template></div></div><button type="submit" class="w-8 h-8 flex items-center justify-center bg-pink-400 hover:bg-pink-500 text-white rounded-lg" :aria-label="t('common.add')">${svg('check')}</button>${button(`expandToFullModal(${sectionId})`, 'items.more_options', 'dots')}</form></div>
    <div class="divide-y divide-stone-100 dark:divide-stone-700 active-items items-sortable" data-section-id="${sectionId}">${active.map(entry => item(entry, sections)).join('')}</div>
    <div class="border-t border-stone-100 dark:border-stone-700 bg-stone-50/50 dark:bg-stone-900/50 rounded-b-xl completed-wrapper${showCompleted ? '' : ' hidden'}" ${done.length ? '' : 'style="display:none"'} x-data="{open:window.getCompletedSectionState(${sectionId})}"><button type="button" @click="open=!open; try { const s=JSON.parse(localStorage.getItem('completedSections')||'{}'); s['section-${sectionId}']=open; localStorage.setItem('completedSections',JSON.stringify(s)); } catch(e) {}" class="w-full px-4 py-2 flex items-center gap-2 cursor-pointer select-none hover:bg-stone-100/50 dark:hover:bg-stone-700/50">${svg('right', 'w-4 h-4 text-stone-400')}<span class="text-xs text-stone-500 dark:text-stone-400"><span x-text="t('list.bought')"></span> (<span class="completed-count">${done.length}</span>)</span></button><div x-show="open" x-cloak x-collapse class="divide-y divide-stone-100 dark:divide-stone-700 completed-items">${done.map(entry => item(entry, sections)).join('')}</div></div>
</div>`;
    }

    function manageSections(sections = []) {
        const ordered = sortEntities(sections);
        return ordered.map((value, index) => {
            const sectionId = id(value.id);
            return `<div id="manage-section-${sectionId}" data-name="${escape(value.name)}" x-data="{editing:false,editName:$el.dataset.name}" class="p-3 bg-stone-50 dark:bg-stone-700 rounded-lg border border-stone-100 dark:border-stone-600 flex items-center gap-3" @click="if(selectMode) toggleSection(${sectionId})" :class="{'ring-2 ring-pink-400':selectMode && selectedSections.includes(${sectionId})}">
    <input type="checkbox" x-show="selectMode" :checked="selectedSections.includes(${sectionId})" @click.stop="toggleSection(${sectionId})" :aria-label="t('sections.select')" class="w-4 h-4 rounded border-stone-300 text-pink-400">
    <span x-show="!editing" class="flex-1 font-medium text-stone-700 dark:text-stone-200 text-sm">${escape(value.name)}</span>
    <form x-show="editing" x-cloak @click.stop @submit.prevent="crudUpdateSection(${sectionId},editName); editing=false" class="flex-1 flex gap-1"><input name="name" x-ref="sectionName" x-model="editName" required maxlength="100" @keydown.escape="editing=false" class="${fieldClass}"><button type="submit" class="${actionClass}" :aria-label="t('common.save')">${svg('check')}</button></form>
    <div x-show="!selectMode" @click.stop class="flex items-center gap-1">${button('editing=true; $nextTick(()=>$refs.sectionName.focus())', 'common.edit', 'edit')}${button(`crudMoveSection(${sectionId},'up')`, 'actions.move_up', 'up', index === 0 ? 'disabled' : '')}${button(`crudMoveSection(${sectionId},'down')`, 'actions.move_down', 'down', index === ordered.length - 1 ? 'disabled' : '')}${button(`crudDeleteSection(${sectionId})`, 'common.delete', 'close')}</div>
</div>`;
        }).join('');
    }

    function lists(values = [], items = [], sections = []) {
        const owners = new Map(sections.map(value => [id(value.id), id(value.list_id)]));
        const ordered = sortEntities(values);
        return ordered.map((value, index) => {
            const listId = id(value.id);
            const members = items.filter(entry => Number(entry.list_id ?? owners.get(Number(entry.section_id))) === listId);
            const total = members.length, done = members.filter(entry => completed(entry.completed)).length;
            const percentage = total ? Math.round(done / total * 100) : 0;
            const navigation = `@click.prevent="crudNavigateList(${listId})" href="${listURL(listId)}"`;
            return `<div id="home-list-${listId}" data-list-id="${listId}" data-name="${escape(value.name)}" data-icon="${escape(value.icon || '🛒')}" x-data="{showActions:false,editing:false,editName:$el.dataset.name,editIcon:$el.dataset.icon}" class="relative bg-white dark:bg-stone-800 rounded-xl border border-stone-200 dark:border-stone-700 p-4 flex items-center gap-4 hover:border-pink-200 dark:hover:border-pink-700 hover:shadow-md transition-all group w-full">
    <a ${navigation} class="w-12 h-12 rounded-xl bg-pink-50 dark:bg-pink-900/30 flex items-center justify-center flex-shrink-0 text-2xl" aria-label="${escape(value.name)}"><span style="filter:grayscale(100%) sepia(50%) hue-rotate(-30deg) saturate(300%)">${escape(value.icon || '🛒')}</span></a>
    <a ${navigation} x-show="!editing" class="flex-1 min-w-0"><p class="font-medium text-stone-800 dark:text-stone-100 truncate">${escape(value.name)}</p><p class="text-sm text-stone-400 dark:text-stone-500" data-stats-count>${done}/${total} <span x-text="t('list.completed')"></span></p></a>
    <form x-show="editing" x-cloak @submit.prevent="crudUpdateList(${listId},editName,editIcon); editing=false" class="flex-1 min-w-0 flex flex-wrap gap-2"><input name="name" x-model="editName" x-ref="listName" required maxlength="100" :aria-label="t('common.name')" class="${fieldClass}"><input name="icon" x-model="editIcon" maxlength="16" :aria-label="t('lists.icon')" class="${fieldClass}"><button type="submit" class="${actionClass}" :aria-label="t('common.save')">${svg('check')}</button>${button('editing=false', 'common.cancel', 'close')}</form>
    <div x-show="!editing" class="flex items-center gap-3">${total ? `<div class="w-16 h-2 bg-stone-100 dark:bg-stone-700 rounded-full overflow-hidden hidden sm:block"><div class="h-full bg-pink-400 rounded-full" style="width:${percentage}%"></div></div><span class="text-sm text-stone-400 dark:text-stone-500 w-10 text-right hidden sm:block">${percentage}%</span>` : ''}<div class="relative">${button('showActions=!showActions', 'items.more_options', 'dots')}<div x-show="showActions" x-cloak @click.outside="showActions=false" class="absolute right-0 top-full mt-1 bg-white dark:bg-stone-800 rounded-xl border border-stone-200 dark:border-stone-700 shadow-lg p-2 z-10 min-w-40 flex flex-col">${menuButton(`showActions=false; crudMoveList(${listId},'up')`, 'actions.move_up', 'up', index === 0 ? 'disabled' : '')}${menuButton(`showActions=false; crudMoveList(${listId},'down')`, 'actions.move_down', 'down', index === ordered.length - 1 ? 'disabled' : '')}${menuButton('showActions=false; editing=true; $nextTick(()=>$refs.listName.focus())', 'common.edit', 'edit')}${menuButton(`crudDeleteList(${listId})`, 'common.delete', 'close')}</div></div><a ${navigation} class="p-1" aria-label="${escape(value.name)}">${svg('right', 'w-5 h-5 text-stone-300 dark:text-stone-600')}</a></div>
</div>`;
        }).join('');
    }

    function listOptions(values = []) {
        return sortEntities(values).map(value => `<a href="${listURL(value.id)}" @click.prevent="showListSwitcher=false; crudNavigateList(${id(value.id)})" class="flex items-center gap-3 px-4 py-3 text-sm text-stone-700 dark:text-stone-200 hover:bg-stone-50 dark:hover:bg-stone-700"><span class="text-lg">${escape(value.icon || '🛒')}</span><span class="truncate">${escape(value.name)}</span></a>`).join('');
    }

    return { item, section, manageSections, lists, listOptions, listMoveMenu, mobileListMoveMenu, sortItems, sortEntities, escape };
});
