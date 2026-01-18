// Toast Notification System
window.Toast = {
    container: null,

    init() {
        this.container = document.getElementById('toast-container');
    },

    show(message, type = 'info', duration = 3000) {
        if (!this.container) this.init();
        if (!this.container) return;

        const toast = document.createElement('div');
        const baseClasses = 'min-w-[280px] max-w-[90vw] px-5 py-3 rounded-2xl shadow-xl text-sm font-medium flex items-center gap-3 transform transition-all duration-300 ease-out';
        const typeClasses = {
            warning: 'bg-rose-100 dark:bg-rose-900/50 text-rose-700 dark:text-rose-300 border border-rose-200 dark:border-rose-800',
            info: 'bg-stone-100 dark:bg-stone-700 text-stone-700 dark:text-stone-200 border border-stone-200 dark:border-stone-600',
            success: 'bg-emerald-100 dark:bg-emerald-900/50 text-emerald-700 dark:text-emerald-300 border border-emerald-200 dark:border-emerald-800'
        };

        toast.className = `${baseClasses} ${typeClasses[type] || typeClasses.info}`;
        toast.innerHTML = `
            <svg class="w-5 h-5 flex-shrink-0" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                ${type === 'warning' ? '<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M18.364 5.636a9 9 0 010 12.728m0 0l-2.829-2.829m2.829 2.829L21 21M15.536 8.464a5 5 0 010 7.072m0 0l-2.829-2.829m-4.243 2.829a4.978 4.978 0 01-1.414-2.83m-1.414 5.658a9 9 0 01-2.167-9.238m7.824 2.167a1 1 0 111.414 1.414m-1.414-1.414L3 3m8.293 8.293l1.414 1.414"></path>' : ''}
                ${type === 'success' ? '<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 13l4 4L19 7"></path>' : ''}
                ${type === 'info' ? '<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13 16h-1v-4h-1m1-4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z"></path>' : ''}
            </svg>
            <span class="flex-1">${message}</span>
        `;

        // Start hidden
        toast.style.opacity = '0';
        toast.style.transform = 'translateY(1rem)';

        this.container.appendChild(toast);

        // Animate in
        requestAnimationFrame(() => {
            toast.style.opacity = '1';
            toast.style.transform = 'translateY(0)';
        });

        // Remove after duration
        setTimeout(() => {
            toast.style.opacity = '0';
            toast.style.transform = 'translateY(1rem)';
            setTimeout(() => toast.remove(), 300);
        }, duration);
    }
};

// Form error handler for section creation
window.handleSectionFormError = function(form, xhr) {
    // Remove any existing error message
    form.querySelector('.error-msg')?.remove();

    // Get error text from response
    let errorText = xhr.responseText || 'Error';

    // Translate known error codes
    if (errorText === 'This name is reserved for system use') {
        errorText = window.getTranslation?.('common.reserved_name') || 'Ta nazwa jest zarezerwowana dla systemu';
    }

    // Create and insert error message
    const errorDiv = document.createElement('div');
    errorDiv.className = 'error-msg text-sm text-red-500';
    errorDiv.textContent = errorText;
    form.appendChild(errorDiv);
};

