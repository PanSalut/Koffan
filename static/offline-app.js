// Local-first shopping UI. Every mutation is durable before the view changes.
(function () {
    const clone = value => JSON.parse(JSON.stringify(value));
    const byOrder = (a, b) => a.sort_order - b.sort_order || a.id - b.id;
    const entityItems = (state, entity) => state[entity === 'list' ? 'lists' : entity === 'section' ? 'sections' : 'items'];
    // Pages embed the snapshot current when they were rendered. It is drawn
    // before IndexedDB opens only when it is at least as new as the stored model
    // and this device holds no unsynced changes; an older copy (for example a
    // page restored from the offline cache) waits for the stored model instead.
    // Both times come from the server clock, so device clock skew does not matter.
    const MODEL_KEY = 'koffan-offline-model';
    function rememberModel(crud) {
        try {
            localStorage.setItem(MODEL_KEY, JSON.stringify({ pending: crud.pendingCount, snapshot_at: crud.snapshotTime }));
        } catch (_) { /* Only disables the early render. */ }
    }
    function bootstrapState() {
        const snapshot = window.offlineBootstrap;
        let model;
        try { model = JSON.parse(localStorage.getItem(MODEL_KEY)); } catch (_) { return null; }
        if (!model || model.pending !== 0 || !Number.isFinite(snapshot?.generated_at) || snapshot.generated_at < model.snapshot_at) return null;
        try { return window.OfflineCRUD.normalizeSnapshot(snapshot); } catch (_) { return null; }
    }
    const common = {
        crudError: '',
        _crudReady: false,
        _crudShown: false,
        _crudConflict: null,
        async initCRUD() {
            if (this._crudInit) return this._crudInit;
            this._crudInit = (async () => {
                const early = bootstrapState();
                if (early) this.renderCRUD(early);
                await window.offlineStorage.init();
                this._crud = new window.OfflineCRUD({
                    storage: window.offlineStorage,
                    request: (url, options) => this.request(url, options),
                    onChange: () => {
                        this._pendingActionCount = this._crud.pendingCount;
                        rememberModel(this._crud);
                        if (this._crudReady) this.renderCRUD();
                    },
                    onError: error => this.crudReportError(error)
                });
                await this._crud.init();
                await this._crud.seed(window.offlineBootstrap || { lists: [], sections: [], items: [] });
                try { await this.migrateLegacyCRUD(); } catch (error) {
                    this.crudError = t('offline.sync_failed');
                    this._legacyMigrationBlocked = true;
                    console.error('Pending offline changes could not be migrated', error);
                }
                this._crudReady = true;
                this.offlineStorageReady = true;
                this._pendingActionCount = this._crud.pendingCount;
                this.renderCRUD();
                this._crudOnline = () => { this.isOnline = true; this._syncNeedsLogin = false; if (!document.hidden) this._realtime?.reconnect(); this.fullRefresh(); };
                this._crudOffline = () => { this.isOnline = false; this._realtime?.pause(); this.renderCRUD(); };
                window.addEventListener('online', this._crudOnline);
                window.addEventListener('offline', this._crudOffline);
                this.fullRefresh();
            })();
            return this._crudInit;
        },
        crudReportError(error) {
            if (!error.status || error.status >= 500) {
                this.isOnline = false;
            } else {
                this.crudError = t('offline.sync_failed');
                this._crudConflict = error.operationId || null;
                this._syncNeedsLogin = error.status === 401 || error.status === 403 || error.status === 409 || error.status === 400;
            }
        },
        async crudDiscardConflict() {
            if (!this._crudConflict || !confirm(t('offline.discard_confirm'))) return;
            await this._crud.discardOperation(this._crudConflict);
            this.crudError = '';
            this._crudConflict = null;
            this._syncNeedsLogin = false;
            this.fullRefresh();
        },
        async crudRetry() {
            if (this._legacyMigrationBlocked) {
                try { await this.migrateLegacyCRUD(); this._legacyMigrationBlocked = false; } catch (_) { return; }
            }
            this._syncNeedsLogin = false;
            this.crudError = '';
            return this.fullRefresh();
        },
        async fullRefresh() {
            if (!this._crudReady || !navigator.onLine || this._syncNeedsLogin || this._legacyMigrationBlocked) return;
            if (this._crudRefresh) return this._crudRefresh;
            this._crudRefresh = (async () => {
                try {
                    await this._crud.sync();
                    this.isOnline = true;
                    this._crudSyncedOnce = true;
                    this.crudError = '';
                    this._crudConflict = null;
                    this.renderCRUD();
                } catch (error) {
                    this.crudReportError(error);
                    this.renderCRUD();
                }
            })().finally(() => {
                this._crudRefresh = null;
                if (this.hasPendingChanges() && !this._syncNeedsLogin) this.scheduleQueueRetry();
                if (this._crudRefreshAgain && !this._syncNeedsLogin) { this._crudRefreshAgain = false; this.fullRefresh(); }
            });
            return this._crudRefresh;
        },
        hasPendingChanges() { return !!this._crud?.pendingCount || this._queueWrites > 0; },
        scheduleQueueRetry() {
            if (this._queueRetryTimer || this._destroyed) return;
            this._queueRetryTimer = setTimeout(() => {
                this._queueRetryTimer = null;
                if (document.visibilityState !== 'hidden') this.fullRefresh();
            }, 5000);
        },
        async crudMutate(entity, action, entity_id, values = {}, extras = {}) {
            return (await this.crudBatch([{ entity, action, entity_id, values, ...extras }]))[0];
        },
        async crudBatch(operations) {
            await this.initCRUD();
            this._queueWrites = (this._queueWrites || 0) + 1;
            this._crudLocalMutation = true;
            try {
                const op = await this._crud.mutateMany(operations);
                this._mutationRevision = (this._mutationRevision || 0) + 1;
                this.renderCRUD();
                return op;
            } catch (error) {
                window.Toast?.show(t('error.update_failed'), 'warning');
                throw error;
            } finally {
                this._queueWrites--;
                this._crudLocalMutation = false;
                this.scheduleQueueRetry();
                this.fullRefresh();
            }
        },
        crudState() { return this._crud.getState(); },
        crudFind(entity, id) {
            const canonical = this._crud.resolveID?.(entity, Number(id)) ?? Number(id);
            return entityItems(this.crudState(), entity).find(row => row.id === canonical);
        },
        currentListId() {
            const id = Number(location.pathname.match(/^\/lists\/(-?\d+)/)?.[1] || new URLSearchParams(location.search).get('list_id') || document.querySelector('[data-list-id]')?.dataset.listId);
            return this._crud?.resolveID?.('list', id) ?? id;
        },
        crudNavigateList(id) {
            id = this._crud?.resolveID?.('list', Number(id)) ?? Number(id);
            location.href = id < 0 ? `/offline/list?list_id=${id}` : `/lists/${id}`;
        },
        async crudCreateList(form) {
            const values = Object.fromEntries(new FormData(form));
            await this.crudMutate('list', 'create', undefined, { name: values.name.trim(), icon: values.icon || '🛒' });
            form.reset();
        },
        async crudUpdateList(id, name, icon) {
            await this.initCRUD();
            const list = this.crudFind('list', id);
            await this.crudMutate('list', 'update', Number(id), { name: name.trim(), icon: icon || list.icon });
        },
        async crudDeleteList(id) {
            await this.initCRUD();
            const list = this.crudFind('list', id);
            if (!list || !confirm(t('lists.delete_confirm', {name: list.name}))) return;
            await this.crudMutate('list', 'delete', Number(id));
        },
        async crudMoveList(id, direction) { return this.crudMoveEntity('list', id, direction); },
        async crudMoveSection(id, direction) { return this.crudMoveEntity('section', id, direction); },
        async crudMoveEntity(entity, id, direction) {
            await this.initCRUD();
            const row = this.crudFind(entity, id);
            const rows = entityItems(this.crudState(), entity).filter(other => entity !== 'section' || other.list_id === row.list_id).sort(byOrder);
            const index = rows.findIndex(other => other.id === row.id);
            const otherIndex = index + (direction === 'up' ? -1 : 1);
            if (otherIndex < 0 || otherIndex >= rows.length) return;
            [rows[index], rows[otherIndex]] = [rows[otherIndex], rows[index]];
            await this.crudBatch(rows.map((row, index) => ({entity, action:'update', entity_id:row.id, values:{sort_order:index}})));
        },
        async crudCreateSection(form) {
            const name = new FormData(form).get('name')?.trim();
            if (!name) return;
            await this.crudMutate('section', 'create', undefined, { list_id: this.currentListId(), name });
            form.reset();
        },
        async crudUpdateSection(id, name) {
            if (!name.trim()) return;
            await this.crudMutate('section', 'update', Number(id), { name: name.trim() });
        },
        async crudDeleteSection(id) {
            await this.initCRUD();
            const section = this.crudFind('section', id);
            if (!section || !confirm(t('confirm.delete_section', {name: section.name}))) return;
            await this.crudMutate('section', 'delete', Number(id));
        },
        async migrateLegacyCRUD() {
            const actions = await window.offlineStorage.getQueuedActions();
            for (const action of actions) {
                const body = new URLSearchParams(typeof action.body === 'string' ? action.body : '');
                const match = action.url?.match(/^\/(items|sections|lists)\/(-?\d+)/);
                const entity = match ? {items:'item',sections:'section',lists:'list'}[match[1]] : action.type === 'create_list' ? 'list' : action.type === 'create_section' ? 'section' : 'item';
                const id = match ? Number(match[2]) : undefined;
                const current = entityItems(this._crud.getState(), entity).find(row => row.id === id);
                let operation;
                if (action.type?.startsWith('create_')) {
                    const allowed = entity === 'item' ? ['name','description','section_id','quantity'] : entity === 'section' ? ['name','list_id'] : ['name','icon'];
                    const values = Object.fromEntries([...body].filter(([key]) => allowed.includes(key)));
                    for (const key of ['section_id','list_id','quantity']) if (key in values) values[key] = Number(values[key]);
                    operation = { entity, action:'create', values };
                } else if (action.method === 'DELETE') operation = {entity, action:'delete',entity_id:id,values:{}};
                else if (action.type === 'toggle_item') operation = {entity:'item',action:'update',entity_id:id,values:{completed:action.completed ?? !current?.completed}};
                else if (action.type === 'toggle_uncertain') operation = {entity:'item',action:'update',entity_id:id,values:{uncertain:!current?.uncertain}};
                else if (action.type === 'adjust_quantity') operation = {entity:'item',action:'update',entity_id:id,values:{quantity:Math.max(0,(current?.quantity || 0)+JSON.parse(action.body).delta)}};
                else if (action.type === 'edit_item' || action.type === 'edit_list') {
                    const values = Object.fromEntries(body);
                    if ('quantity' in values) values.quantity = Number(values.quantity);
                    operation = {entity,action:'update',entity_id:id,values};
                } else if (action.type === 'move_item' || action.type === 'reorder_item') {
                    operation = {entity:'item',action:'update',entity_id:id,values:{section_id:Number(body.get('section_id')),sort_order:Number(body.get('position') || 0)}};
                }
                if (!operation) throw new Error('Unsupported pending legacy action: ' + action.type);
                await this._crud.mutate({...operation, legacy_id:String(action.id)});
                await window.offlineStorage.clearAction(action.id);
            }
        }
    };

    const originalShoppingList = window.shoppingList;
    window.shoppingList = function () {
        const original = originalShoppingList();
        const init = original.init;
        const destroy = original.destroy;
        return Object.assign(original, common, {
            async init() {
                if (this._crudStarted) return;
                this._crudStarted = true;
                return init.call(this);
            },
            initOffline() { return this.initCRUD(); },
            processOfflineQueue() { return this.fullRefresh(); },
            cacheData() { return Promise.resolve(); },
            refreshList() { return this.fullRefresh(); },
            refreshSection() { return this.fullRefresh(); },
            refreshStats() { if (this._crudReady) this.renderCRUD(); },
            refreshSectionSelectsFromServer() { if (this._crudReady) this.renderCRUD(); },
            refreshManageSectionsModal() { if (this._crudReady) this.renderCRUD(); },
            handleMessage(raw) {
                try { if (JSON.parse(raw).type !== 'pong') { if (this._crudRefresh) this._crudRefreshAgain = true; this.fullRefresh(); } } catch (_) {}
            },
            destroy() {
                window.removeEventListener('online', this._crudOnline);
                window.removeEventListener('offline', this._crudOffline);
                this._crud?.close?.();
                destroy.call(this);
            },
            // Draws the durable model, or `state` when given (the page snapshot
            // drawn before the model has loaded).
            renderCRUD(state) {
                if (!state && !this._crudReady) return;
                if (this._pointerDown && !this._crudLocalMutation) { this._needsRefresh = true; return; }
                this._needsRefresh = false;
                state ||= this.crudState();
                const listId = this.currentListId();
                const list = state.lists.find(row => row.id === listId);
                if (!list) {
                    if (this._crudSyncedOnce || this._crudHasRendered) location.href = '/';
                    return;
                }
                if (location.pathname === '/offline/list' && listId > 0) history.replaceState(null, '', `/lists/${listId}`);
                const root = document.querySelector('[data-list-id]');
                root.dataset.listId = listId;
                const heading = root.querySelector('h1');
                if (heading) heading.textContent = list.name;
                const icon = root.querySelector('[data-list-icon]');
                if (icon) icon.textContent = list.icon;
                this.showCompleted = list.show_completed;
                const sections = state.sections.filter(section => section.list_id === listId).sort(byOrder);
                const sectionIds = new Set(sections.map(section => section.id));
                const items = state.items.filter(item => sectionIds.has(item.section_id));
                const destinations = state.lists.filter(row => row.id !== listId).sort(byOrder)
                    .map(row => ({ id: row.id, name: row.name, icon: row.icon, sections: state.sections.filter(section => section.list_id === row.id).sort(byOrder).map(section => ({ id: section.id, name: section.name })) }))
                    .filter(destination => destination.sections.length);
                const stats = { total: items.length, completed: items.filter(item => item.completed).length, percentage: items.length ? Math.round(items.filter(item => item.completed).length / items.length * 100) : 0 };
                const container = document.getElementById('sections-list');
                for (const el of container.querySelectorAll(':scope > [data-section-id]')) {
                    if (!sectionIds.has(Number(el.dataset.sectionId))) { Alpine.destroyTree(el); el.remove(); }
                }
                this._crudRenderedItems ||= new Map();
                const addedSections = [];
                const addedRows = [];
                for (const section of sections) {
                    let el = document.getElementById(`section-${section.id}`);
                    const signature = JSON.stringify([section.name,section.sort_mode,list.show_completed]);
                    if (!el || el.dataset.crudSignature !== signature) {
                        const html = window.OfflineView.section(section, [], sections, list.show_completed);
                        if (el) { el.insertAdjacentHTML('afterend',html); Alpine.destroyTree(el); el.remove(); }
                        else container.insertAdjacentHTML('beforeend',html);
                        el = document.getElementById(`section-${section.id}`);
                        el.dataset.crudSignature = signature;
                        addedSections.push(el);
                    }
                    const sectionItems = items.filter(item => item.section_id === section.id);
                    const desiredIds = new Set(sectionItems.map(item => item.id));
                    for (const row of el.querySelectorAll('.active-items > [data-item-id], .completed-items > [data-item-id]')) {
                        if (!desiredIds.has(Number(row.dataset.itemId))) { Alpine.destroyTree(row); row.remove(); }
                    }
                    for (const item of window.OfflineView.sortItems(sectionItems,section.sort_mode)) {
                        const target = el.querySelector(item.completed ? '.completed-items' : '.active-items');
                        if (!target) continue;
                        let row = document.getElementById(`item-${item.id}`);
                        const before = this._crudRenderedItems.get(item.id);
                        const withoutCompletion = value => JSON.stringify({...value, completed:false,updated_at:0});
                        if (row && before && withoutCompletion(before) === withoutCompletion(item)) {
                            if (before.completed !== item.completed) this._applyOfflineToggle(item.id,section.id,item.completed,false);
                        } else {
                            const html = window.OfflineView.item(item,sections,destinations);
                            if (row) { row.insertAdjacentHTML('afterend',html); Alpine.destroyTree(row); row.remove(); }
                            else target.insertAdjacentHTML('beforeend',html);
                            row = document.getElementById(`item-${item.id}`);
                            addedRows.push(row);
                        }
                        if (row.parentNode !== target) target.appendChild(row);
                        this._crudRenderedItems.set(item.id,clone(item));
                    }
                    for (const target of el.querySelectorAll('.active-items, .completed-items')) {
                        const ordered = window.OfflineView.sortItems(sectionItems.filter(item => item.completed === target.classList.contains('completed-items')),section.sort_mode);
                        let previous = null;
                        for (const item of ordered) {
                            const row = document.getElementById(`item-${item.id}`);
                            if (row && (previous ? previous.nextElementSibling : target.firstElementChild) !== row) target.insertBefore(row,previous ? previous.nextElementSibling : target.firstElementChild);
                            previous = row;
                        }
                    }
                    this.updateSectionCounter(el); this.updateCompletedCount(el); this.updateCompletedVisibility(el);
                    const toggleAll = el.querySelector('[data-crud-toggle-all]');
                    if (toggleAll) { const label = t(sectionItems.some(item => !item.completed) ? 'items.check_all' : 'items.uncheck_all'); toggleAll.setAttribute('aria-label', label); toggleAll.title = label; }
                }
                let previous = null;
                for (const section of sections) {
                    const el = document.getElementById(`section-${section.id}`);
                    if ((previous ? previous.nextElementSibling : container.firstElementChild) !== el) container.insertBefore(el,previous ? previous.nextElementSibling : container.firstElementChild);
                    previous = el;
                }
                const selectSignature = JSON.stringify(sections.map(section => [section.id,section.name]));
                if (this._crudSectionsSignature !== selectSignature) {
                    this.updateSectionSelects(sections);
                    this.refreshAllMoveMenus(sections);
                    this._crudSectionsSignature = selectSignature;
                    const manage = document.getElementById('manage-sections-list');
                    if (manage) manage.innerHTML = window.OfflineView.manageSections(sections);
                }
                const destinationSignature = JSON.stringify(destinations);
                if (this._crudDestinationsSignature !== destinationSignature) {
                    this.refreshListMoveMenus(destinations);
                    this._crudDestinationsSignature = destinationSignature;
                }
                const switcher = document.getElementById('crud-list-options');
                if (switcher) {
                    const signature = JSON.stringify(state.lists);
                    if (switcher.dataset.signature !== signature) { switcher.innerHTML = window.OfflineView.listOptions(state.lists); switcher.dataset.signature = signature; }
                }
                window.checkEmptyStates();
                this.stats = stats;
                this._crudHasRendered = true;
                this._crudShown = true;
                this.$nextTick(() => {
                    // Alpine skips new nodes moved by sorting in the same mutation batch.
                    // Clean up any observer initialization before installing one handler set.
                    const trees = [...addedSections, ...addedRows.filter(row => !addedSections.some(section => section.contains(row)))];
                    for (const el of trees) if (el.isConnected) { Alpine.destroyTree(el); Alpine.initTree(el); }
                    this.initMobileSortable();
                });
            },
            // Keeps the "move to another list" menus of rows and of the mobile
            // action sheet in step with the lists and sections of the model.
            refreshListMoveMenus(destinations) {
                for (const el of document.querySelectorAll('.item-move-list')) {
                    el.classList.toggle('hidden', !destinations.length);
                    const menu = el.querySelector('[data-list-moves]');
                    if (menu) menu.innerHTML = window.OfflineView.listMoveMenu(el.dataset.itemId, destinations);
                }
                const mobile = document.getElementById('mobile-move-list');
                if (mobile) {
                    mobile.classList.toggle('hidden', !destinations.length);
                    const options = document.getElementById('mobile-move-list-options');
                    if (options) options.innerHTML = window.OfflineView.mobileListMoveMenu(destinations);
                }
            },
            async moveItemToList(id, sectionId) {
                await this.initCRUD();
                const state = this.crudState();
                const section = state.sections.find(row => row.id === Number(sectionId));
                const list = section && state.lists.find(row => row.id === section.list_id);
                await this.crudMoveItem(id, sectionId);
                // Toast.show renders its message as HTML, so names go in escaped.
                if (list) window.Toast?.show(t('actions.moved_to_list', { list: window.OfflineView.escape(list.name) }), 'success');
            },
            async moveToListSection(sectionId) {
                if (!this.mobileActionItem) return;
                const id = this.mobileActionItem.id;
                this.mobileActionItem = null;
                await this.moveItemToList(id, sectionId);
            },
            async toggleItem(id) {
                await this.initCRUD();
                const item = this.crudFind('item',id);
                if (item) await this.crudMutate('item','update',item.id,{completed:!item.completed});
            },
            async submitEditItem() {
                if (!this.editingItem || !this.editItemName.trim()) return;
                await this.crudMutate('item','update',this.editingItem.id,{name:this.editItemName.trim(),description:this.editItemDescription.trim(),quantity:Number(this.editItemQuantity)||0});
                this.editingItem = null;
            },
            async crudAddItem(sectionId,name,description='',quantity=0) {
                await this.initCRUD();
                const state = this.crudState();
                const existing = state.items.find(item => item.section_id === Number(sectionId) && item.name.replace(/[A-Z]/g,c=>c.toLowerCase()) === name.replace(/[A-Z]/g,c=>c.toLowerCase()));
                if (existing) {
                    if (existing.completed) await this.crudMutate('item','update',existing.id,{completed:false});
                    return;
                }
                return this.crudMutate('item','create',undefined,{section_id:Number(sectionId),name,description,quantity:Number(quantity)||0});
            },
            async submitAddItemForm(form) {
                const values = Object.fromEntries(new FormData(form));
                if (!values.name?.trim()) return;
                await this.crudAddItem(values.section_id,values.name.trim(),values.description?.trim()||'',values.quantity);
                this._resetDesktopAddForm(form);
            },
            async submitAddItemFormMobile(form) {
                const values = Object.fromEntries(new FormData(form));
                if (!values.name?.trim()) return;
                await this.crudAddItem(values.section_id,values.name.trim(),values.description?.trim()||'',values.quantity);
                this._resetMobileAddForm(form);
            },
            async submitQuickAdd(sectionId) {
                if (!this.quickAddName.trim()) return;
                await this.crudAddItem(sectionId,this.quickAddName.trim());
                this.closeQuickAdd();
            },
            async deleteItemDirect(id) { await this.crudMutate('item','delete',Number(id)); },
            async deleteItem() {
                if (!this.mobileActionItem || !confirm(t('confirm.delete_item',{name:this.mobileActionItem.name}))) return;
                await this.deleteItemDirect(this.mobileActionItem.id);
                this.mobileActionItem = null;
            },
            async deleteCompletedItems() {
                if (!confirm(t('confirm.delete_completed_items'))) return;
                await this.initCRUD();
                const state = this.crudState();
                const ids = new Set(state.sections.filter(section => section.list_id === this.currentListId()).map(section => section.id));
                await this.crudBatch(state.items.filter(item => ids.has(item.section_id) && item.completed).map(item => ({entity:'item',action:'delete',entity_id:item.id,values:{}})));
                this.showSettings = false;
            },
            async adjustQuantity(delta) {
                if (!this.mobileActionItem) return;
                await this.initCRUD();
                const item = this.crudFind('item',this.mobileActionItem.id);
                const quantity = Math.max(0,Math.min(999,item.quantity+delta));
                await this.crudMutate('item','update',item.id,{quantity});
                this.mobileActionItem.quantity = quantity;
            },
            async toggleUncertainFetch(id) {
                await this.initCRUD();
                const item = this.crudFind('item',id);
                await this.crudMutate('item','update',item.id,{uncertain:!item.uncertain});
            },
            async toggleUncertain() {
                if (!this.mobileActionItem) return;
                await this.toggleUncertainFetch(this.mobileActionItem.id);
                this.mobileActionItem = null;
            },
            async moveItemDesktop(id,fromId,toId) { await this.crudMoveItem(id,toId); },
            async moveToSection(sectionId) {
                if (!this.mobileActionItem) return;
                await this.crudMoveItem(this.mobileActionItem.id,sectionId);
                this.mobileActionItem = null;
            },
            async crudMoveItem(id,sectionId,position) {
                await this.initCRUD();
                const rows = this.crudState().items.filter(item => item.section_id === Number(sectionId) && item.id !== Number(id)).sort(byOrder);
                const sort_order = position ?? Math.max(-1,...rows.map(item => item.sort_order))+1;
                const operations = position === undefined ? [] : rows.filter(item => item.sort_order >= sort_order).map(row => ({entity:'item',action:'update',entity_id:row.id,values:{sort_order:row.sort_order+1}}));
                operations.push({entity:'item',action:'update',entity_id:Number(id),values:{section_id:Number(sectionId),sort_order}});
                await this.crudBatch(operations);
            },
            async syncItemPosition(id,sectionId,index) {
                const container = document.getElementById(`section-${sectionId}`)?.querySelector('.active-items');
                const rows = Array.from(container?.children || []).filter(row => row.dataset.itemId);
                await this.crudBatch(rows.map((row,index) => ({entity:'item',action:'update',entity_id:Number(row.dataset.itemId),values:{sort_order:index}})));
            },
            async moveItemToSection(id,fromId,toId,index) { await this.crudMoveItem(id,toId,index); },
            async cycleSortMode(id,mode) {
                const modes=['manual','alphabetical','alphabetical_desc'];
                await this.crudMutate('section','update',Number(id),{sort_mode:modes[(modes.indexOf(mode)+1)%3]});
            },
            async toggleAllItems(id) {
                await this.initCRUD();
                const items = this.crudState().items.filter(item=>item.section_id===Number(id));
                const completed = items.some(item=>!item.completed);
                await this.crudBatch(items.filter(item => item.completed!==completed).map(item => ({entity:'item',action:'update',entity_id:item.id,values:{completed}})));
            },
            async toggleShowCompleted(id) {
                await this.initCRUD();
                const list = this.crudFind('list',id);
                await this.crudMutate('list','update',list.id,{show_completed:!list.show_completed});
            },
            async deleteSelectedSections() {
                if (!this.selectedSections.length || !confirm(t('confirm.delete_sections',{count:this.selectedSections.length}))) return;
                await this.crudBatch(this.selectedSections.map(id => ({entity:'section',action:'delete',entity_id:Number(id),values:{}})));
                this.selectedSections=[]; this.selectMode=false;
            }
        });
    };
    if (typeof window.homePage === 'function') {
        const originalHomePage = window.homePage;
        window.homePage = function () {
            return Object.assign(originalHomePage(), common, {
                request: originalShoppingList().request,
                crudHasLists: false,
                async init() {
                    if (this._crudStarted) return;
                    this._crudStarted = true;
                    await this.initCRUD();
                    this._realtime = new window.KoffanRealtime({url:()=>`${location.protocol==='https:'?'wss:':'ws:'}//${location.host}/ws`,onConnected:()=>this.fullRefresh(),onMessage:()=>{ if (this._crudRefresh) this._crudRefreshAgain = true; this.fullRefresh(); }});
                    this._realtime.start();
                    this._crudVisibility = () => { if(document.hidden) this._realtime.pause(); else { this._realtime.reconnect(); this.fullRefresh(); } };
                    document.addEventListener('visibilitychange',this._crudVisibility);
                },
                destroy() {
                    this._destroyed=true;
                    this._realtime?.stop(); this._crud?.close?.(); clearTimeout(this._queueRetryTimer);
                    window.removeEventListener('online',this._crudOnline); window.removeEventListener('offline',this._crudOffline);
                    document.removeEventListener('visibilitychange',this._crudVisibility);
                },
                renderCRUD(state) {
                    if (!state && !this._crudReady) return;
                    state ||= this.crudState();
                    this.crudHasLists = state.lists.length > 0;
                    const container=document.getElementById('lists-container');
                    if (!container) return;
                    if (!this._crudLocalMutation && container.contains(document.activeElement) && document.activeElement.matches('input,textarea')) {
                        document.activeElement.addEventListener('blur', () => this.renderCRUD(), {once:true}); return;
                    }
                    this._crudShown = true;
                    const signature=JSON.stringify({lists:state.lists,sections:state.sections,items:state.items});
                    if (container.dataset.crudSignature===signature) return;
                    container.innerHTML=window.OfflineView.lists(state.lists,state.items,state.sections);
                    container.dataset.crudSignature=signature;
                },
                async submitList() {
                    if (!this.listName.trim()) return;
                    try {
                        if (this.editingList) await this.crudUpdateList(this.editingList.id,this.listName,this.selectedIcon);
                        else await this.crudMutate('list','create',undefined,{name:this.listName.trim(),icon:this.selectedIcon});
                        this.showNewListModal=false; this.editingList=null; this.listName=''; this.listNameError='';
                    } catch (_) { this.listNameError=t('error.update_failed'); }
                },
                deleteList(id) { return this.crudDeleteList(id); },
                moveList(id,direction) { return this.crudMoveList(id,direction); }
            });
        };
    }
})();