// Shopping List Alpine.js Component
function shoppingList() {
    return {
        // WebSocket
        ws: null,
        connected: false,
        reconnectAttempts: 0,
        maxReconnectAttempts: 5,

        // Offline support
        isOnline: navigator.onLine,
        processingQueue: false,
        offlineStorageReady: false,

        // Modals
        showManageSections: false,
        showAddItem: false,
        addMore: false,
        showEditModal: false,
        showSettings: false,
        showOfflineModal: false,

        // Section management
        selectMode: false,
        selectedSections: [],

        // History management
        showHistoryModal: false,
        historyItems: [],
        historySearch: '',
        selectedHistoryIds: [],
        historySectionMode: localStorage.getItem('history_section_mode') || 'use_first_section',

        // Import/Export
        showImportPreview: false,
        importPreview: {},
        importFile: null,
        importConflictResolution: 'skip',

        // Stats (updated from server)
        stats: {
            total: window.initialStats?.total || 0,
            completed: window.initialStats?.completed || 0,
            percentage: window.initialStats?.percentage || 0
        },

        // Current item for mobile actions
        mobileActionItem: null,

        // Edit item
        editingItem: null,
        editItemName: '',
        editItemDescription: '',

        // Auto-completion
        suggestions: [],
        showSuggestions: false,
        selectedSuggestionIndex: -1,
        itemNameInput: '',
        _suggestionTimer: null,

        // Track pending local actions to avoid WebSocket race conditions
        pendingLocalActions: {},
        localActionTimeout: 1000, // ms to ignore WebSocket updates after local action

        // Debounce timers for refresh
        _refreshListTimer: null,
        _refreshStatsTimer: null,
        _isRefreshing: false,

        // Check if add-item form is currently active (to prevent dropdown updates during form use)
        _isAddFormActive() {
            // Check if mobile add-item modal is open
            if (this.showAddItem) {
                return true;
            }
            // Check if desktop form has focus (name input or section select)
            const desktopForm = document.getElementById('add-item-form');
            if (desktopForm) {
                const activeEl = document.activeElement;
                if (desktopForm.contains(activeEl)) {
                    return true;
                }
            }
            return false;
        },

        async init() {
            await this.initOffline();
            this.initWebSocket();
            this.initCompletedSectionsStore();
            this.initLocalActionTracking();
            this.cacheSuggestions();

            // Listen for mobile action modal
            this.$el.addEventListener('open-mobile-action', (e) => {
                this.openMobileAction(e.detail);
            });

            // Listen for uncertain toggle events (use window because $dispatch bubbles to window)
            window.addEventListener('toggle-uncertain', (e) => {
                this.toggleUncertainAnimated(e.detail.id);
            });

            // Keyboard shortcut for save (Cmd+Enter)
            document.addEventListener('keydown', (e) => {
                if ((e.metaKey || e.ctrlKey) && e.key === 'Enter' && this.editingItem) {
                    e.preventDefault();
                    this.submitEditItem();
                }
            });

            // Initialize mobile drag-and-drop
            this.$nextTick(() => {
                this.initMobileSortable();
            });

            // Re-initialize sortable after HTMX swaps
            document.body.addEventListener('htmx:afterSwap', (e) => {
                if (e.detail.target?.id === 'sections-list') {
                    this.$nextTick(() => this.initMobileSortable());
                }
            });
        },

        initCompletedSectionsStore() {
            // Load state from localStorage
            try {
                const saved = localStorage.getItem('completedSections');
                Alpine.store('completedSections', saved ? JSON.parse(saved) : {});
            } catch (e) {
                Alpine.store('completedSections', {});
            }
        },

        saveCompletedSections() {
            try {
                const store = Alpine.store('completedSections');
                localStorage.setItem('completedSections', JSON.stringify(store));
            } catch (e) {
                console.error('Failed to save completed sections:', e);
            }
        },

        initLocalActionTracking() {
            // Listen for HTMX requests to track local actions
            document.body.addEventListener('htmx:beforeRequest', (e) => {
                const path = e.detail.requestConfig?.path || '';
                // Track reorder actions
                if (path.includes('/move-up') || path.includes('/move-down')) {
                    this.markLocalAction('items_reordered');
                }
                // Track delete actions
                if (e.detail.requestConfig?.verb === 'delete' && path.includes('/items/')) {
                    this.markLocalAction('item_deleted');
                }
                // Track uncertain toggle
                if (path.includes('/uncertain')) {
                    this.markLocalAction('item_updated');
                }
                // Track item toggle (checked/unchecked)
                if (path.includes('/toggle')) {
                    this.markLocalAction('item_toggled');
                }
            });
        },

        markLocalAction(actionType) {
            this.pendingLocalActions[actionType] = Date.now();
            // Auto-clear after timeout
            setTimeout(() => {
                if (this.pendingLocalActions[actionType] &&
                    Date.now() - this.pendingLocalActions[actionType] >= this.localActionTimeout) {
                    delete this.pendingLocalActions[actionType];
                }
            }, this.localActionTimeout + 100);
        },

        isLocalAction(actionType) {
            const timestamp = this.pendingLocalActions[actionType];
            if (timestamp && Date.now() - timestamp < this.localActionTimeout) {
                return true;
            }
            return false;
        },

        // ===== OFFLINE SUPPORT =====

        async initOffline() {
            // Initialize IndexedDB
            try {
                await window.offlineStorage.init();
                this.offlineStorageReady = true;
                console.log('[App] Offline storage initialized');

                // Process any pending offline actions first (retry after reload)
                if (this.isOnline) {
                    const pendingCount = await window.offlineStorage.getQueueLength();
                    if (pendingCount > 0) {
                        console.log('[App] Found', pendingCount, 'pending offline actions, syncing...');
                        await this.processOfflineQueue();
                    }
                    this.cacheData();
                }
            } catch (error) {
                console.error('[App] Failed to initialize offline storage:', error);
            }

            // Online/offline event listeners
            window.addEventListener('online', async () => {
                // Prevent double execution
                if (this._onlineHandled) return;
                this._onlineHandled = true;

                console.log('[App] Back online');
                this.isOnline = true;

                // Sync offline actions and refresh (no page reload)
                const hadActions = await this.processOfflineQueue();
                window.Toast.show(t('offline.back_online'), 'success', 2000);

                // Only refresh if no queued actions (processOfflineQueue already refreshes)
                if (!hadActions) {
                    this.refreshList();
                    this.refreshStats();
                }
            });

            window.addEventListener('offline', () => {
                console.log('[App] Gone offline');
                this.isOnline = false;
                this._onlineHandled = false; // Reset for next online event
            });
        },

        async cacheData() {
            if (!this.offlineStorageReady) return;

            try {
                const response = await fetch('/api/data');
                if (response.ok) {
                    const data = await response.json();
                    await window.offlineStorage.saveSections(data.sections || []);
                    await window.offlineStorage.setLastSyncTimestamp(data.timestamp);
                    console.log('[App] Data cached for offline use');
                }
            } catch (error) {
                console.error('[App] Failed to cache data:', error);
            }
        },

        async queueOfflineAction(action) {
            if (!this.offlineStorageReady) {
                console.warn('[App] Offline storage not ready, action lost:', action);
                return;
            }

            await window.offlineStorage.queueAction(action);
            console.log('[App] Action queued for sync:', action.type);
        },

        async processOfflineQueue() {
            if (this.processingQueue || !this.isOnline || !this.offlineStorageReady) return false;

            this.processingQueue = true;
            console.log('[App] Processing offline queue...');

            try {
                const actions = await window.offlineStorage.getQueuedActions();

                if (actions.length === 0) {
                    console.log('[App] No queued actions');
                    this.processingQueue = false;
                    return false;
                }

                console.log('[App] Processing', actions.length, 'queued actions');

                for (const action of actions) {
                    try {
                        // For all modifying actions - check server version (Last Write Wins)
                        if (action.type === 'toggle_item' || action.type === 'update_item' || action.type === 'edit_item') {
                            const itemId = this.extractItemId(action.url);
                            if (itemId) {
                                const serverVersion = await this.getItemVersion(itemId);
                                if (serverVersion && serverVersion.updated_at > action.timestamp) {
                                    // Server has newer version - skip offline action
                                    console.log('[Sync] Server version newer, skipping:', action.type,
                                        'server:', serverVersion.updated_at, 'offline:', action.timestamp);
                                    await window.offlineStorage.clearAction(action.id);
                                    continue;
                                }
                            }
                        }

                        const fetchOptions = {
                            method: action.method,
                            headers: action.headers || {}
                        };

                        if (action.body) {
                            fetchOptions.body = action.body;
                        }

                        const response = await fetch(action.url, fetchOptions);

                        if (response.ok || response.status === 404) {
                            // Success or item no longer exists - remove from queue
                            await window.offlineStorage.clearAction(action.id);
                            console.log('[App] Synced action:', action.type);
                        } else {
                            console.error('[App] Failed to sync action:', action.type, response.status);
                        }
                    } catch (error) {
                        console.error('[App] Error syncing action:', action.type, error);
                        // Keep in queue for retry
                    }
                }

                // Refresh data after sync - small delay to ensure server processed all changes
                await new Promise(resolve => setTimeout(resolve, 150));
                await this.cacheData();

                // Force full htmx refresh of sections list
                const sectionsList = document.getElementById('sections-list');
                if (sectionsList) {
                    const refreshUrl = window.location.pathname.startsWith('/lists/')
                        ? window.location.pathname
                        : '/';
                    await htmx.ajax('GET', refreshUrl, {
                        target: '#sections-list',
                        swap: 'innerHTML',
                        select: '#sections-list > *'
                    });
                    // Reinitialize sortable after swap
                    this.$nextTick(() => this.initMobileSortable());
                }
                this.refreshStats();
                console.log('[App] Offline queue processed, UI refreshed');

                return true; // Had queued actions

            } finally {
                this.processingQueue = false;
            }
        },

        // Get item version from server for conflict resolution
        async getItemVersion(itemId) {
            try {
                const response = await fetch(`/api/item/${itemId}/version`);
                if (response.ok) {
                    return await response.json();
                }
            } catch (e) {
                console.error('[Sync] Failed to get item version:', e);
            }
            return null;
        },

        // Extract item ID from URL like /items/123/toggle
        extractItemId(url) {
            const match = url.match(/\/items\/(\d+)/);
            return match ? match[1] : null;
        },

        async fullRefresh() {
            console.log('[App] Full refresh triggered');

            // Reconnect WebSocket if needed
            if (!this.connected && this.isOnline) {
                this.reconnectAttempts = 0;
                this.connect();
            }

            // Process any pending offline actions first (this already calls refreshList/Stats)
            if (this.isOnline) {
                const hadQueuedActions = await this.processOfflineQueue();

                // Only refresh if there were no queued actions (processOfflineQueue already did it)
                if (!hadQueuedActions) {
                    this.refreshList();
                    this.refreshStats();
                }

                this.cacheData();
            }
        },

        // Wrapper for fetch that queues action when offline
        async offlineFetch(url, options, actionType) {
            if (this.isOnline) {
                return fetch(url, options);
            }

            // Queue action for later sync
            await this.queueOfflineAction({
                type: actionType,
                url: url,
                method: options.method || 'GET',
                headers: options.headers || {},
                body: options.body || null
            });

            // Return fake successful response
            return { ok: true, offline: true };
        },

        // ===== WEBSOCKET =====

        initWebSocket() {
            this.connect();

            document.addEventListener('visibilitychange', () => {
                if (document.visibilityState === 'visible') {
                    // Full refresh when returning from background
                    this.fullRefresh();
                }
            });
        },

        connect() {
            const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
            const wsUrl = `${protocol}//${window.location.host}/ws`;

            try {
                this.ws = new WebSocket(wsUrl);

                this.ws.onopen = () => {
                    console.log('WebSocket connected');
                    this.connected = true;
                    this.reconnectAttempts = 0;
                };

                this.ws.onclose = () => {
                    console.log('WebSocket disconnected');
                    this.connected = false;
                    this.scheduleReconnect();
                };

                this.ws.onerror = (error) => {
                    console.error('WebSocket error:', error);
                };

                this.ws.onmessage = (event) => {
                    this.handleMessage(event.data);
                };

                this.startPingPong();
            } catch (error) {
                console.error('Failed to create WebSocket:', error);
                this.scheduleReconnect();
            }
        },

        scheduleReconnect() {
            if (this.reconnectAttempts >= this.maxReconnectAttempts) {
                console.log('Max reconnection attempts reached');
                return;
            }

            this.reconnectAttempts++;
            const delay = Math.min(1000 * Math.pow(2, this.reconnectAttempts), 30000);
            setTimeout(() => this.connect(), delay);
        },

        startPingPong() {
            setInterval(() => {
                if (this.ws && this.ws.readyState === WebSocket.OPEN) {
                    this.ws.send(JSON.stringify({ type: 'ping' }));
                }
            }, 30000);
        },

        handleMessage(data) {
            try {
                const message = JSON.parse(data);
                console.log('WebSocket message:', message.type);

                switch (message.type) {
                    case 'section_created':
                    case 'section_updated':
                    case 'section_deleted':
                    case 'sections_deleted':
                    case 'sections_reordered':
                        // Sections changed - refresh sections list and selects
                        this.refreshSectionsAndSelects();
                        break;
                    case 'item_created':
                    case 'item_moved':
                        // Requires full list refresh
                        this.refreshList();
                        this.refreshStats();
                        break;
                    case 'item_deleted':
                        // If local action - HTMX already deleted element
                        // If remote - refresh list to sync
                        if (!this.isLocalAction('item_deleted')) {
                            this.refreshList();
                        }
                        this.refreshStats();
                        break;
                    case 'items_reordered':
                        // If local action - HTMX already updated order
                        // If remote - refresh list to sync
                        if (!this.isLocalAction('items_reordered')) {
                            this.refreshList();
                        }
                        this.refreshStats();
                        break;
                    case 'item_toggled':
                        // If local action - HTMX already updated element
                        // If remote - refresh list to sync
                        if (!this.isLocalAction('item_toggled')) {
                            this.refreshList();
                        }
                        this.refreshStats();
                        break;
                    case 'item_updated':
                        // If local action - HTMX already updated element
                        // If remote - refresh list to sync
                        if (!this.isLocalAction('item_updated')) {
                            this.refreshList();
                        }
                        this.refreshStats();
                        break;
                    case 'completed_items_deleted':
                        // All purchased items were deleted
                        this.refreshList();
                        this.refreshStats();
                        break;
                    case 'pong':
                        break;
                    default:
                        console.log('Unknown message type:', message.type);
                }
            } catch (error) {
                console.error('Failed to parse WebSocket message:', error);
            }
        },

        refreshList() {
            // Debounce - prevent multiple rapid refreshes
            if (this._refreshListTimer) {
                clearTimeout(this._refreshListTimer);
            }

            this._refreshListTimer = setTimeout(() => {
                if (this._isRefreshing) return;
                this._isRefreshing = true;

                const overlay = document.getElementById('refresh-overlay');
                const sectionsList = document.getElementById('sections-list');

                if (sectionsList) {
                    // Show overlay
                    if (overlay) {
                        overlay.classList.add('active');
                    }

                    // Use current URL if on a list page, otherwise use /
                    const refreshUrl = window.location.pathname.startsWith('/lists/')
                        ? window.location.pathname
                        : '/';

                    htmx.ajax('GET', refreshUrl, {
                        target: '#sections-list',
                        swap: 'innerHTML',
                        select: '#sections-list > *'
                    }).then(() => {
                        // Small delay then fade out overlay
                        setTimeout(() => {
                            if (overlay) {
                                overlay.classList.add('fade-out');
                                setTimeout(() => {
                                    overlay.classList.remove('active', 'fade-out');
                                    this._isRefreshing = false;
                                }, 250);
                            } else {
                                this._isRefreshing = false;
                            }
                        }, 50);
                    }).catch(() => {
                        if (overlay) {
                            overlay.classList.remove('active', 'fade-out');
                        }
                        this._isRefreshing = false;
                    });
                } else {
                    this._isRefreshing = false;
                }

                const manageSectionsList = document.getElementById('manage-sections-list');
                if (manageSectionsList) {
                    htmx.ajax('GET', '/sections/list', {
                        target: '#manage-sections-list',
                        swap: 'innerHTML'
                    });
                }
            }, 100); // 100ms debounce
        },

        refreshSection(sectionId) {
            const section = document.getElementById(`section-${sectionId}`);
            if (section) {
                const refreshUrl = window.location.pathname.startsWith('/lists/')
                    ? window.location.pathname
                    : '/';
                htmx.ajax('GET', refreshUrl, {
                    target: `#section-${sectionId}`,
                    swap: 'outerHTML',
                    select: `#section-${sectionId}`
                });
            }
        },

        refreshItem(itemId) {
            const item = document.getElementById(`item-${itemId}`);
            if (item) {
                const refreshUrl = window.location.pathname.startsWith('/lists/')
                    ? window.location.pathname
                    : '/';
                htmx.ajax('GET', refreshUrl, {
                    target: `#item-${itemId}`,
                    swap: 'outerHTML',
                    select: `#item-${itemId}`
                });
            }
        },

        async refreshSectionsAndSelects() {
            // Refresh sections list in management modal
            const manageSectionsList = document.getElementById('manage-sections-list');
            if (manageSectionsList) {
                htmx.ajax('GET', '/sections/list', {
                    target: '#manage-sections-list',
                    swap: 'innerHTML'
                }).then(() => {
                    // Reinitialize Alpine.js for new elements after HTMX swap
                    const el = document.getElementById('manage-sections-list');
                    if (el) {
                        Alpine.initTree(el);
                    }
                });
            }

            // Refresh main sections list
            this.refreshList();

            // Fetch new sections and update selects
            try {
                const response = await fetch('/sections/list?format=json');
                if (response.ok) {
                    const sections = await response.json();
                    this.updateSectionSelects(sections);
                }
            } catch (error) {
                console.error('Failed to refresh sections:', error);
            }
        },

        updateSectionSelects(sections) {
            // Skip if user is actively using the add-item form (prevents race condition)
            if (this._isAddFormActive()) {
                console.log('[App] Skipping section select update - form is active');
                return;
            }

            // Find all section selects
            const selects = document.querySelectorAll('select[name="section_id"]');
            selects.forEach(select => {
                const currentValue = select.value;

                // Clear all options
                select.innerHTML = '';

                // Add new options (first will be selected by default by browser)
                sections.forEach((section) => {
                    const opt = document.createElement('option');
                    opt.value = section.id;
                    opt.textContent = section.name;
                    select.appendChild(opt);
                });

                // Restore previous value if still exists
                if (currentValue && sections.some(s => s.id == currentValue)) {
                    select.value = currentValue;
                }
            });
        },

        refreshStats() {
            // Debounce - prevent multiple rapid stat refreshes
            if (this._refreshStatsTimer) {
                clearTimeout(this._refreshStatsTimer);
            }

            this._refreshStatsTimer = setTimeout(async () => {
                try {
                    const response = await fetch('/stats');
                    if (response.ok) {
                        const data = await response.json();
                        // JSON uses snake_case
                        this.stats = {
                            total: data.total_items || 0,
                            completed: data.completed_items || 0,
                            percentage: data.percentage || 0
                        };
                    }
                } catch (error) {
                    console.error('Failed to refresh stats:', error);
                }
            }, 100); // 100ms debounce
        },

        // Section Management
        toggleSection(id) {
            const index = this.selectedSections.indexOf(id);
            if (index > -1) {
                this.selectedSections.splice(index, 1);
            } else {
                this.selectedSections.push(id);
            }
        },

        async deleteSelectedSections() {
            if (this.selectedSections.length === 0) return;
            if (!this.isOnline) {
                window.Toast.show(t('offline.action_blocked'), 'warning');
                return;
            }

            const confirmed = confirm(t('confirm.delete_sections', { count: this.selectedSections.length }));
            if (!confirmed) return;

            try {
                const response = await fetch('/sections/batch-delete', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
                    body: `ids=${this.selectedSections.join(',')}`
                });

                if (response.ok) {
                    this.selectMode = false;
                    this.selectedSections = [];
                    // Refresh sections list and selects without page reload
                    this.refreshSectionsAndSelects();
                }
            } catch (error) {
                console.error('Failed to delete sections:', error);
            }
        },

        // Mobile Action Modal
        openMobileAction(item) {
            this.mobileActionItem = {
                id: item.id,
                name: item.name,
                description: item.description || '',
                section_id: item.section_id,
                uncertain: item.uncertain
            };
        },

        closeMobileAction() {
            this.mobileActionItem = null;
        },

        async toggleUncertain() {
            if (!this.mobileActionItem) return;
            const itemId = this.mobileActionItem.id;

            // Mark as local action to prevent WebSocket race condition
            this.markLocalAction('item_updated');

            // Optimistic UI update
            this.mobileActionItem.uncertain = !this.mobileActionItem.uncertain;
            this.mobileActionItem = null;

            try {
                const response = await this.offlineFetch(
                    `/items/${itemId}/uncertain`,
                    { method: 'POST' },
                    'toggle_uncertain'
                );

                if (response.ok && !response.offline) {
                    this.refreshList();
                }
            } catch (error) {
                console.error('Failed to toggle uncertain:', error);
                // Queue for offline sync
                await this.queueOfflineAction({
                    type: 'toggle_uncertain',
                    url: `/items/${itemId}/uncertain`,
                    method: 'POST'
                });
            }

            this.refreshList();
        },

        async moveToSection(sectionId) {
            if (!this.mobileActionItem) return;
            if (!this.isOnline) {
                window.Toast.show(t('offline.action_blocked'), 'warning');
                this.mobileActionItem = null;
                return;
            }
            const itemId = this.mobileActionItem.id;
            this.mobileActionItem = null;

            try {
                const response = await this.offlineFetch(
                    `/items/${itemId}/move`,
                    {
                        method: 'POST',
                        headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
                        body: `section_id=${sectionId}`
                    },
                    'move_item'
                );

                if (response.ok) {
                    this.refreshList();
                }
            } catch (error) {
                console.error('Failed to move item:', error);
                await this.queueOfflineAction({
                    type: 'move_item',
                    url: `/items/${itemId}/move`,
                    method: 'POST',
                    headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
                    body: `section_id=${sectionId}`
                });
                this.refreshList();
            }
        },

        async deleteItem() {
            if (!this.mobileActionItem) return;
            if (!this.isOnline) {
                window.Toast.show(t('offline.action_blocked'), 'warning');
                this.mobileActionItem = null;
                return;
            }
            const confirmed = confirm(t('confirm.delete_item', { name: this.mobileActionItem.name }));
            if (!confirmed) return;

            const itemId = this.mobileActionItem.id;

            // Optimistic UI - update section counter and remove item from DOM
            const itemEl = document.getElementById(`item-${itemId}`);
            if (itemEl) {
                window.updateSectionAfterDelete(itemEl);
                itemEl.classList.add('item-exit');
                setTimeout(() => itemEl.remove(), 200);
            }

            this.mobileActionItem = null;
            this.markLocalAction('item_deleted');

            try {
                const response = await this.offlineFetch(
                    `/items/${itemId}`,
                    { method: 'DELETE' },
                    'delete_item'
                );

                if (response.ok) {
                    this.refreshStats();
                }
            } catch (error) {
                console.error('Failed to delete item:', error);
                await this.queueOfflineAction({
                    type: 'delete_item',
                    url: `/items/${itemId}`,
                    method: 'DELETE'
                });
            }

            this.refreshStats();
        },

        // Delete all completed items
        async deleteCompletedItems() {
            if (!this.isOnline) {
                window.Toast.show(t('offline.action_blocked'), 'warning');
                return;
            }

            const confirmed = confirm(t('confirm.delete_completed_items'));
            if (!confirmed) return;

            try {
                const response = await fetch('/items/delete-completed', { method: 'POST' });
                if (response.ok) {
                    const result = await response.json();
                    console.log('[App] Deleted', result.deleted, 'completed items');

                    // Close settings modal
                    this.showSettings = false;

                    // Refresh the list and stats
                    this.refreshList();
                    this.refreshStats();

                    // Show success toast
                    if (result.deleted > 0) {
                        window.Toast.show(t('settings.delete_completed') + ': ' + result.deleted, 'success');
                    }
                } else {
                    window.Toast.show(t('error.delete_items'), 'warning');
                }
            } catch (error) {
                console.error('[App] Failed to delete completed items:', error);
                window.Toast.show(t('error.delete_items'), 'warning');
            }
        },

        // History management methods
        async fetchHistory() {
            if (!this.isOnline) return;

            try {
                const response = await fetch('/api/history');
                if (response.ok) {
                    this.historyItems = await response.json();
                }
            } catch (error) {
                console.error('[App] Failed to fetch history:', error);
            }
        },

        get filteredHistoryItems() {
            if (!this.historySearch) return this.historyItems;
            const search = this.historySearch.toLowerCase();
            return this.historyItems.filter(item =>
                item.name.toLowerCase().includes(search)
            );
        },

        toggleHistoryItem(id) {
            const index = this.selectedHistoryIds.indexOf(id);
            if (index === -1) {
                this.selectedHistoryIds.push(id);
            } else {
                this.selectedHistoryIds.splice(index, 1);
            }
        },

        async deleteHistoryItem(item) {
            if (!this.isOnline) {
                window.Toast.show(t('offline.action_blocked'), 'warning');
                return;
            }

            const confirmed = confirm(t('history.confirm_delete', { name: item.name }));
            if (!confirmed) return;

            try {
                const response = await fetch(`/api/history/${item.id}`, { method: 'DELETE' });
                if (response.ok) {
                    this.historyItems = this.historyItems.filter(h => h.id !== item.id);
                    this.selectedHistoryIds = this.selectedHistoryIds.filter(id => id !== item.id);
                    // Refresh suggestions cache
                    this.cacheSuggestions();
                }
            } catch (error) {
                console.error('[App] Failed to delete history item:', error);
                window.Toast.show(t('error.generic'), 'warning');
            }
        },

        async deleteSelectedHistory() {
            if (!this.isOnline) {
                window.Toast.show(t('offline.action_blocked'), 'warning');
                return;
            }

            if (this.selectedHistoryIds.length === 0) return;

            const confirmed = confirm(t('history.confirm_delete_batch', { count: this.selectedHistoryIds.length }));
            if (!confirmed) return;

            try {
                const response = await fetch('/api/history/batch-delete', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
                    body: `ids=${this.selectedHistoryIds.join(',')}`
                });

                if (response.ok) {
                    const result = await response.json();
                    this.historyItems = this.historyItems.filter(
                        h => !this.selectedHistoryIds.includes(h.id)
                    );
                    this.selectedHistoryIds = [];
                    // Refresh suggestions cache
                    this.cacheSuggestions();
                    window.Toast.show(t('history.deleted', { count: result.deleted }), 'success');
                }
            } catch (error) {
                console.error('[App] Failed to delete history items:', error);
                window.Toast.show(t('error.generic'), 'warning');
            }
        },

        // Auto-completion methods
        async cacheSuggestions() {
            // Cache suggestions for offline use (run in background)
            if (this.isOnline) {
                try {
                    const response = await fetch('/api/suggestions?limit=100');
                    if (response.ok) {
                        const suggestions = await response.json();
                        await window.offlineStorage.saveSuggestions(suggestions);
                    }
                } catch (error) {
                    console.error('[App] Failed to cache suggestions:', error);
                }
            }
        },

        async fetchSuggestions(query) {
            // Clear previous timer
            if (this._suggestionTimer) {
                clearTimeout(this._suggestionTimer);
            }

            // Don't search for very short queries
            if (!query || query.length < 2) {
                this.suggestions = [];
                this.showSuggestions = false;
                return;
            }

            // Debounce 150ms
            this._suggestionTimer = setTimeout(async () => {
                try {
                    if (this.isOnline) {
                        const response = await fetch(`/api/suggestions?q=${encodeURIComponent(query)}&limit=8`);
                        if (response.ok) {
                            this.suggestions = await response.json();
                        }
                    } else {
                        // Offline: search in cached suggestions
                        this.suggestions = await window.offlineStorage.getSuggestions(query);
                    }
                    this.showSuggestions = this.suggestions && this.suggestions.length > 0;
                    this.selectedSuggestionIndex = -1;
                } catch (error) {
                    console.error('[App] Failed to fetch suggestions:', error);
                    this.suggestions = [];
                    this.showSuggestions = false;
                }
            }, 150);
        },

        async selectSuggestion(suggestion, inputRef = null, selectRef = null) {
            // Fill the input with suggestion name
            this.itemNameInput = suggestion.name;

            // Update the input - prefer passed ref, fallback to selectors
            if (inputRef) {
                inputRef.value = suggestion.name;
            } else {
                const desktopInput = document.getElementById('item-name-input');
                if (desktopInput) {
                    desktopInput.value = suggestion.name;
                }
                const mobileInput = this.$refs.itemNameInput;
                if (mobileInput) {
                    mobileInput.value = suggestion.name;
                }
            }

            // Get the target select element
            const targetSelect = selectRef ||
                document.querySelector('#add-item-form select[name="section_id"]') ||
                this.$refs.mobileSectionSelect;

            if (!targetSelect) {
                this.showSuggestions = false;
                this.suggestions = [];
                this.selectedSuggestionIndex = -1;
                return;
            }

            // Auto-select section with smart mapping
            if (suggestion.last_section_id) {
                const options = Array.from(targetSelect.options);

                // 1. Try exact ID match (same section)
                const exactMatch = options.find(opt => opt.value == suggestion.last_section_id);
                if (exactMatch) {
                    targetSelect.value = suggestion.last_section_id;
                    this.showSuggestions = false;
                    this.suggestions = [];
                    this.selectedSuggestionIndex = -1;
                    return;
                }

                // 2. Try matching by section name (for cross-list suggestions)
                if (suggestion.last_section_name) {
                    const nameMatch = options.find(opt =>
                        opt.textContent.toLowerCase().trim() === suggestion.last_section_name.toLowerCase().trim()
                    );
                    if (nameMatch) {
                        targetSelect.value = nameMatch.value;
                        this.showSuggestions = false;
                        this.suggestions = [];
                        this.selectedSuggestionIndex = -1;
                        return;
                    }
                }

                // 3. Section not found - apply user preference
                const mode = this.historySectionMode || 'use_first_section';

                if (mode === 'auto_create_section' && suggestion.last_section_name && this.isOnline) {
                    // Create new section with the same name (only when online)
                    try {
                        const createResponse = await fetch('/sections', {
                            method: 'POST',
                            headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
                            body: `name=${encodeURIComponent(suggestion.last_section_name)}`
                        });

                        if (createResponse.ok) {
                            // Fetch updated sections list to get the new ID
                            const listResponse = await fetch('/sections/list?format=json');
                            if (listResponse.ok) {
                                const sections = await listResponse.json();
                                // Find the newly created section by name
                                const newSection = sections.find(s =>
                                    s.name.toLowerCase().trim() === suggestion.last_section_name.toLowerCase().trim()
                                );

                                if (newSection) {
                                    // Update all section dropdowns on the page
                                    document.querySelectorAll('select[name="section_id"]').forEach(select => {
                                        // Check if option already exists
                                        const exists = Array.from(select.options).some(opt => opt.value == newSection.id);
                                        if (!exists) {
                                            const option = document.createElement('option');
                                            option.value = newSection.id;
                                            option.textContent = newSection.name;
                                            select.appendChild(option);
                                        }
                                    });

                                    // Select the new section in target dropdown
                                    targetSelect.value = newSection.id;

                                    // Also update the main sections list if visible (trigger HTMX refresh)
                                    const sectionsList = document.getElementById('sections-list');
                                    if (sectionsList) {
                                        htmx.trigger(sectionsList, 'refresh');
                                    }
                                }
                            }

                            this.showSuggestions = false;
                            this.suggestions = [];
                            this.selectedSuggestionIndex = -1;
                            return;
                        }
                    } catch (e) {
                        console.error('[App] Failed to create section:', e);
                    }
                }
                // else: use_first_section - keep default selection (first option)
            }

            this.showSuggestions = false;
            this.suggestions = [];
            this.selectedSuggestionIndex = -1;
        },

        handleSuggestionKeydown(event) {
            if (!this.showSuggestions || this.suggestions.length === 0) {
                return;
            }

            switch (event.key) {
                case 'ArrowDown':
                    event.preventDefault();
                    this.selectedSuggestionIndex = Math.min(
                        this.selectedSuggestionIndex + 1,
                        this.suggestions.length - 1
                    );
                    break;
                case 'ArrowUp':
                    event.preventDefault();
                    this.selectedSuggestionIndex = Math.max(this.selectedSuggestionIndex - 1, -1);
                    break;
                case 'Enter':
                    if (this.selectedSuggestionIndex >= 0) {
                        event.preventDefault();
                        this.selectSuggestion(this.suggestions[this.selectedSuggestionIndex]);
                    }
                    break;
                case 'Escape':
                    this.showSuggestions = false;
                    this.selectedSuggestionIndex = -1;
                    break;
            }
        },

        hideSuggestionsDelayed() {
            // Delay to allow click on suggestion
            setTimeout(() => {
                this.showSuggestions = false;
            }, 200);
        },

        setHistorySectionMode(mode) {
            this.historySectionMode = mode;
            localStorage.setItem('history_section_mode', mode);
        },

        // Edit Item
        editItem(item) {
            this.editingItem = item;

            // Check for offline-edited data in DOM attributes
            const itemEl = document.getElementById(`item-${item.id}`);
            if (itemEl) {
                const offlineName = itemEl.getAttribute('data-offline-name');
                const offlineDesc = itemEl.getAttribute('data-offline-description');

                // Use offline data if exists, otherwise use original
                this.editItemName = offlineName !== null ? offlineName : item.name;
                this.editItemDescription = offlineDesc !== null ? offlineDesc : (item.description || '');
            } else {
                this.editItemName = item.name;
                this.editItemDescription = item.description || '';
            }

            this.$nextTick(() => {
                const input = document.querySelector('[x-model="editItemName"]');
                if (input) input.focus();
            });
        },

        async submitEditItem() {
            if (!this.editItemName.trim() || !this.editingItem) return;

            const itemId = this.editingItem.id;
            const name = this.editItemName.trim();
            const description = this.editItemDescription.trim();
            const body = `name=${encodeURIComponent(name)}&description=${encodeURIComponent(description)}`;

            this.editingItem = null;
            this.editItemName = '';
            this.editItemDescription = '';

            // If offline, do optimistic UI update
            if (!this.isOnline) {
                const itemEl = document.getElementById(`item-${itemId}`);
                if (itemEl) {
                    // Update item name in DOM
                    const nameEl = itemEl.querySelector('p.text-sm.text-stone-700, p.text-sm.line-through');
                    if (nameEl) {
                        nameEl.textContent = name;
                    }

                    // Update description if exists
                    const descEl = itemEl.querySelector('p.text-xs.text-stone-400');
                    if (descEl) {
                        if (description) {
                            descEl.textContent = description;
                        } else {
                            descEl.remove();
                        }
                    } else if (description) {
                        // Add description element if it doesn't exist
                        const contentDiv = itemEl.querySelector('.flex-1.min-w-0');
                        if (contentDiv) {
                            const newDescEl = document.createElement('p');
                            newDescEl.className = 'text-xs text-stone-400 truncate mt-0.5';
                            newDescEl.textContent = description;
                            contentDiv.appendChild(newDescEl);
                        }
                    }

                    // Store offline-edited data in DOM attributes for later edit
                    itemEl.setAttribute('data-offline-name', name);
                    itemEl.setAttribute('data-offline-description', description);

                    // Update mobile action button data attributes
                    const mobileBtn = itemEl.querySelector('button[data-item-id]');
                    if (mobileBtn) {
                        mobileBtn.setAttribute('data-item-name', name);
                        mobileBtn.setAttribute('data-item-description', description);
                    }

                    // Add sync indicator if not already present
                    if (!itemEl.classList.contains('pending-sync')) {
                        itemEl.classList.add('pending-sync', 'bg-rose-50/40', 'border-l-2', 'border-rose-400');
                        itemEl.setAttribute('data-pending-sync', 'true');

                        // Add sync badge
                        const existingBadge = itemEl.querySelector('.sync-badge');
                        if (!existingBadge) {
                            const badge = document.createElement('span');
                            badge.className = 'sync-badge text-xs text-rose-500 font-medium flex items-center gap-1';
                            badge.innerHTML = `
                                <svg class="w-3 h-3 animate-spin" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                    <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15"></path>
                                </svg>
                                sync
                            `;
                            // Insert before action buttons or at the end
                            const actionDiv = itemEl.querySelector('.flex.items-center.gap-1');
                            if (actionDiv) {
                                actionDiv.parentNode.insertBefore(badge, actionDiv);
                            } else {
                                itemEl.appendChild(badge);
                            }
                        }
                    }
                }

                // Queue action for sync
                await this.queueOfflineAction({
                    type: 'edit_item',
                    url: `/items/${itemId}`,
                    method: 'PUT',
                    headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
                    body: body
                });
                return;
            }

            try {
                const response = await this.offlineFetch(
                    `/items/${itemId}`,
                    {
                        method: 'PUT',
                        headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
                        body: body
                    },
                    'edit_item'
                );

                if (response.ok) {
                    this.refreshList();
                }
            } catch (error) {
                console.error('Failed to save edit:', error);
                await this.queueOfflineAction({
                    type: 'edit_item',
                    url: `/items/${itemId}`,
                    method: 'PUT',
                    headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
                    body: body
                });
                this.refreshList();
            }
        },

        // Toggle uncertain status with animation (no page refresh)
        async toggleUncertainAnimated(itemId) {
            const item = document.getElementById(`item-${itemId}`);
            if (!item) return;

            // Check current state from DOM (has amber background = uncertain)
            const currentlyUncertain = item.classList.contains('bg-amber-50/50');
            const newState = !currentlyUncertain;

            // Update item background
            if (newState) {
                item.classList.add('bg-amber-50/50');
            } else {
                item.classList.remove('bg-amber-50/50');
            }

            // Update ? icon in content
            const contentDiv = item.querySelector('.flex-1.min-w-0');
            if (contentDiv) {
                const innerDiv = contentDiv.querySelector('.flex.items-center.gap-2');
                if (innerDiv) {
                    const questionMark = innerDiv.querySelector('.text-amber-500.text-xs');
                    if (newState && !questionMark) {
                        const span = document.createElement('span');
                        span.className = 'text-amber-500 text-xs';
                        span.textContent = '?';
                        innerDiv.insertBefore(span, innerDiv.firstChild);
                    } else if (!newState && questionMark) {
                        questionMark.remove();
                    }
                }
            }

            // Update button color and icon fill
            const btn = item.querySelector('.uncertain-btn');
            if (btn) {
                if (newState) {
                    btn.classList.remove('text-stone-400');
                    btn.classList.add('text-amber-500');
                } else {
                    btn.classList.remove('text-amber-500');
                    btn.classList.add('text-stone-400');
                }

                const svg = btn.querySelector('.uncertain-icon');
                if (svg) {
                    svg.setAttribute('fill', newState ? 'currentColor' : 'none');
                }
            }

            // Send request to server in background
            this.markLocalAction('item_updated');
            try {
                const response = await this.offlineFetch(
                    `/items/${itemId}/uncertain`,
                    { method: 'POST' },
                    'toggle_uncertain'
                );

                if (response.ok) {
                    this.refreshStats();
                }
            } catch (error) {
                console.error('Failed to toggle uncertain:', error);
                await this.queueOfflineAction({
                    type: 'toggle_uncertain',
                    url: `/items/${itemId}/uncertain`,
                    method: 'POST'
                });
            }
        },

        // Quick add to section - sets section and focuses input
        quickAddToSection(sectionId) {
            const isMobile = window.innerWidth < 768;

            if (isMobile) {
                this.showAddItem = true;
                this.$nextTick(() => {
                    const mobileSelect = this.$refs.mobileSectionSelect;
                    if (mobileSelect) {
                        mobileSelect.value = sectionId;
                    }
                    setTimeout(() => {
                        const nameInput = this.$refs.itemNameInput;
                        if (nameInput) {
                            nameInput.focus();
                        }
                    }, 350);
                });
            } else {
                const desktopSelect = this.$refs.desktopSectionSelect;
                if (desktopSelect) {
                    desktopSelect.value = sectionId;
                }
                const desktopInput = this.$refs.desktopNameInput;
                if (desktopInput) {
                    desktopInput.focus();
                    desktopInput.scrollIntoView({ behavior: 'smooth', block: 'center' });
                }
            }
        },

        // Drag-and-drop for item reordering
        initMobileSortable() {
            // Check if SortableJS is available
            if (typeof Sortable === 'undefined') {
                console.warn('[App] SortableJS not loaded');
                return;
            }

            // Find all sortable containers (works on both mobile and desktop)
            const containers = document.querySelectorAll('.items-sortable');

            containers.forEach(container => {
                // Skip if already initialized
                if (container._sortableInstance) {
                    container._sortableInstance.destroy();
                }

                const sectionId = container.dataset.sectionId;

                container._sortableInstance = new Sortable(container, {
                    animation: 200,
                    easing: 'cubic-bezier(0.4, 0, 0.2, 1)',
                    handle: '.drag-handle',
                    draggable: '[data-item-id]',
                    ghostClass: 'sortable-ghost',
                    chosenClass: 'sortable-chosen',
                    dragClass: 'sortable-drag',

                    // Allow dragging between sections
                    group: 'items',

                    // Threshold before starting drag (prevents accidental drags)
                    delay: 150,
                    delayOnTouchOnly: true,
                    touchStartThreshold: 5,

                    // Scroll behavior - scroll window when near edges
                    scroll: true,
                    scrollSensitivity: 150,
                    scrollSpeed: 15,
                    bubbleScroll: true,
                    forceFallback: true,
                    fallbackOnBody: true,
                    fallbackTolerance: 3,

                    // Callbacks
                    onStart: (evt) => {
                        // Haptic feedback if available
                        if (navigator.vibrate) {
                            navigator.vibrate(10);
                        }
                        // Prevent body scroll during drag
                        document.body.classList.add('is-dragging');
                    },

                    onEnd: async (evt) => {
                        document.body.classList.remove('is-dragging');

                        const itemId = evt.item.dataset.itemId;
                        const fromSectionId = evt.from.dataset.sectionId;
                        const toSectionId = evt.to.dataset.sectionId;
                        const newIndex = evt.newIndex;
                        const oldIndex = evt.oldIndex;

                        // Moving to different section
                        if (fromSectionId !== toSectionId) {
                            // Pass target position directly - server handles positioning among active items
                            await this.moveItemToSection(itemId, toSectionId, newIndex);
                            return;
                        }

                        // Same section - reorder
                        if (newIndex !== oldIndex) {
                            await this.syncItemPosition(itemId, toSectionId, newIndex);
                        }
                    }
                });
            });
        },

        // Sync item position with server - single request (supports offline)
        async syncItemPosition(itemId, sectionId, newIndex) {
            // Mark as local action to prevent WebSocket race condition
            this.markLocalAction('items_reordered');

            try {
                const url = `/items/${itemId}/move`;
                const options = {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
                    body: `section_id=${encodeURIComponent(sectionId)}&position=${encodeURIComponent(newIndex)}`
                };

                const response = await this.offlineFetch(url, options, 'reorder_item');

                if (response.offline) {
                    // Queued for later - silent sync, just haptic feedback
                    if (navigator.vibrate) navigator.vibrate([10, 50, 10]);
                    return;
                }

                if (!response.ok) {
                    console.error('Failed to reorder item');
                    window.Toast?.show(window.t?.('errors.reorder_failed') || 'Failed to reorder item', 'error');
                    if (navigator.onLine) this.refreshList();
                    return;
                }

                // Haptic feedback on success
                if (navigator.vibrate) {
                    navigator.vibrate([10, 50, 10]);
                }
            } catch (error) {
                console.error('Failed to sync item position:', error);
                window.Toast?.show(window.t?.('errors.reorder_failed') || 'Failed to reorder item', 'error');
                if (navigator.onLine) this.refreshList();
            }
        },

        // Move item to different section via drag-and-drop (supports offline)
        async moveItemToSection(itemId, sectionId, targetIndex) {
            this.markLocalAction('item_moved');

            try {
                const url = `/items/${itemId}/move`;
                const options = {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
                    body: `section_id=${encodeURIComponent(sectionId)}&position=${encodeURIComponent(targetIndex)}`
                };

                const response = await this.offlineFetch(url, options, 'move_item');

                if (response.offline) {
                    // Queued for later - silent sync, just haptic feedback
                    if (navigator.vibrate) navigator.vibrate([10, 50, 10]);
                    return;
                }

                if (!response.ok) {
                    console.error('Failed to move item to section');
                    window.Toast?.show(window.t?.('errors.move_failed') || 'Failed to move item', 'error');
                    if (navigator.onLine) this.refreshList();
                    return;
                }

                // Haptic feedback on success
                if (navigator.vibrate) {
                    navigator.vibrate([10, 50, 10]);
                }

                // Refresh to ensure sync
                if (navigator.onLine) this.refreshList();
            } catch (error) {
                console.error('Failed to move item to section:', error);
                window.Toast?.show(window.t?.('errors.move_failed') || 'Failed to move item', 'error');
                if (navigator.onLine) this.refreshList();
            }
        },

        // Import/Export functions
        async handleImportFile(file) {
            if (!file) return;

            // Validate file size (5MB max)
            if (file.size > 5 * 1024 * 1024) {
                if (window.Toast) {
                    window.Toast.show(t('import.error_file_too_large'), 'warning');
                }
                return;
            }

            // Validate file type
            const validTypes = ['application/json', 'text/csv'];
            const validExtensions = ['.json', '.csv'];
            const hasValidType = validTypes.includes(file.type) || validExtensions.some(ext => file.name.toLowerCase().endsWith(ext));

            if (!hasValidType) {
                if (window.Toast) {
                    window.Toast.show(t('import.error_invalid_format'), 'warning');
                }
                return;
            }

            this.importFile = file;

            // Send file for preview
            const formData = new FormData();
            formData.append('file', file);

            try {
                const response = await fetch('/import/preview', {
                    method: 'POST',
                    body: formData
                });

                const preview = await response.json();

                if (!preview.valid) {
                    if (window.Toast) {
                        window.Toast.show(preview.error || t('import.error_invalid_file'), 'warning');
                    }
                    return;
                }

                this.importPreview = preview;
                this.importConflictResolution = 'skip';
                this.showSettings = false;
                this.showImportPreview = true;
            } catch (error) {
                console.error('Failed to preview import:', error);
                if (window.Toast) {
                    window.Toast.show(t('import.error_preview'), 'warning');
                }
            }
        },

        async confirmImport() {
            if (!this.importFile) return;

            const formData = new FormData();
            formData.append('file', this.importFile);
            formData.append('conflict_resolution', this.importConflictResolution);
            formData.append('copy_suffix', this.t('import.copy_suffix'));

            try {
                const response = await fetch('/import', {
                    method: 'POST',
                    body: formData
                });

                const result = await response.json();

                if (result.success) {
                    this.showImportPreview = false;
                    if (window.Toast) {
                        window.Toast.show(t('import.success'), 'success');
                    }
                    // Reload to show imported data
                    window.location.reload();
                } else {
                    if (window.Toast) {
                        window.Toast.show(result.error || t('import.error'), 'warning');
                    }
                }
            } catch (error) {
                console.error('Failed to import:', error);
                if (window.Toast) {
                    window.Toast.show(t('import.error'), 'warning');
                }
            }
        }
    };
}

// HTMX configuration
document.addEventListener('DOMContentLoaded', function() {
    htmx.config.defaultSwapStyle = 'outerHTML';
    htmx.config.globalViewTransitions = true;

    // Track existing items before swap to animate only new ones
    let existingItemIds = new Set();

    // Counter for temporary offline IDs
    let offlineItemCounter = Date.now();

    // Intercept HTMX requests when offline
    document.body.addEventListener('htmx:beforeRequest', function(event) {
        if (navigator.onLine) return; // Online - let HTMX handle it

        const path = event.detail.requestConfig?.path || '';
        const verb = event.detail.requestConfig?.verb?.toUpperCase() || 'GET';

        // Handle POST /items (add item) offline
        if (verb === 'POST' && path === '/items') {
            event.preventDefault();

            const form = event.detail.elt;
            const formData = new FormData(form);
            const sectionId = formData.get('section_id');
            const name = formData.get('name');
            const description = formData.get('description') || '';

            if (!sectionId || !name) return;

            // Generate temporary ID
            const tempId = 'offline-' + (++offlineItemCounter);

            // Create optimistic item HTML
            const itemHtml = createOfflineItemHtml(tempId, name, description, sectionId);

            // Find the section by exact ID and add item to it
            const sectionEl = document.getElementById(`section-${sectionId}`);
            if (sectionEl) {
                // Show section if it was hidden (empty section)
                sectionEl.classList.remove('hidden');

                // Find the active items container (not completed items)
                const itemsContainer = sectionEl.querySelector('.active-items');
                if (itemsContainer) {
                    // Insert at the beginning (newest first based on sort_order)
                    itemsContainer.insertAdjacentHTML('afterbegin', itemHtml);

                    // Add animation
                    const newItem = document.getElementById(`item-${tempId}`);
                    if (newItem) {
                        newItem.classList.add('item-enter');
                        setTimeout(() => newItem.classList.remove('item-enter'), 300);
                    }

                    // Update section counter
                    const counter = sectionEl.querySelector('.section-counter');
                    if (counter) {
                        const text = counter.textContent;
                        const match = text.match(/(\d+)\/(\d+)/);
                        if (match) {
                            const completed = parseInt(match[1]);
                            const total = parseInt(match[2]) + 1;
                            counter.textContent = `${completed}/${total}`;
                        } else {
                            counter.textContent = '0/1';
                        }
                    }
                }
            } else {
                console.warn('[Offline] Section not found in DOM:', sectionId);
            }

            // Queue action for sync
            window.offlineStorage.queueAction({
                type: 'create_item',
                url: '/items',
                method: 'POST',
                headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
                body: `section_id=${sectionId}&name=${encodeURIComponent(name)}&description=${encodeURIComponent(description)}`,
                tempId: tempId
            }).then(() => {
                console.log('[Offline] Item queued:', name);
            });

            // Update stats and handle modal
            const alpineData = Alpine.$data(document.querySelector('[x-data="shoppingList()"]'));
            if (alpineData) {
                alpineData.stats.total++;
                // Close mobile add item modal if open (unless addMore is enabled)
                if (!alpineData.addMore) {
                    form.reset();
                    alpineData.showAddItem = false;
                } else {
                    // Keep modal open, clear only name and description
                    form.querySelector('[name=name]').value = '';
                    form.querySelector('[name=description]').value = '';
                    setTimeout(() => {
                        const nameInput = form.querySelector('[name=name]');
                        if (nameInput) nameInput.focus();
                    }, 100);
                }
            } else {
                // Fallback if no Alpine data
                form.reset();
            }

            return false;
        }

        // Handle POST /items/:id/toggle offline
        if (verb === 'POST' && path.match(/\/items\/\d+\/toggle/)) {
            event.preventDefault();

            const itemId = path.match(/\/items\/(\d+)\/toggle/)[1];
            const itemEl = document.getElementById(`item-${itemId}`);

            if (itemEl) {
                // Check if item is currently completed (has pink checkbox bg)
                const checkboxSpan = itemEl.querySelector('button > span');
                const isCompleted = checkboxSpan && checkboxSpan.classList.contains('bg-pink-400');

                // Add pending sync styling
                itemEl.classList.add('pending-sync');
                itemEl.dataset.pendingSync = 'true';

                // Toggle visual state
                if (isCompleted) {
                    // Uncomplete: change from pink checkbox to empty border
                    if (checkboxSpan) {
                        checkboxSpan.classList.remove('bg-pink-400', 'flex', 'items-center', 'justify-center');
                        checkboxSpan.classList.add('border-2', 'border-stone-300', 'hover:border-pink-400', 'hover:scale-110');
                        checkboxSpan.innerHTML = '';
                    }
                    // Change text style
                    const textEl = itemEl.querySelector('.line-through');
                    if (textEl) {
                        textEl.classList.remove('line-through', 'text-stone-400', 'text-stone-300');
                        textEl.classList.add('text-stone-700');
                    }
                    // Update stats
                    const alpineData = Alpine.$data(document.querySelector('[x-data="shoppingList()"]'));
                    if (alpineData) {
                        alpineData.stats.completed = Math.max(0, alpineData.stats.completed - 1);
                        alpineData.stats.percentage = Math.round((alpineData.stats.completed / alpineData.stats.total) * 100) || 0;
                    }
                } else {
                    // Complete: change from empty border to pink checkbox
                    if (checkboxSpan) {
                        checkboxSpan.classList.remove('border-2', 'border-stone-300', 'hover:border-pink-400', 'hover:scale-110');
                        checkboxSpan.classList.add('bg-pink-400', 'flex', 'items-center', 'justify-center');
                        checkboxSpan.innerHTML = '<svg class="w-3 h-3 text-white" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="3" d="M5 13l4 4L19 7"></path></svg>';
                    }
                    // Change text style
                    const textEl = itemEl.querySelector('.text-stone-700');
                    if (textEl) {
                        textEl.classList.remove('text-stone-700');
                        textEl.classList.add('line-through', 'text-stone-400');
                    }
                    // Update stats
                    const alpineData = Alpine.$data(document.querySelector('[x-data="shoppingList()"]'));
                    if (alpineData) {
                        alpineData.stats.completed++;
                        alpineData.stats.percentage = Math.round((alpineData.stats.completed / alpineData.stats.total) * 100) || 0;
                    }
                }

                // Add visual pending sync indicator (rose border + badge)
                itemEl.classList.add('bg-rose-50/40', 'border-l-2', 'border-rose-400');

                // Add sync badge if not already present
                if (!itemEl.querySelector('.offline-sync-badge')) {
                    const badge = document.createElement('span');
                    badge.className = 'offline-sync-badge inline-flex items-center gap-1 px-2 py-0.5 rounded-full text-xs font-medium bg-rose-100 text-rose-600 ml-2';
                    badge.innerHTML = `
                        <svg class="w-3 h-3 animate-spin" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15"></path>
                        </svg>
                        sync
                    `;
                    // Insert badge after content div
                    const contentDiv = itemEl.querySelector('.flex-1.min-w-0');
                    if (contentDiv) {
                        contentDiv.after(badge);
                    }
                }

                // Animate checkbox
                if (checkboxSpan) {
                    checkboxSpan.classList.add('checkbox-pulse');
                    setTimeout(() => checkboxSpan.classList.remove('checkbox-pulse'), 300);
                }
            }

            // Queue for sync
            window.offlineStorage.queueAction({
                type: 'toggle_item',
                url: path,
                method: 'POST'
            });

            console.log('[Offline] Toggle queued:', itemId);
            return false;
        }

        // Block DELETE /items/:id offline - show toast
        if (verb === 'DELETE' && path.match(/\/items\/\d+$/)) {
            event.preventDefault();
            window.Toast.show(t('offline.action_blocked'), 'warning');
            return false;
        }

        // Handle POST /items/:id/uncertain offline with optimistic UI
        if (verb === 'POST' && path.match(/\/items\/\d+\/uncertain/)) {
            event.preventDefault();

            const itemId = path.match(/\/items\/(\d+)\/uncertain/)[1];
            const itemEl = document.getElementById(`item-${itemId}`);

            if (itemEl) {
                // Check current state
                const currentlyUncertain = itemEl.classList.contains('bg-amber-50/50');
                const newState = !currentlyUncertain;

                // Toggle uncertain styling
                if (newState) {
                    itemEl.classList.add('bg-amber-50/50');
                } else {
                    itemEl.classList.remove('bg-amber-50/50');
                }

                // Update ? icon
                const contentDiv = itemEl.querySelector('.flex-1.min-w-0');
                if (contentDiv) {
                    const innerDiv = contentDiv.querySelector('.flex.items-center.gap-2');
                    if (innerDiv) {
                        const questionMark = innerDiv.querySelector('.text-amber-500.text-xs:not(.offline-sync-badge)');
                        if (newState && !questionMark) {
                            const span = document.createElement('span');
                            span.className = 'text-amber-500 text-xs';
                            span.textContent = '?';
                            innerDiv.insertBefore(span, innerDiv.firstChild);
                        } else if (!newState && questionMark) {
                            questionMark.remove();
                        }
                    }
                }

                // Update button styling
                const btn = itemEl.querySelector('.uncertain-btn');
                if (btn) {
                    if (newState) {
                        btn.classList.remove('text-stone-400');
                        btn.classList.add('text-amber-500');
                    } else {
                        btn.classList.remove('text-amber-500');
                        btn.classList.add('text-stone-400');
                    }
                    const svg = btn.querySelector('.uncertain-icon');
                    if (svg) {
                        svg.setAttribute('fill', newState ? 'currentColor' : 'none');
                    }
                }

                // Add sync indicator
                itemEl.classList.add('border-l-2', 'border-rose-400');
                if (!itemEl.querySelector('.offline-sync-badge')) {
                    const badge = document.createElement('span');
                    badge.className = 'offline-sync-badge inline-flex items-center gap-1 px-2 py-0.5 rounded-full text-xs font-medium bg-rose-100 text-rose-600 ml-2';
                    badge.innerHTML = `<svg class="w-3 h-3 animate-spin" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15"></path></svg>${t('status.syncing')}`;
                    if (contentDiv) {
                        contentDiv.after(badge);
                    }
                }
            }

            // Queue for sync
            window.offlineStorage.queueAction({
                type: 'toggle_uncertain',
                url: path,
                method: 'POST'
            });

            console.log('[Offline] Uncertain toggle queued:', itemId);
            return false;
        }

        // Block POST /items/:id/move offline - show toast
        if (verb === 'POST' && path.match(/\/items\/\d+\/move$/)) {
            event.preventDefault();
            window.Toast.show(t('offline.action_blocked'), 'warning');
            return false;
        }

        // Block POST /items/:id/move-up and move-down offline - show toast
        if (verb === 'POST' && path.match(/\/items\/\d+\/move-(up|down)/)) {
            event.preventDefault();
            window.Toast.show(t('offline.action_blocked'), 'warning');
            return false;
        }

        // Block section operations offline - show toast
        if (path.startsWith('/sections')) {
            event.preventDefault();
            window.Toast.show(t('offline.action_blocked'), 'warning');
            return false;
        }
    });

    document.body.addEventListener('htmx:responseError', function(event) {
        console.error('HTMX error:', event.detail);
        if (event.detail.xhr.status === 401) {
            window.location.href = '/login';
        }
    });

    document.body.addEventListener('htmx:beforeSwap', function(event) {
        const redirectUrl = event.detail.xhr.getResponseHeader('HX-Redirect');
        if (redirectUrl) {
            window.location.href = redirectUrl;
            event.detail.shouldSwap = false;
        }

        // Capture existing item IDs before swap
        if (event.detail.target?.id === 'sections-list') {
            existingItemIds = new Set(
                [...document.querySelectorAll('[id^="item-"]')].map(el => el.id)
            );
        }
    });

    document.body.addEventListener('htmx:afterSwap', function(event) {
        // Restore correct page title after swap (prevents HTMX from resetting to hardcoded title)
        document.title = window.translations[window.currentLang]?.list?.title || 'Koffan Shopping List';

        // Animate only new items after swap
        if (event.detail.target?.id === 'sections-list') {
            document.querySelectorAll('[id^="item-"]').forEach(el => {
                if (!existingItemIds.has(el.id)) {
                    el.classList.add('item-enter');
                    // Remove animation class after it completes
                    setTimeout(() => el.classList.remove('item-enter'), 300);
                }
            });
            existingItemIds.clear();
        }
    });

    // Also restore title after any HTMX request completes (belt and suspenders approach)
    document.body.addEventListener('htmx:afterRequest', function(event) {
        document.title = window.translations[window.currentLang]?.list?.title || 'Koffan Shopping List';
    });

});

// Create HTML for offline item (simplified version without all actions)
function createOfflineItemHtml(id, name, description, sectionId) {
    const descHtml = description
        ? `<p class="text-xs text-stone-400 dark:text-stone-500 truncate mt-0.5">${escapeHtml(description)}</p>`
        : '';

    return `
<div id="item-${id}" class="px-4 py-3 flex items-center gap-3 hover:bg-stone-50 dark:hover:bg-stone-700 transition-all group bg-rose-50/40 dark:bg-rose-900/30 border-l-2 border-rose-400 pending-sync" data-pending-sync="true">
    <!-- Checkbox (disabled offline) -->
    <div class="flex-shrink-0 w-5 h-5 rounded-full border-2 border-stone-200 dark:border-stone-600 bg-stone-50 dark:bg-stone-700"></div>

    <!-- Content -->
    <div class="flex-1 min-w-0">
        <p class="text-sm text-stone-700 dark:text-stone-200 truncate">${escapeHtml(name)}</p>
        ${descHtml}
    </div>

    <!-- Sync badge -->
    <span class="sync-badge text-xs text-rose-500 dark:text-rose-400 font-medium flex items-center gap-1">
        <svg class="w-3 h-3 animate-spin" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15"></path>
        </svg>
        sync
    </span>
</div>`;
}

// Escape HTML to prevent XSS
function escapeHtml(text) {
    const div = document.createElement('div');
    div.textContent = text;
    return div.innerHTML;
}

// Update section counter and visibility after item deletion
window.updateSectionAfterDelete = function(itemElement) {
    // Find parent section
    const section = itemElement.closest('[data-section-id]');
    if (!section) return;

    // Count remaining items (active + completed)
    const activeItems = section.querySelectorAll('.active-items > [id^="item-"]').length;
    const completedContainer = section.querySelector('.completed-items');
    const completedItems = completedContainer ? completedContainer.querySelectorAll('[id^="item-"]').length : 0;

    // Subtract 1 for the item being deleted (it's still in DOM at this point)
    const isActiveItem = itemElement.closest('.active-items') !== null;
    const newActiveCount = isActiveItem ? activeItems - 1 : activeItems;
    const newCompletedCount = isActiveItem ? completedItems : completedItems - 1;
    const newTotal = newActiveCount + newCompletedCount;

    // Update counter
    const counter = section.querySelector('.section-counter');
    if (counter) {
        counter.textContent = `${newCompletedCount}/${newTotal}`;
    }

    // Hide section if empty
    if (newTotal === 0) {
        section.classList.add('hidden');
    }
};

// Get completed section state from localStorage (safe for iOS Safari)
window.getCompletedSectionState = function(sectionId) {
    try {
        const stored = localStorage.getItem('completedSections');
        if (!stored) return false;
        const data = JSON.parse(stored);
        return data['section-' + sectionId] || false;
    } catch(e) {
        return false;
    }
};

// Global function for uncertain toggle (called from onclick)
window.toggleUncertain = async function(itemId) {
    const item = document.getElementById(`item-${itemId}`);
    if (!item) return;

    // Check current state from DOM
    const currentlyUncertain = item.classList.contains('bg-amber-50/50');
    const newState = !currentlyUncertain;

    // Animate the button
    const btn = item.querySelector('.uncertain-btn');
    if (btn) {
        btn.classList.add('checkbox-pulse');
        setTimeout(() => btn.classList.remove('checkbox-pulse'), 300);
    }

    // Update item background with smooth transition
    if (newState) {
        item.classList.add('bg-amber-50/50');
    } else {
        item.classList.remove('bg-amber-50/50');
    }

    // Update ? icon in content
    const contentDiv = item.querySelector('.flex-1.min-w-0');
    if (contentDiv) {
        const innerDiv = contentDiv.querySelector('.flex.items-center.gap-2');
        if (innerDiv) {
            const questionMark = innerDiv.querySelector('.text-amber-500.text-xs');
            if (newState && !questionMark) {
                const span = document.createElement('span');
                span.className = 'text-amber-500 text-xs';
                span.textContent = '?';
                innerDiv.insertBefore(span, innerDiv.firstChild);
            } else if (!newState && questionMark) {
                questionMark.remove();
            }
        }
    }

    // Update button color and icon fill
    if (btn) {
        if (newState) {
            btn.classList.remove('text-stone-400');
            btn.classList.add('text-amber-500');
        } else {
            btn.classList.remove('text-amber-500');
            btn.classList.add('text-stone-400');
        }

        const svg = btn.querySelector('.uncertain-icon');
        if (svg) {
            svg.setAttribute('fill', newState ? 'currentColor' : 'none');
        }
    }

    // Send request to server in background
    try {
        if (navigator.onLine) {
            await fetch(`/items/${itemId}/uncertain`, { method: 'POST' });
        } else {
            // Queue for offline sync
            await window.offlineStorage.queueAction({
                type: 'toggle_uncertain',
                url: `/items/${itemId}/uncertain`,
                method: 'POST'
            });
        }
    } catch (error) {
        console.error('Failed to toggle uncertain:', error);
        // Queue for offline sync on error
        try {
            await window.offlineStorage.queueAction({
                type: 'toggle_uncertain',
                url: `/items/${itemId}/uncertain`,
                method: 'POST'
            });
        } catch (e) {
            console.error('Failed to queue action:', e);
        }
    }
};

// Pull to refresh
(function() {
    const spinner = document.getElementById('pull-to-refresh');
    if (!spinner) return;

    const svg = spinner.querySelector('svg');
    const threshold = 70; // px to trigger refresh
    const maxPull = 140; // max pull distance

    let startY = 0;
    let currentY = 0;
    let isPulling = false;
    let isRefreshing = false;

    function canPull() {
        // Only pull when at top of page and no modal is open
        return window.scrollY === 0 &&
               !document.querySelector('[x-show="showAddItem"]:not([style*="display: none"])') &&
               !document.querySelector('[x-show="showManageSections"]:not([style*="display: none"])') &&
               !document.querySelector('[x-show="showSettings"]:not([style*="display: none"])');
    }

    document.addEventListener('touchstart', (e) => {
        if (isRefreshing || !canPull()) return;
        startY = e.touches[0].pageY;
        isPulling = false;
    }, { passive: true });

    document.addEventListener('touchmove', (e) => {
        if (isRefreshing || startY === 0) return;

        currentY = e.touches[0].pageY;
        const pullDistance = currentY - startY;

        // Only activate if pulling down from top
        if (pullDistance > 0 && window.scrollY === 0) {
            isPulling = true;

            // Calculate position with resistance (follows finger with dampening)
            const pullWithResistance = Math.min(pullDistance * 0.5, maxPull);

            // Position spinner to follow finger (offset a bit above finger)
            const spinnerY = startY + pullWithResistance - 50;
            spinner.style.top = Math.max(10, spinnerY) + 'px';
            spinner.classList.add('visible');

            // Rotate icon based on pull progress
            const rotation = (pullWithResistance / threshold) * 360;
            svg.style.transform = `rotate(${rotation}deg)`;
        }
    }, { passive: true });

    document.addEventListener('touchend', () => {
        if (!isPulling) {
            startY = 0;
            return;
        }

        const pullDistance = currentY - startY;
        const pullWithResistance = pullDistance * 0.5;

        if (pullWithResistance >= threshold && !isRefreshing) {
            // Trigger refresh - keep spinner visible at current position
            isRefreshing = true;
            spinner.classList.add('refreshing');

            doRefresh().finally(() => {
                isRefreshing = false;
                spinner.classList.remove('refreshing', 'visible');
                spinner.style.top = '0';
            });
        } else {
            // Cancel - hide spinner
            spinner.classList.remove('visible');
            spinner.style.top = '0';
        }

        startY = 0;
        currentY = 0;
        isPulling = false;
    }, { passive: true });

    async function doRefresh() {
        // Get Alpine component and call fullRefresh
        const appEl = document.querySelector('[x-data="shoppingList()"]');
        if (appEl && window.Alpine) {
            const data = Alpine.$data(appEl);
            if (data && data.fullRefresh) {
                await data.fullRefresh();
            }
        } else {
            // Fallback - just reload the page
            window.location.reload();
        }

        // Minimum spinner time for UX
        await new Promise(r => setTimeout(r, 400));
    }
})();
