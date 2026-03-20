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

// Check and update all empty states (no sections / no products) and add form visibility
window.checkEmptyStates = function() {
    const sl = document.getElementById('sections-list');
    if (!sl) return;
    const hasSections = sl.querySelector('[data-section-id]') !== null;
    const hasItems = sl.querySelector('[id^="item-"]') !== null;
    const daf = document.getElementById('desktop-add-form');
    const mab = document.getElementById('mobile-add-item-btn');

    if (!hasSections) {
        // No sections: show "No sections", hide "No products", hide add form
        document.getElementById('empty-no-products')?.remove();
        if (!document.getElementById('empty-no-sections')) {
            sl.insertAdjacentHTML('beforeend',
                '<div id="empty-no-sections" class="text-center py-20">' +
                '<div class="w-16 h-16 mx-auto mb-4 bg-stone-100 dark:bg-stone-800 rounded-2xl flex items-center justify-center">' +
                '<svg class="w-8 h-8 text-stone-400 dark:text-stone-500" fill="none" stroke="currentColor" viewBox="0 0 24 24">' +
                '<path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M9 5H7a2 2 0 00-2 2v12a2 2 0 002 2h10a2 2 0 002-2V7a2 2 0 00-2-2h-2M9 5a2 2 0 002 2h2a2 2 0 002-2M9 5a2 2 0 012-2h2a2 2 0 012 2"></path>' +
                '</svg></div>' +
                '<p class="text-stone-600 dark:text-stone-300 font-medium" x-text="t(\'sections.no_sections\')"></p>' +
                '<p class="text-sm text-stone-400 dark:text-stone-500 mt-1" x-text="t(\'sections.add_first_section\')"></p>' +
                '<button @click="showManageSections = true" class="mt-4 bg-pink-400 hover:bg-pink-500 text-white px-5 py-2.5 rounded-lg text-sm font-medium transition-colors" x-text="t(\'sections.add_section_btn\')"></button>' +
                '</div>'
            );
            Alpine.initTree(document.getElementById('empty-no-sections'));
        }
        if (daf) daf.style.display = 'none';
        if (mab) mab.style.display = 'none';
    } else if (!hasItems) {
        // Sections exist but no items: show "No products", hide "No sections", show add form
        document.getElementById('empty-no-sections')?.remove();
        if (!document.getElementById('empty-no-products')) {
            sl.insertAdjacentHTML('beforeend',
                '<div id="empty-no-products" class="text-center py-20">' +
                '<div class="w-16 h-16 mx-auto mb-4 bg-stone-100 dark:bg-stone-800 rounded-2xl flex items-center justify-center">' +
                '<svg class="w-8 h-8 text-stone-400 dark:text-stone-500" fill="none" stroke="currentColor" viewBox="0 0 24 24">' +
                '<path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M16 11V7a4 4 0 00-8 0v4M5 9h14l1 12H4L5 9z"></path>' +
                '</svg></div>' +
                '<p class="text-stone-600 dark:text-stone-300 font-medium" x-text="t(\'items.no_items\')"></p>' +
                '<p class="text-sm text-stone-400 dark:text-stone-500 mt-1" x-text="t(\'items.add_first_item\')"></p>' +
                '</div>'
            );
            Alpine.initTree(document.getElementById('empty-no-products'));
        }
        if (daf) daf.style.removeProperty('display');
        if (mab) mab.style.removeProperty('display');
    } else {
        // Has sections and items: remove all empty states, show add form
        document.getElementById('empty-no-sections')?.remove();
        document.getElementById('empty-no-products')?.remove();
        if (daf) daf.style.removeProperty('display');
        if (mab) mab.style.removeProperty('display');
    }
};

// Fuzzy Search Utilities
function normalizePolish(str) {
    const map = {
        'ą': 'a', 'ć': 'c', 'ę': 'e', 'ł': 'l', 'ń': 'n',
        'ó': 'o', 'ś': 's', 'ź': 'z', 'ż': 'z',
        'Ą': 'A', 'Ć': 'C', 'Ę': 'E', 'Ł': 'L', 'Ń': 'N',
        'Ó': 'O', 'Ś': 'S', 'Ź': 'Z', 'Ż': 'Z'
    };
    return str.replace(/[ąćęłńóśźżĄĆĘŁŃÓŚŹŻ]/g, c => map[c] || c);
}

function jaroSimilarity(s1, s2) {
    if (s1 === s2) return 1.0;
    if (s1.length === 0 || s2.length === 0) return 0.0;

    const matchWindow = Math.floor(Math.max(s1.length, s2.length) / 2) - 1;
    const s1Matches = new Array(s1.length).fill(false);
    const s2Matches = new Array(s2.length).fill(false);

    let matches = 0;
    let transpositions = 0;

    for (let i = 0; i < s1.length; i++) {
        const start = Math.max(0, i - matchWindow);
        const end = Math.min(i + matchWindow + 1, s2.length);

        for (let j = start; j < end; j++) {
            if (s2Matches[j] || s1[i] !== s2[j]) continue;
            s1Matches[i] = true;
            s2Matches[j] = true;
            matches++;
            break;
        }
    }

    if (matches === 0) return 0.0;

    let k = 0;
    for (let i = 0; i < s1.length; i++) {
        if (!s1Matches[i]) continue;
        while (!s2Matches[k]) k++;
        if (s1[i] !== s2[k]) transpositions++;
        k++;
    }

    return (matches / s1.length + matches / s2.length + (matches - transpositions / 2) / matches) / 3;
}

function jaroWinklerSimilarity(s1, s2) {
    const jaro = jaroSimilarity(s1, s2);
    let prefixLength = 0;
    const maxPrefix = Math.min(4, s1.length, s2.length);

    for (let i = 0; i < maxPrefix; i++) {
        if (s1[i] === s2[i]) prefixLength++;
        else break;
    }

    return jaro + prefixLength * 0.1 * (1 - jaro);
}

function fuzzyMatchScore(query, text) {
    const normQuery = normalizePolish(query.toLowerCase());
    const normText = normalizePolish(text.toLowerCase());

    // Exact substring match
    if (normText.includes(normQuery)) {
        const startBonus = normText.startsWith(normQuery) ? 0.1 : 0;
        return 0.9 + startBonus;
    }

    // Word-level match
    const words = normText.split(/\s+/);
    let bestWordScore = 0;
    for (const word of words) {
        const score = jaroWinklerSimilarity(normQuery, word);
        if (score > bestWordScore) bestWordScore = score;
    }

    // Full text match
    const fullScore = jaroWinklerSimilarity(normQuery, normText);

    return Math.max(bestWordScore, fullScore);
}

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
        showListSwitcher: false,

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
        editItemQuantity: 0,

        // Auto-completion
        suggestions: [],
        showSuggestions: false,
        selectedSuggestionIndex: -1,
        itemNameInput: '',
        _suggestionTimer: null,

        // Quick Add inline
        quickAddSectionId: null,
        quickAddName: '',
        quickAddSuggestions: [],
        showQuickAddSuggestions: false,
        selectedQuickAddSuggestionIndex: -1,
        _quickAddSuggestionTimer: null,

        // Search
        searchQuery: '',
        searchResults: [],

        // Track pending local actions to avoid WebSocket race conditions
        pendingLocalActions: {},
        localActionTimeout: 1000, // ms to ignore WebSocket updates after local action

        // Debounce timers for refresh
        _refreshListTimer: null,
        _refreshStatsTimer: null,
        _isRefreshing: false,
        _suppressOverlayUntil: 0, // Timestamp until which overlay should be suppressed
        _fullRefreshInProgress: false, // Flag to suppress WS-driven refreshes during full refresh

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

            // Keyboard shortcut for save (Cmd+Enter)
            document.addEventListener('keydown', (e) => {
                if ((e.metaKey || e.ctrlKey) && e.key === 'Enter' && this.editingItem) {
                    e.preventDefault();
                    this.submitEditItem();
                }
            });

            // Keyboard shortcut for search (Ctrl+F / Cmd+F)
            document.addEventListener('keydown', (e) => {
                if (!((e.ctrlKey || e.metaKey) && e.key === 'f')) return;
                if (window.innerWidth <= 768) return;
                if (this.stats.total <= 5) return;

                const activeEl = document.activeElement;
                if (activeEl === this.$refs.searchInput) return;

                e.preventDefault();
                this.$refs.searchInput?.focus();
                this.$refs.searchInput?.select();
            });

            // Initialize mobile drag-and-drop
            this.$nextTick(() => {
                this.initMobileSortable();
            });

            // Re-initialize sortable after any remaining HTMX swaps (e.g. section rename)
            // NOTE: Alpine.initTree is NOT called here - Alpine's MutationObserver
            // auto-initializes new elements. Calling initTree manually causes duplicate
            // event handlers (e.g. confirm() dialog appearing multiple times).
            const _sortableInitTimers = {};
            document.body.addEventListener('htmx:afterSwap', (e) => {
                const target = e.detail.target;
                const targetId = target?.id || '';

                if (target && targetId) {
                    const isSection = targetId.startsWith('section-') || targetId === 'sections-list';
                    if (isSection) {
                        clearTimeout(_sortableInitTimers[targetId]);
                        _sortableInitTimers[targetId] = setTimeout(() => {
                            delete _sortableInitTimers[targetId];
                            const el = document.getElementById(targetId);
                            if (el) {
                                const container = el.querySelector('.items-sortable');
                                if (container) this.initSortableForContainer(container);
                            }
                        }, 50);
                    }
                }
            });

            // Listen for refresh-sections events (fallback)
            window.addEventListener('refresh-sections', (e) => {
                const fromSectionId = e.detail.fromSectionId;
                const toSectionId = e.detail.toSectionId;
                const fromSection = document.getElementById(`section-${fromSectionId}`);
                const toSection = document.getElementById(`section-${toSectionId}`);
                if (fromSection) this.updateSectionCounter(fromSection);
                if (toSection) {
                    toSection.classList.remove('hidden');
                    this.updateSectionCounter(toSection);
                }
                this.refreshStats();
            });

            // Stats refresh triggered by server via HX-Trigger-After-Settle
            document.body.addEventListener('statsRefresh', () => {
                htmx.trigger('#stats-container', 'refresh');
            });

            // Full list refresh triggered by server (e.g. template apply)
            document.body.addEventListener('refreshList', () => {
                this.refreshList();
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

                // Refresh sections list using lightweight per-section fetches
                this.refreshList(false);
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

            // Suppress WS-driven refreshes during full refresh to prevent race conditions
            this._fullRefreshInProgress = true;

            // Reconnect WebSocket if needed
            const wsOpen = this.ws && this.ws.readyState === WebSocket.OPEN;
            if (!wsOpen && this.isOnline) {
                console.log('[App] Reconnecting');
                this.reconnectAttempts = 0;
                this.connect();
            }

            try {
                if (this.isOnline) {
                    const hadQueuedActions = await this.processOfflineQueue();

                    if (!hadQueuedActions) {
                        // Smooth per-section update instead of full innerHTML swap
                        await this.refreshSectionsSmooth();
                        this.refreshStats();
                    }

                    this.cacheData();
                }
            } finally {
                this._fullRefreshInProgress = false;
            }
        },

        async refreshSectionsSmooth() {
            // Delegates to refreshList which now uses lightweight per-section fetches
            this.refreshList(false);
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
            if (this._wsInitialized) return;
            this._wsInitialized = true;

            this.connect();

            document.addEventListener('visibilitychange', () => {
                if (document.visibilityState === 'visible') {
                    this.fullRefresh();
                }
            });
        },

        connect() {
            // Close existing connection to prevent duplicates
            if (this.ws) {
                this.ws.onclose = null; // Prevent scheduleReconnect from firing
                this.ws.close();
                this.ws = null;
            }
            this.stopPingPong();

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
                    this.stopPingPong();
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

            // Clear any pending reconnect timer
            if (this._reconnectTimer) {
                clearTimeout(this._reconnectTimer);
            }

            this.reconnectAttempts++;
            const delay = Math.min(1000 * Math.pow(2, this.reconnectAttempts), 30000);
            this._reconnectTimer = setTimeout(() => {
                this._reconnectTimer = null;
                this.connect();
            }, delay);
        },

        startPingPong() {
            this.stopPingPong();
            this._pingInterval = setInterval(() => {
                if (this.ws && this.ws.readyState === WebSocket.OPEN) {
                    this.ws.send(JSON.stringify({ type: 'ping' }));
                }
            }, 30000);
        },

        stopPingPong() {
            if (this._pingInterval) {
                clearInterval(this._pingInterval);
                this._pingInterval = null;
            }
        },

        handleMessage(data) {
            try {
                const message = JSON.parse(data);
                console.log('WebSocket message:', message.type);

                // Skip refresh-triggering messages during full refresh (background return)
                if (this._fullRefreshInProgress && message.type !== 'pong') {
                    console.log(`[App] Skipping WebSocket message '${message.type}' - full refresh in progress`);
                    return;
                }

                const sectionId = message.data?.section_id;

                switch (message.type) {
                    case 'section_created':
                        // Skip on creating client - HTMX already handled DOM insertion
                        if (!this.isLocalAction('section_created')) {
                            // Add new section to DOM without refreshing the entire list
                            if (message.data?.id) {
                                const sectionsList = document.getElementById('sections-list');
                                if (sectionsList && !document.getElementById(`section-${message.data.id}`)) {
                                    fetch(`/sections/${message.data.id}/html`).then(r => {
                                        if (r.ok) return r.text();
                                    }).then(html => {
                                        // Re-check to avoid race condition with concurrent fetches
                                        if (html && sectionsList && !document.getElementById(`section-${message.data.id}`)) {
                                            sectionsList.insertAdjacentHTML('beforeend', html.trim());
                                            window.checkEmptyStates();
                                            this.$nextTick(() => {
                                                const newEl = document.getElementById(`section-${message.data.id}`);
                                                if (newEl) {
                                                    const container = newEl.querySelector('.items-sortable');
                                                    if (container) this.initSortableForContainer(container);
                                                }
                                                this.initMobileSortable();
                                            });
                                        }
                                    }).catch(e => console.error('[App] Failed to add new section:', e));
                                }
                            }
                            this.refreshSectionSelectsFromServer();
                            this.refreshManageSectionsModal();
                        }
                        break;
                    case 'section_updated':
                        if (!this.isLocalAction('section_updated')) {
                            // Refresh only the changed section, not the entire list
                            if (message.data?.id) {
                                this.refreshSection(message.data.id);
                            }
                            this.refreshSectionSelectsFromServer();
                            this.refreshManageSectionsModal();
                        }
                        break;
                    case 'section_deleted':
                        if (!this.isLocalAction('section_deleted')) {
                            // Remove section from DOM
                            if (message.data?.id) {
                                const delEl = document.getElementById(`section-${message.data.id}`);
                                if (delEl) {
                                    Alpine.destroyTree(delEl);
                                    delEl.remove();
                                }
                            }
                            window.checkEmptyStates();
                            this.refreshSectionSelectsFromServer();
                            this.refreshManageSectionsModal();
                            this.refreshStats();
                        }
                        break;
                    case 'sections_deleted':
                        if (!this.isLocalAction('sections_deleted')) {
                            // Remove multiple sections from DOM
                            if (message.data?.ids) {
                                for (const id of message.data.ids) {
                                    const delSecEl = document.getElementById(`section-${id}`);
                                    if (delSecEl) {
                                        Alpine.destroyTree(delSecEl);
                                        delSecEl.remove();
                                    }
                                }
                            }
                            window.checkEmptyStates();
                            this.refreshSectionSelectsFromServer();
                            this.refreshManageSectionsModal();
                            this.refreshStats();
                        }
                        break;
                    case 'sections_reordered':
                        if (!this.isLocalAction('sections_reordered')) {
                            // Lightweight reorder - move existing DOM elements
                            this.reorderSections();
                            this.refreshManageSectionsModal();
                        }
                        break;
                    case 'item_created':
                        if (!this.isLocalAction('item_created')) {
                            const itemId = message.data?.id;
                            if (itemId && sectionId) {
                                this.insertRemoteItem(itemId, sectionId);
                            } else {
                                sectionId ? this.refreshSection(sectionId) : this.refreshList();
                            }
                        }
                        this.refreshStats();
                        break;
                    case 'item_moved':
                        if (!this.isLocalAction('item_moved')) {
                            const fromId = message.data?.from_section_id;
                            const toId = message.data?.section_id;
                            if (fromId) this.refreshSection(fromId);
                            if (toId && toId !== fromId) this.refreshSection(toId);
                        }
                        this.refreshStats();
                        break;
                    case 'item_deleted':
                        if (!this.isLocalAction('item_deleted')) {
                            const delItemId = message.data?.id;
                            if (delItemId && sectionId) {
                                this.removeRemoteItem(delItemId, sectionId);
                            } else {
                                sectionId ? this.refreshSection(sectionId) : this.refreshList();
                            }
                        }
                        this.refreshStats();
                        break;
                    case 'items_reordered':
                        if (!this.isLocalAction('items_reordered')) {
                            sectionId ? this.refreshSection(sectionId) : this.refreshList();
                        }
                        break;
                    case 'item_toggled':
                        if (!this.isLocalAction('item_toggled')) {
                            const togItemId = message.data?.id;
                            const togCompleted = message.data?.completed;
                            if (togItemId && sectionId) {
                                this.toggleRemoteItem(togItemId, sectionId, togCompleted);
                            } else {
                                sectionId ? this.refreshSection(sectionId) : this.refreshList();
                            }
                        }
                        this.refreshStats();
                        break;
                    case 'item_updated':
                        if (!this.isLocalAction('item_updated')) {
                            const updItemId = message.data?.id;
                            if (updItemId) {
                                this.replaceRemoteItem(updItemId);
                            } else {
                                sectionId ? this.refreshSection(sectionId) : this.refreshList();
                            }
                        }
                        this.refreshStats();
                        break;
                    case 'template_applied':
                        // Template adds items to multiple sections - full refresh needed
                        this.refreshList();
                        this.refreshStats();
                        break;
                    case 'section_sort_changed':
                        if (!this.isLocalAction('section_sort_changed')) {
                            sectionId ? this.refreshSection(sectionId) : this.refreshList();
                        }
                        break;
                    case 'section_items_checked':
                        if (!this.isLocalAction('section_items_checked')) {
                            sectionId ? this.refreshSection(sectionId) : this.refreshList();
                        }
                        this.refreshStats();
                        break;
                    case 'section_items_unchecked':
                        if (!this.isLocalAction('section_items_unchecked')) {
                            sectionId ? this.refreshSection(sectionId) : this.refreshList();
                        }
                        this.refreshStats();
                        break;
                    case 'completed_items_deleted':
                        if (!this.isLocalAction('completed_items_deleted')) {
                            this.removeAllCompletedItemsFromDOM();
                        }
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

        refreshList(showOverlay = true) {
            // Debounce - prevent multiple rapid refreshes
            if (this._refreshListTimer) {
                clearTimeout(this._refreshListTimer);
            }

            this._refreshListTimer = setTimeout(async () => {
                if (this._isRefreshing) return;
                this._isRefreshing = true;

                try {
                    // Fetch current sections list as JSON
                    const resp = await fetch('/sections/list?format=json');
                    if (!resp.ok) { this._isRefreshing = false; return; }
                    const sections = await resp.json();
                    const sectionsList = document.getElementById('sections-list');
                    if (!sectionsList) { this._isRefreshing = false; return; }

                    const serverIds = new Set(sections.map(s => s.id));
                    const domIds = new Set();
                    sectionsList.querySelectorAll(':scope > [id^="section-"]').forEach(el => {
                        const id = parseInt(el.id.replace('section-', ''));
                        if (id) domIds.add(id);
                    });

                    // Remove sections no longer on server
                    for (const id of domIds) {
                        if (!serverIds.has(id)) {
                            const el = document.getElementById(`section-${id}`);
                            if (el) { Alpine.destroyTree(el); el.remove(); }
                        }
                    }

                    // Refresh existing + add new sections
                    for (const section of sections) {
                        const el = document.getElementById(`section-${section.id}`);
                        if (el) {
                            await this.refreshSection(section.id);
                        } else {
                            const r = await fetch(`/sections/${section.id}/html`);
                            if (r.ok) {
                                const html = await r.text();
                                sectionsList.insertAdjacentHTML('beforeend', html.trim());
                            }
                        }
                    }

                    // Fix order (like reorderSections)
                    for (const section of sections) {
                        const el = document.getElementById(`section-${section.id}`);
                        if (el) sectionsList.appendChild(el);
                    }

                    this.$nextTick(() => this.initMobileSortable());
                    this.updateSectionSelects(sections);
                } catch (e) {
                    console.error('[App] refreshList failed:', e);
                }

                this._isRefreshing = false;
            }, 100); // 100ms debounce
        },

        async cycleSortMode(sectionId, currentMode) {
            const modes = ['manual', 'alphabetical', 'alphabetical_desc'];
            const currentIndex = modes.indexOf(currentMode);
            const nextMode = modes[(currentIndex + 1) % modes.length];

            this.markLocalAction('section_sort_changed');

            try {
                const response = await fetch(`/sections/${sectionId}/sort-mode`, {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
                    body: `sort_mode=${encodeURIComponent(nextMode)}`
                });
                if (response.ok) {
                    await this.refreshSection(sectionId);
                }
            } catch (error) {
                console.error('[SortMode] Failed:', error);
            }
        },

        async toggleAllItems(sectionId) {
            const section = document.getElementById(`section-${sectionId}`);
            if (!section) return;

            const hasActive = section.querySelector('.active-items [data-item-id]') !== null;
            const action = hasActive ? 'check-all' : 'uncheck-all';

            this.markLocalAction(action === 'check-all' ? 'section_items_checked' : 'section_items_unchecked');

            try {
                const response = await fetch(`/sections/${sectionId}/${action}`, { method: 'POST' });
                if (response.ok) {
                    await this.refreshSection(sectionId);
                    this.refreshStats();
                }
            } catch (error) {
                console.error('[ToggleAll] Failed:', error);
            }
        },

        async refreshSection(sectionId) {
            const section = document.getElementById(`section-${sectionId}`);
            if (!section) return;

            try {
                const resp = await fetch(`/sections/${sectionId}/html`);
                if (!resp.ok) return;
                const html = await resp.text();
                // Verify section still exists (may have been removed by concurrent operation)
                if (!section.parentNode) return;
                section.insertAdjacentHTML('afterend', html.trim());
                // Remove old section first to prevent duplicates, then clean up Alpine
                section.remove();
                try { Alpine.destroyTree(section); } catch (_) {}
                this.$nextTick(() => {
                    const el = document.getElementById(`section-${sectionId}`);
                    if (el) {
                        const container = el.querySelector('.items-sortable');
                        if (container) this.initSortableForContainer(container);
                    }
                });
            } catch (e) {
                console.error('[App] refreshSection failed:', e);
            }
        },

        updateSectionCounter(section) {
            const activeItems = section.querySelectorAll('.active-items > [id^="item-"]').length;
            const completedContainer = section.querySelector('.completed-items');
            const completedItems = completedContainer ? completedContainer.querySelectorAll('[id^="item-"]').length : 0;
            const totalItems = activeItems + completedItems;

            const counter = section.querySelector('.section-counter');
            if (counter) {
                counter.textContent = `${completedItems}/${totalItems}`;
            }

            if (totalItems === 0) {
                section.classList.add('hidden');
            } else {
                section.classList.remove('hidden');
            }
        },

        updateCompletedCount(section) {
            const completedContainer = section.querySelector('.completed-items');
            const completedItems = completedContainer ? completedContainer.querySelectorAll('[id^="item-"]').length : 0;

            const countSpan = section.querySelector('.completed-wrapper .completed-count');
            if (countSpan) {
                countSpan.textContent = completedItems;
            }
        },

        // Show/hide completed wrapper based on completed items count
        updateCompletedVisibility(section) {
            const completedWrapper = section.querySelector('.completed-wrapper');
            const completedContainer = section.querySelector('.completed-items');
            const completedCount = completedContainer
                ? completedContainer.querySelectorAll('[id^="item-"]').length : 0;

            if (completedWrapper) {
                completedWrapper.style.display = completedCount === 0 ? 'none' : '';
            }
            const countEl = section.querySelector('.completed-count');
            if (countEl) countEl.textContent = completedCount;
        },

        handleAfterMove(itemId, fromSectionId, toSectionId, newItemHtml) {
            const item = document.getElementById(`item-${itemId}`);
            const fromSection = document.getElementById(`section-${fromSectionId}`);
            const toSection = document.getElementById(`section-${toSectionId}`);

            if (item && toSection) {
                const targetContainer = toSection.querySelector('.active-items');
                if (targetContainer) {
                    if (newItemHtml) {
                        // Replace with server HTML using insertAdjacentHTML for proper Alpine init
                        Alpine.destroyTree(item);
                        item.remove();
                        targetContainer.insertAdjacentHTML('beforeend', newItemHtml.trim());
                    } else {
                        targetContainer.appendChild(item);
                        item.dataset.sectionId = String(toSectionId);
                    }
                }
            }

            if (fromSection) {
                this.updateSectionCounter(fromSection);
            }
            if (toSection) {
                toSection.classList.remove('hidden');
                this.updateSectionCounter(toSection);
            }

            this.refreshStats();
            this.$nextTick(() => this.initMobileSortable());

            const toSortMode = toSection?.dataset?.sortMode || 'manual';
            if (toSortMode !== 'manual') {
                this.$nextTick(() => this.refreshSection(parseInt(toSectionId)));
            }
        },

        async refreshSectionsAndSelects() {
            // refreshList() now handles section selects update internally
            this.refreshList();
        },

        async refreshSectionSelectsFromServer() {
            try {
                const r = await fetch('/sections/list?format=json');
                if (r.ok) {
                    const sections = await r.json();
                    this.updateSectionSelects(sections);
                    this.refreshAllMoveMenus(sections);
                }
            } catch (e) {
                console.error('[App] refreshSectionSelectsFromServer failed:', e);
            }
        },

        refreshManageSectionsModal() {
            const manageSectionsList = document.getElementById('manage-sections-list');
            if (manageSectionsList) {
                fetch('/sections/list').then(r => {
                    if (r.ok) return r.text();
                }).then(html => {
                    if (html) {
                        manageSectionsList.innerHTML = html;
                        htmx.process(manageSectionsList);
                    }
                }).catch(e => console.error('[App] manage sections refresh failed:', e));
            }
        },

        async reorderSections() {
            try {
                const response = await fetch('/sections/list?format=json');
                if (!response.ok) return;
                const sections = await response.json();
                const sectionsList = document.getElementById('sections-list');
                if (sectionsList) {
                    for (const section of sections) {
                        const el = document.getElementById(`section-${section.id}`);
                        if (el) sectionsList.appendChild(el);
                    }
                }
                this.updateSectionSelects(sections);
                this.refreshAllMoveMenus(sections);
            } catch (e) {
                console.error('[App] reorderSections failed:', e);
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

        refreshAllMoveMenus(sections) {
            // Update desktop move menus in each item
            document.querySelectorAll('[id^="item-"]').forEach(itemEl => {
                const itemId = parseInt(itemEl.dataset.itemId);
                const sectionId = parseInt(itemEl.dataset.sectionId);
                if (!itemId || !sectionId) return;

                // Find the move dropdown container (the div with x-show="open" inside desktop actions)
                const desktopActions = itemEl.querySelector('.md\\:flex');
                if (!desktopActions) return;
                const moveDropdown = desktopActions.querySelector('[x-show="open"]');
                if (!moveDropdown) return;

                moveDropdown.innerHTML = '';
                sections.forEach(section => {
                    if (section.id === sectionId) return;
                    const btn = document.createElement('button');
                    btn.className = 'block w-full text-left px-3 py-1.5 text-sm text-stone-600 dark:text-stone-300 hover:bg-stone-50 dark:hover:bg-stone-700';
                    btn.textContent = section.name;
                    btn.addEventListener('click', () => {
                        // Close the dropdown via Alpine
                        const alpineData = Alpine.$data(moveDropdown.closest('[x-data]'));
                        if (alpineData) alpineData.open = false;
                        this.moveItemDesktop(itemId, sectionId, section.id);
                    });
                    moveDropdown.appendChild(btn);
                });
            });

            // Update mobile action sheet move buttons
            const mobileMoveContainer = document.querySelector('[x-show="showSections"]');
            if (mobileMoveContainer) {
                mobileMoveContainer.innerHTML = '';
                sections.forEach(section => {
                    const btn = document.createElement('button');
                    btn.className = 'w-full text-left p-2 rounded-lg text-sm text-stone-600 dark:text-stone-300 hover:bg-stone-100 dark:hover:bg-stone-700';
                    btn.textContent = section.name;
                    btn.setAttribute('x-show', `mobileActionItem?.section_id != ${section.id}`);
                    btn.addEventListener('click', () => {
                        this.moveToSection(section.id);
                    });
                    mobileMoveContainer.appendChild(btn);
                });
            }
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
                this.markLocalAction('sections_deleted');
                const response = await fetch('/sections/batch-delete', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
                    body: `ids=${this.selectedSections.join(',')}`
                });

                if (response.ok) {
                    // Remove deleted sections from DOM
                    for (const id of this.selectedSections) {
                        const el = document.getElementById(`section-${id}`);
                        if (el) {
                            Alpine.destroyTree(el);
                            el.remove();
                        }
                    }
                    this.selectMode = false;
                    this.selectedSections = [];
                    window.checkEmptyStates();
                    this.refreshSectionSelectsFromServer();
                    this.refreshManageSectionsModal();
                    this.refreshStats();
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
                uncertain: item.uncertain,
                quantity: item.quantity || 0
            };
        },

        closeMobileAction() {
            this.mobileActionItem = null;
        },

        async toggleUncertain() {
            if (!this.mobileActionItem) return;
            const itemId = this.mobileActionItem.id;
            const sectionId = this.mobileActionItem.section_id;

            // Mark as local action to prevent WebSocket race condition
            this.markLocalAction('item_updated');

            // Optimistic UI update for modal
            this.mobileActionItem.uncertain = !this.mobileActionItem.uncertain;
            this.mobileActionItem = null;

            if (!this.isOnline) {
                // Queue for offline sync
                await this.queueOfflineAction({
                    type: 'toggle_uncertain',
                    url: `/items/${itemId}/uncertain`,
                    method: 'POST'
                });
                return;
            }

            // Fetch + DOM replace (no HTMX)
            try {
                const response = await fetch(`/items/${itemId}/uncertain`, { method: 'POST' });
                if (!response.ok) return;
                const html = await response.text();
                if (!html.trim()) return;
                const oldItem = document.getElementById(`item-${itemId}`);
                if (oldItem) {
                    oldItem.insertAdjacentHTML('afterend', html.trim());
                    Alpine.destroyTree(oldItem);
                    oldItem.remove();
                }
            } catch (error) {
                console.error('[Uncertain] Failed:', error);
            }
        },

        // Toggle uncertain via desktop button (no mobile action sheet)
        async toggleUncertainFetch(itemId) {
            this.markLocalAction('item_updated');
            try {
                const response = await fetch(`/items/${itemId}/uncertain`, { method: 'POST' });
                if (!response.ok) return;
                const html = await response.text();
                if (!html.trim()) return;
                const oldItem = document.getElementById(`item-${itemId}`);
                if (oldItem) {
                    oldItem.insertAdjacentHTML('afterend', html.trim());
                    Alpine.destroyTree(oldItem);
                    oldItem.remove();
                }
            } catch (error) {
                console.error('[Uncertain] Failed:', error);
            }
        },

        // Toggle item completed status via fetch (no HTMX - avoids section re-render)
        _toggleInFlight: {},
        async toggleItem(itemId, sectionId) {
            // Prevent concurrent toggles on the same item
            if (this._toggleInFlight[itemId]) return;
            this._toggleInFlight[itemId] = true;

            this.markLocalAction('item_toggled');

            try {
                const response = await this.offlineFetch(
                    `/items/${itemId}/toggle`,
                    { method: 'POST' },
                    'toggle_item'
                );

                if (response.offline) {
                    await this._applyOfflineToggle(itemId, sectionId);
                    return;
                }

                if (!response.ok) {
                    console.error('[Toggle] Server error:', response.status);
                    return;
                }

                // Refresh the entire section to get correct sort order
                await this.refreshSection(sectionId);
                this.refreshStats();
            } catch (error) {
                // Intermittent signal: isOnline=true but fetch fails
                console.error('[Toggle] Failed, falling back to offline queue:', error);
                await this.queueOfflineAction({
                    type: 'toggle_item',
                    url: `/items/${itemId}/toggle`,
                    method: 'POST'
                });
                await this._applyOfflineToggle(itemId, sectionId);
            } finally {
                delete this._toggleInFlight[itemId];
            }
        },

        async _applyOfflineToggle(itemId, sectionId) {
            const itemEl = document.getElementById(`item-${itemId}`);
            if (!itemEl) return;

            const section = document.getElementById(`section-${sectionId}`);
            const checkboxSpan = itemEl.querySelector('button > span');
            const isCompleted = checkboxSpan && checkboxSpan.classList.contains('bg-pink-400');

            // Toggle visual checkbox state
            if (isCompleted) {
                // Uncomplete: pink checkbox -> empty border
                if (checkboxSpan) {
                    checkboxSpan.classList.remove('bg-pink-400', 'flex', 'items-center', 'justify-center');
                    checkboxSpan.classList.add('border-2', 'border-stone-300', 'hover:border-pink-400', 'hover:scale-110');
                    checkboxSpan.innerHTML = '';
                }
                // Remove strikethrough from text
                const textEl = itemEl.querySelector('.line-through');
                if (textEl) {
                    textEl.classList.remove('line-through', 'text-stone-400', 'text-stone-300');
                    textEl.classList.add('text-stone-700');
                }
                // Move to active items container
                if (section) {
                    const activeContainer = section.querySelector('.active-items');
                    if (activeContainer) activeContainer.appendChild(itemEl);
                }
                // Update stats
                this.stats.completed = Math.max(0, this.stats.completed - 1);
            } else {
                // Complete: empty border -> pink checkbox
                if (checkboxSpan) {
                    checkboxSpan.classList.remove('border-2', 'border-stone-300', 'hover:border-pink-400', 'hover:scale-110');
                    checkboxSpan.classList.add('bg-pink-400', 'flex', 'items-center', 'justify-center');
                    checkboxSpan.innerHTML = '<svg class="w-3 h-3 text-white" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="3" d="M5 13l4 4L19 7"></path></svg>';
                }
                // Add strikethrough to text
                const textEl = itemEl.querySelector('.text-stone-700');
                if (textEl) {
                    textEl.classList.remove('text-stone-700');
                    textEl.classList.add('line-through', 'text-stone-400');
                }
                // Move to completed items container
                if (section) {
                    const completedContainer = section.querySelector('.completed-items');
                    if (completedContainer) completedContainer.appendChild(itemEl);
                }
                // Update stats
                this.stats.completed++;
            }

            // Update stats percentage
            this.stats.percentage = Math.round((this.stats.completed / this.stats.total) * 100) || 0;

            // Add pending sync styling
            itemEl.classList.add('pending-sync', 'bg-rose-50/40', 'border-l-2', 'border-rose-400');
            itemEl.dataset.pendingSync = 'true';

            // Add sync badge
            if (!itemEl.querySelector('.offline-sync-badge')) {
                const badge = document.createElement('span');
                badge.className = 'offline-sync-badge inline-flex items-center gap-1 px-2 py-0.5 rounded-full text-xs font-medium bg-rose-100 text-rose-600 ml-2';
                badge.innerHTML = `
                    <svg class="w-3 h-3 animate-spin" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15"></path>
                    </svg>
                    sync
                `;
                const contentDiv = itemEl.querySelector('.flex-1.min-w-0');
                if (contentDiv) contentDiv.after(badge);
            }

            // Animate checkbox
            if (checkboxSpan) {
                checkboxSpan.classList.add('checkbox-pulse');
                setTimeout(() => checkboxSpan.classList.remove('checkbox-pulse'), 300);
            }

            // Update section counters
            if (section) {
                this.updateSectionCounter(section);
                this.updateCompletedCount(section);
                this.updateCompletedVisibility(section);
            }

            // Update IndexedDB cache
            if (this.offlineStorageReady) {
                await window.offlineStorage.updateItemInCache(
                    parseInt(itemId),
                    { completed: !isCompleted }
                );
            }

            console.log('[Toggle] Offline toggle applied:', itemId, isCompleted ? '-> active' : '-> completed');
        },

        async moveItemDesktop(itemId, fromSectionId, toSectionId) {
            this.markLocalAction('item_moved');
            try {
                const response = await fetch(`/items/${itemId}/move`, {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
                    body: `section_id=${toSectionId}`
                });
                if (!response.ok) {
                    console.error('Failed to move item');
                    return;
                }
                const html = await response.text();
                this.handleAfterMove(itemId, fromSectionId, toSectionId, html);
            } catch (error) {
                console.error('Failed to move item:', error);
            }
        },

        async moveToSection(sectionId) {
            if (!this.mobileActionItem) return;
            if (!this.isOnline) {
                window.Toast.show(t('offline.action_blocked'), 'warning');
                this.mobileActionItem = null;
                return;
            }
            const itemId = this.mobileActionItem.id;
            const fromSectionId = this.mobileActionItem.section_id;
            this.mobileActionItem = null;
            this.markLocalAction('item_moved');

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
                    const html = await response.text();
                    this.handleAfterMove(itemId, fromSectionId, parseInt(sectionId), html);
                }
            } catch (error) {
                console.error('Failed to move item:', error);
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
            const sectionId = this.mobileActionItem.section_id;
            this.mobileActionItem = null;

            await this.deleteItemDirect(itemId, sectionId);
        },

        // Delete item via fetch + DOM removal (used by both desktop and mobile)
        async deleteItemDirect(itemId, sectionId) {
            this.markLocalAction('item_deleted');
            try {
                const response = await fetch(`/items/${itemId}`, { method: 'DELETE' });
                if (!response.ok) return;

                const item = document.getElementById(`item-${itemId}`);
                if (item) {
                    Alpine.destroyTree(item);
                    item.remove();
                }

                const section = document.getElementById(`section-${sectionId}`);
                if (section) {
                    this.updateSectionCounter(section);
                    this.updateCompletedVisibility(section);
                }

                window.checkEmptyStates();
                this.refreshStats();
            } catch (error) {
                console.error('[Delete] Failed:', error);
            }
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

                    this.markLocalAction('completed_items_deleted');

                    // Close settings modal
                    this.showSettings = false;

                    // Remove completed items from DOM instead of full refresh
                    this.removeAllCompletedItemsFromDOM();
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

        // Remove all completed items from DOM (no server fetch)
        removeAllCompletedItemsFromDOM() {
            document.querySelectorAll('.completed-items').forEach(container => {
                const items = container.querySelectorAll('[id^="item-"]');
                items.forEach(item => {
                    try { Alpine.destroyTree(item); } catch (_) {}
                    item.remove();
                });
                // Update section counters and visibility
                const section = container.closest('[id^="section-"]');
                if (section) {
                    this.updateSectionCounter(section);
                    this.updateCompletedVisibility(section);
                }
            });
            window.checkEmptyStates();
        },

        // ===== REMOTE ITEM HELPERS (for WS events on other browsers) =====

        // Insert a new item fetched from server into the correct section
        async insertRemoteItem(itemId, sectionId) {
            try {
                const resp = await fetch(`/items/${itemId}/html`);
                if (!resp.ok) { this.refreshSection(sectionId); return; }
                const html = await resp.text();
                const section = document.getElementById(`section-${sectionId}`);
                if (!section) return;
                const container = section.querySelector('.active-items');
                if (container) {
                    container.insertAdjacentHTML('beforeend', html.trim());
                }
                document.getElementById('empty-no-products')?.remove();
                section.classList.remove('hidden');
                this.updateSectionCounter(section);
            } catch (e) {
                console.error('[App] insertRemoteItem failed:', e);
                this.refreshSection(sectionId);
            }
        },

        // Remove a deleted item from DOM (no fetch needed)
        removeRemoteItem(itemId, sectionId) {
            const el = document.getElementById(`item-${itemId}`);
            if (el) {
                try { Alpine.destroyTree(el); } catch (_) {}
                el.remove();
            }
            const section = document.getElementById(`section-${sectionId}`);
            if (section) {
                this.updateSectionCounter(section);
                this.updateCompletedVisibility(section);
            }
            window.checkEmptyStates();
        },

        // Replace an updated item in-place with fresh HTML from server
        async replaceRemoteItem(itemId) {
            const existing = document.getElementById(`item-${itemId}`);
            if (!existing) return;
            try {
                const resp = await fetch(`/items/${itemId}/html`);
                if (!resp.ok) {
                    const sectionId = existing.dataset.sectionId;
                    if (sectionId) this.refreshSection(parseInt(sectionId));
                    return;
                }
                const html = await resp.text();
                existing.insertAdjacentHTML('afterend', html.trim());
                try { Alpine.destroyTree(existing); } catch (_) {}
                existing.remove();
            } catch (e) {
                console.error('[App] replaceRemoteItem failed:', e);
                const sectionId = existing.dataset?.sectionId;
                if (sectionId) this.refreshSection(parseInt(sectionId));
            }
        },

        // Toggle an item on remote browser: remove from old container, fetch new HTML, insert into correct container
        async toggleRemoteItem(itemId, sectionId, isCompleted) {
            const existing = document.getElementById(`item-${itemId}`);
            const section = document.getElementById(`section-${sectionId}`);
            if (!section) { this.refreshSection(sectionId); return; }

            try {
                const resp = await fetch(`/items/${itemId}/html`);
                if (!resp.ok) { this.refreshSection(sectionId); return; }
                const html = await resp.text();

                // Remove old element
                if (existing) {
                    try { Alpine.destroyTree(existing); } catch (_) {}
                    existing.remove();
                }

                // Insert into the correct container
                const targetContainer = isCompleted
                    ? section.querySelector('.completed-items')
                    : section.querySelector('.active-items');
                if (targetContainer) {
                    targetContainer.insertAdjacentHTML('beforeend', html.trim());
                }

                this.updateSectionCounter(section);
                this.updateCompletedVisibility(section);
            } catch (e) {
                console.error('[App] toggleRemoteItem failed:', e);
                this.refreshSection(sectionId);
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
                const offlineQty = itemEl.getAttribute('data-offline-quantity');

                // Use offline data if exists, otherwise use original
                this.editItemName = offlineName !== null ? offlineName : item.name;
                this.editItemDescription = offlineDesc !== null ? offlineDesc : (item.description || '');
                this.editItemQuantity = offlineQty !== null ? parseInt(offlineQty) || 0 : (item.quantity || 0);
            } else {
                this.editItemName = item.name;
                this.editItemDescription = item.description || '';
                this.editItemQuantity = item.quantity || 0;
            }

            this.$nextTick(() => {
                const input = document.querySelector('[x-model="editItemName"]');
                if (input) input.focus();
            });
        },

        async submitEditItem() {
            if (!this.editItemName.trim() || !this.editingItem) return;

            const itemId = this.editingItem.id;
            const itemEl = document.getElementById(`item-${itemId}`);
            const sectionId = itemEl?.closest('div[id^="section-"]')?.dataset?.sectionId;
            const name = this.editItemName.trim();
            const description = this.editItemDescription.trim();
            const quantity = parseInt(this.editItemQuantity) || 0;
            const body = `name=${encodeURIComponent(name)}&description=${encodeURIComponent(description)}&quantity=${quantity}`;

            this.editingItem = null;
            this.editItemName = '';
            this.editItemDescription = '';
            this.editItemQuantity = 0;

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

            this.markLocalAction('item_updated');

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
                    const html = await response.text();
                    const itemEl = document.getElementById(`item-${itemId}`);
                    if (itemEl && html) {
                        Alpine.destroyTree(itemEl);
                        itemEl.outerHTML = html.trim();
                    }
                    this.refreshStats();
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
            }
        },


        // Quick add inline - opens inline input in section
        openQuickAdd(sectionId) {
            // iOS keyboard trick: focus hidden input SYNCHRONOUSLY to trigger keyboard
            const iosTrigger = document.getElementById('ios-keyboard-trigger');
            if (iosTrigger) {
                iosTrigger.focus();
            }

            // Close any other open quick add
            this.quickAddSectionId = sectionId;
            this.quickAddName = '';
            this.quickAddSuggestions = [];
            this.showQuickAddSuggestions = false;
            this.selectedQuickAddSuggestionIndex = -1;

            // Transfer focus to actual input after Alpine renders it
            this.$nextTick(() => {
                setTimeout(() => {
                    const input = document.querySelector(`#quick-add-input-${sectionId}`);
                    if (input) {
                        input.focus();
                    }
                }, 10);
            });
        },

        closeQuickAdd() {
            this.quickAddSectionId = null;
            this.quickAddName = '';
            this.quickAddSuggestions = [];
            this.showQuickAddSuggestions = false;
            this.selectedQuickAddSuggestionIndex = -1;
        },

        async submitQuickAdd(sectionId) {
            const name = this.quickAddName.trim();
            if (!name) return;

            // Mark as local action to prevent double refresh from WebSocket
            this.markLocalAction('item_created');

            // Offline: optimistic UI without refresh (like standard add)
            if (!this.isOnline) {
                const tempId = 'offline-' + Date.now();

                // Check if item already exists in completed items - reactivate instead of duplicate
                const sectionEl = document.getElementById(`section-${sectionId}`);
                if (sectionEl) {
                    const completedItems = sectionEl.querySelectorAll('.completed-items [id^="item-"]');
                    for (const el of completedItems) {
                        const nameEl = el.querySelector('.item-name, [data-item-name]');
                        const itemName = nameEl?.textContent?.trim() || nameEl?.dataset?.itemName || '';
                        if (itemName.toLowerCase() === name.toLowerCase()) {
                            // Move from completed to active optimistically
                            const activeContainer = sectionEl.querySelector('.active-items');
                            if (activeContainer) {
                                el.remove();
                                const html = createOfflineItemHtml(tempId, name, '', sectionId);
                                activeContainer.insertAdjacentHTML('afterbegin', html);
                                this.markLocalAction('item_toggled');

                                // Update section counter (completed - 1, total stays the same)
                                const counter = sectionEl.querySelector('.section-counter');
                                if (counter) {
                                    const parts = counter.textContent.split('/');
                                    const completed = Math.max(0, (parseInt(parts[0]) || 0) - 1);
                                    const total = parseInt(parts[1]) || 0;
                                    counter.textContent = `${completed}/${total}`;
                                }

                                // Update completed count and hide wrapper if empty
                                const completedContainer = sectionEl.querySelector('.completed-items');
                                const newCompletedCount = completedContainer
                                    ? completedContainer.querySelectorAll('[id^="item-"]').length : 0;
                                const countSpan = sectionEl.querySelector('.completed-count');
                                if (countSpan) countSpan.textContent = newCompletedCount;
                                const completedWrapper = sectionEl.querySelector('.completed-wrapper');
                                if (completedWrapper) {
                                    completedWrapper.style.display = newCompletedCount === 0 ? 'none' : '';
                                }
                            }
                            // Queue toggle for sync
                            await window.offlineStorage.queueAction({
                                type: 'create_item',
                                url: '/items',
                                method: 'POST',
                                headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
                                body: `section_id=${sectionId}&name=${encodeURIComponent(name)}&description=`,
                                tempId: tempId
                            });
                            this.closeQuickAdd();
                            window.Toast?.show(t('offline.queued'), 'info', 2000);
                            return;
                        }
                    }
                }

                // Create optimistic item HTML
                const itemHtml = createOfflineItemHtml(tempId, name, '', sectionId);

                // Find section and add item
                document.getElementById('empty-no-products')?.remove();
                const offlineSectionEl = document.getElementById(`section-${sectionId}`);
                if (offlineSectionEl) {
                    offlineSectionEl.classList.remove('hidden');
                    const itemsContainer = offlineSectionEl.querySelector('.active-items');
                    if (itemsContainer) {
                        itemsContainer.insertAdjacentHTML('afterbegin', itemHtml);
                    }

                    // Update section counter
                    const counter = offlineSectionEl.querySelector('.section-counter');
                    if (counter) {
                        const parts = counter.textContent.split('/');
                        const completed = parseInt(parts[0]) || 0;
                        const total = (parseInt(parts[1]) || 0) + 1;
                        counter.textContent = `${completed}/${total}`;
                    }
                }

                // Queue for sync
                await window.offlineStorage.queueAction({
                    type: 'create_item',
                    url: '/items',
                    method: 'POST',
                    headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
                    body: `section_id=${sectionId}&name=${encodeURIComponent(name)}&description=`,
                    tempId: tempId
                });

                // Close quick add (no refresh!)
                this.closeQuickAdd();

                // Confirmation toast
                window.Toast?.show(t('offline.queued'), 'info', 2000);

                return;
            }

            // Online: append item partial directly
            try {
                const formData = new URLSearchParams();
                formData.append('name', name);
                formData.append('section_id', sectionId);
                formData.append('quick_add', 'true');

                const response = await this.offlineFetch('/items', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
                    body: formData.toString()
                }, 'create_item');

                if (response.ok) {
                    // Check if item was reactivated (moved from completed to active)
                    const reactivated = response.headers.get('X-Item-Reactivated') === 'true';
                    const alreadyActive = response.headers.get('X-Item-Already-Active') === 'true';

                    if (reactivated) {
                        // Item was unchecked - refresh section to move it from completed to active
                        await this.refreshSection(sectionId);
                        this.closeQuickAdd();
                        this.refreshStats();
                        this.$nextTick(() => this.initMobileSortable());
                    } else if (alreadyActive) {
                        // Item already exists and is active - flash it
                        const existingId = response.headers.get('X-Item-Existing-ID');
                        if (existingId) {
                            const existingEl = document.getElementById(`item-${existingId}`);
                            if (existingEl) {
                                existingEl.classList.add('ring-2', 'ring-pink-400', 'bg-pink-50', 'dark:bg-pink-900/20');
                                setTimeout(() => {
                                    existingEl.classList.remove('ring-2', 'ring-pink-400', 'bg-pink-50', 'dark:bg-pink-900/20');
                                }, 1500);
                                existingEl.scrollIntoView({ behavior: 'smooth', block: 'nearest' });
                            }
                        }
                        this.closeQuickAdd();
                    } else {
                        const section = document.getElementById(`section-${sectionId}`);
                        const sortMode = section?.dataset?.sortMode;
                        if (sortMode === 'alphabetical' || sortMode === 'alphabetical_desc') {
                            // Sorted section - refresh to insert at correct alphabetical position
                            await this.refreshSection(sectionId);
                        } else {
                            const html = await response.text();
                            if (section && html) {
                                const activeContainer = section.querySelector('.active-items');
                                if (activeContainer) {
                                    // Alpine's mutation observer auto-initializes new elements
                                    activeContainer.insertAdjacentHTML('beforeend', html.trim());
                                }
                                document.getElementById('empty-no-products')?.remove();
                                section.classList.remove('hidden');
                                this.updateSectionCounter(section);
                            }
                        }
                        this.closeQuickAdd();
                        this.refreshStats();
                        this.$nextTick(() => this.initMobileSortable());
                    }
                }
            } catch (error) {
                console.error('[QuickAdd] Failed to add item:', error);
            }
        },

        // Desktop add item form (full form with section select)
        async submitAddItemForm(form) {
            const formData = new FormData(form);
            const sectionId = formData.get('section_id');
            const name = formData.get('name')?.trim();
            if (!name) return;

            this.markLocalAction('item_created');

            try {
                const response = await fetch('/items', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
                    body: new URLSearchParams(formData).toString()
                });

                if (response.ok) {
                    const reactivated = response.headers.get('X-Item-Reactivated') === 'true';
                    const alreadyActive = response.headers.get('X-Item-Already-Active') === 'true';

                    if (reactivated) {
                        await this.refreshSection(sectionId);
                    } else if (alreadyActive) {
                        const existingId = response.headers.get('X-Item-Existing-ID');
                        if (existingId) {
                            const existingEl = document.getElementById(`item-${existingId}`);
                            if (existingEl) {
                                existingEl.classList.add('ring-2', 'ring-pink-400', 'bg-pink-50', 'dark:bg-pink-900/20');
                                setTimeout(() => {
                                    existingEl.classList.remove('ring-2', 'ring-pink-400', 'bg-pink-50', 'dark:bg-pink-900/20');
                                }, 1500);
                                existingEl.scrollIntoView({ behavior: 'smooth', block: 'nearest' });
                            }
                        }
                    } else {
                        const section = document.getElementById(`section-${sectionId}`);
                        const sortMode = section?.dataset?.sortMode;
                        if (sortMode === 'alphabetical' || sortMode === 'alphabetical_desc') {
                            // Sorted section - refresh to insert at correct alphabetical position
                            await this.refreshSection(sectionId);
                        } else {
                            const html = await response.text();
                            if (section && html) {
                                const activeContainer = section.querySelector('.active-items');
                                if (activeContainer) {
                                    activeContainer.insertAdjacentHTML('beforeend', html.trim());
                                }
                                document.getElementById('empty-no-products')?.remove();
                                section.classList.remove('hidden');
                                this.updateSectionCounter(section);
                            }
                        }
                    }
                    // Clear form but keep section selected
                    const sectionValue = form.querySelector('select[name="section_id"]').value;
                    form.querySelector('input[name="name"]').value = '';
                    const descInput = form.querySelector('input[name="description"]');
                    if (descInput) descInput.value = '';
                    form.querySelector('select[name="section_id"]').value = sectionValue;
                    setTimeout(() => form.querySelector('input[name="name"]')?.focus(), 50);

                    this.refreshStats();
                    this.$nextTick(() => this.initMobileSortable());
                }
            } catch (error) {
                console.error('[AddItem] Failed:', error);
            }
        },

        // Mobile add item form (modal with add-more toggle)
        async submitAddItemFormMobile(form) {
            const formData = new FormData(form);
            const sectionId = formData.get('section_id');
            const name = formData.get('name')?.trim();
            if (!name) return;

            this.markLocalAction('item_created');

            try {
                const response = await fetch('/items', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
                    body: new URLSearchParams(formData).toString()
                });

                if (response.ok) {
                    const reactivated = response.headers.get('X-Item-Reactivated') === 'true';
                    const alreadyActive = response.headers.get('X-Item-Already-Active') === 'true';

                    if (reactivated) {
                        await this.refreshSection(sectionId);
                    } else if (alreadyActive) {
                        const existingId = response.headers.get('X-Item-Existing-ID');
                        if (existingId) {
                            const existingEl = document.getElementById(`item-${existingId}`);
                            if (existingEl) {
                                existingEl.classList.add('ring-2', 'ring-pink-400', 'bg-pink-50', 'dark:bg-pink-900/20');
                                setTimeout(() => {
                                    existingEl.classList.remove('ring-2', 'ring-pink-400', 'bg-pink-50', 'dark:bg-pink-900/20');
                                }, 1500);
                                existingEl.scrollIntoView({ behavior: 'smooth', block: 'nearest' });
                            }
                        }
                    } else {
                        const html = await response.text();
                        const section = document.getElementById(`section-${sectionId}`);
                        if (section && html) {
                            const activeContainer = section.querySelector('.active-items');
                            if (activeContainer) {
                                activeContainer.insertAdjacentHTML('beforeend', html.trim());
                            }
                            document.getElementById('empty-no-products')?.remove();
                            section.classList.remove('hidden');
                            this.updateSectionCounter(section);
                        }
                    }

                    if (!this.addMore) {
                        form.reset();
                        this.showAddItem = false;
                    } else {
                        form.querySelector('[name=name]').value = '';
                        const descInput = form.querySelector('[name=description]');
                        if (descInput) descInput.value = '';
                        setTimeout(() => form.querySelector('[name=name]')?.focus(), 150);
                    }

                    this.refreshStats();
                    this.$nextTick(() => this.initMobileSortable());
                }
            } catch (error) {
                console.error('[AddItemMobile] Failed:', error);
            }
        },

        async fetchQuickAddSuggestions(query) {
            if (this._quickAddSuggestionTimer) {
                clearTimeout(this._quickAddSuggestionTimer);
            }

            if (!query || query.length < 2) {
                this.quickAddSuggestions = [];
                this.showQuickAddSuggestions = false;
                return;
            }

            this._quickAddSuggestionTimer = setTimeout(async () => {
                try {
                    if (this.isOnline) {
                        const response = await fetch(`/api/suggestions?q=${encodeURIComponent(query)}&limit=8`);
                        if (response.ok) {
                            this.quickAddSuggestions = await response.json();
                        }
                    } else {
                        this.quickAddSuggestions = await window.offlineStorage.getSuggestions(query);
                    }
                    this.showQuickAddSuggestions = this.quickAddSuggestions && this.quickAddSuggestions.length > 0;
                    this.selectedQuickAddSuggestionIndex = -1;
                } catch (error) {
                    console.error('[QuickAdd] Failed to fetch suggestions:', error);
                    this.quickAddSuggestions = [];
                    this.showQuickAddSuggestions = false;
                }
            }, 150);
        },

        selectQuickAddSuggestion(suggestion) {
            this.quickAddName = suggestion.name;
            this.showQuickAddSuggestions = false;
            this.quickAddSuggestions = [];
            this.selectedQuickAddSuggestionIndex = -1;
        },

        handleQuickAddKeydown(event, sectionId) {
            // Handle suggestions navigation
            if (this.showQuickAddSuggestions && this.quickAddSuggestions.length > 0) {
                switch (event.key) {
                    case 'ArrowDown':
                        event.preventDefault();
                        this.selectedQuickAddSuggestionIndex = Math.min(
                            this.selectedQuickAddSuggestionIndex + 1,
                            this.quickAddSuggestions.length - 1
                        );
                        return;
                    case 'ArrowUp':
                        event.preventDefault();
                        this.selectedQuickAddSuggestionIndex = Math.max(this.selectedQuickAddSuggestionIndex - 1, -1);
                        return;
                    case 'Enter':
                        if (this.selectedQuickAddSuggestionIndex >= 0) {
                            event.preventDefault();
                            this.selectQuickAddSuggestion(this.quickAddSuggestions[this.selectedQuickAddSuggestionIndex]);
                            return;
                        }
                        break;
                }
            }

            // Handle Enter to submit
            if (event.key === 'Enter' && !event.shiftKey) {
                event.preventDefault();
                this.submitQuickAdd(sectionId);
                return;
            }

            // Handle Escape to close
            if (event.key === 'Escape') {
                event.preventDefault();
                if (this.showQuickAddSuggestions) {
                    this.showQuickAddSuggestions = false;
                    this.selectedQuickAddSuggestionIndex = -1;
                } else {
                    this.closeQuickAdd();
                }
            }
        },

        hideQuickAddSuggestionsDelayed() {
            setTimeout(() => {
                this.showQuickAddSuggestions = false;
            }, 200);
        },

        // Search functionality
        performSearch(query) {
            if (!query || query.length < 2) {
                this.searchResults = [];
                return;
            }

            const items = document.querySelectorAll('[data-item-id]');
            const results = [];
            const threshold = 0.8;

            items.forEach(el => {
                const name = el.querySelector('.item-name')?.textContent || '';
                const score = fuzzyMatchScore(query, name);

                if (score >= threshold) {
                    results.push({ id: el.dataset.itemId, score });
                }
            });

            results.sort((a, b) => b.score - a.score);
            this.searchResults = results.map(r => r.id);
        },

        clearSearch() {
            this.searchQuery = '';
            this.searchResults = [];
        },

        isItemVisible(itemId) {
            if (!this.searchQuery || this.searchQuery.length < 2) return true;
            return this.searchResults.includes(String(itemId));
        },

        isSectionVisible(sectionId) {
            if (!this.searchQuery || this.searchQuery.length < 2) return true;

            // Check if any item in section matches
            const sectionEl = document.getElementById('section-' + sectionId);
            if (!sectionEl) return true;

            const items = sectionEl.querySelectorAll('[data-item-id]');
            return Array.from(items).some(el => this.searchResults.includes(el.dataset.itemId));
        },

        expandToFullModal(sectionId) {
            const currentName = this.quickAddName;
            this.closeQuickAdd();

            // Open mobile modal with pre-filled data
            this.showAddItem = true;
            this.$nextTick(() => {
                // Set section
                const mobileSelect = this.$refs.mobileSectionSelect;
                if (mobileSelect) {
                    mobileSelect.value = sectionId;
                }
                // Set name
                const nameInput = this.$refs.itemNameInput;
                if (nameInput) {
                    nameInput.value = currentName;
                    setTimeout(() => nameInput.focus(), 100);
                }
            });
        },

        showEmptyDropTargets() {
            const sectionsList = document.getElementById('sections-list');
            if (!sectionsList) return;
            const emptySections = sectionsList.querySelectorAll('div[id^="section-"].hidden');
            emptySections.forEach(section => {
                section.classList.remove('hidden');
                section.classList.add('drag-drop-target');
                sectionsList.appendChild(section);
                const sortableContainer = section.querySelector('.items-sortable');
                if (sortableContainer) {
                    sortableContainer.dataset.dropHint = '↓';
                    if (!sortableContainer._sortableInstance) {
                        this.initSortableForContainer(sortableContainer);
                    }
                }
            });
        },

        hideEmptyDropTargets() {
            const targets = document.querySelectorAll('.drag-drop-target');
            targets.forEach(section => {
                section.classList.remove('drag-drop-target');
                this.updateSectionCounter(section);
            });
        },

        // Drag-and-drop for item reordering
        initSortableForContainer(container) {
            if (typeof Sortable === 'undefined') return;
            if (container._sortableInstance) {
                container._sortableInstance.destroy();
            }
            const sectionId = container.dataset.sectionId;
            // Disable drag when section uses alphabetical sorting
            const sectionEl = container.closest('[data-sort-mode]');
            const sortMode = sectionEl?.dataset?.sortMode || 'manual';
            const isDragDisabled = sortMode !== 'manual';
            // Hide/show drag handles based on sort mode
            const dragHandles = container.querySelectorAll('.drag-handle');
            dragHandles.forEach(h => h.style.display = isDragDisabled ? 'none' : '');

            container._sortableInstance = new Sortable(container, {
                sort: !isDragDisabled,
                animation: 200,
                easing: 'cubic-bezier(0.4, 0, 0.2, 1)',
                handle: '.drag-handle',
                draggable: '[data-item-id]',
                ghostClass: 'sortable-ghost',
                chosenClass: 'sortable-chosen',
                dragClass: 'sortable-drag',
                group: 'items',
                delay: 150,
                delayOnTouchOnly: true,
                touchStartThreshold: 5,
                scroll: true,
                scrollSensitivity: 150,
                scrollSpeed: 15,
                bubbleScroll: true,
                forceFallback: true,
                fallbackOnBody: true,
                fallbackTolerance: 3,
                onStart: (evt) => {
                    if (navigator.vibrate) navigator.vibrate(10);
                    document.body.classList.add('is-dragging');
                    this.showEmptyDropTargets();
                },
                onEnd: async (evt) => {
                    document.body.classList.remove('is-dragging');
                    this.hideEmptyDropTargets();
                    const itemId = evt.item.dataset.itemId;
                    const fromSectionId = evt.from.dataset.sectionId;
                    const toSectionId = evt.to.dataset.sectionId;
                    const newIndex = evt.newIndex;
                    const oldIndex = evt.oldIndex;
                    if (fromSectionId !== toSectionId) {
                        await this.moveItemToSection(itemId, fromSectionId, toSectionId, newIndex);
                        return;
                    }
                    if (newIndex !== oldIndex) {
                        await this.syncItemPosition(itemId, toSectionId, newIndex);
                    }
                }
            });
        },

        initMobileSortable() {
            if (typeof Sortable === 'undefined') {
                console.warn('[App] SortableJS not loaded');
                return;
            }
            const containers = document.querySelectorAll('.items-sortable');
            containers.forEach(container => this.initSortableForContainer(container));
        },

        // Sync item position with server - single request (supports offline)
        async syncItemPosition(itemId, sectionId, newIndex) {
            // Mark as local action to prevent WebSocket race condition
            // Server broadcasts 'item_moved' for /items/:id/move endpoint
            this.markLocalAction('items_reordered');
            this.markLocalAction('item_moved');

            try {
                const url = `/items/${itemId}/move`;
                const options = {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
                    body: `section_id=${encodeURIComponent(sectionId)}&position=${encodeURIComponent(newIndex)}`
                };

                const response = await this.offlineFetch(url, options, 'reorder_item');

                if (response.offline) {
                    if (navigator.vibrate) navigator.vibrate([10, 50, 10]);
                    return;
                }

                if (!response.ok) {
                    console.error('Failed to reorder item');
                    window.Toast?.show(window.t?.('errors.reorder_failed') || 'Failed to reorder item', 'error');
                    if (navigator.onLine) this.refreshSection(parseInt(sectionId));
                    return;
                }

                // Haptic feedback on success
                if (navigator.vibrate) {
                    navigator.vibrate([10, 50, 10]);
                }
            } catch (error) {
                console.error('Failed to sync item position:', error);
                window.Toast?.show(window.t?.('errors.reorder_failed') || 'Failed to reorder item', 'error');
                if (navigator.onLine) this.refreshSection(parseInt(sectionId));
            }
        },

        // Move item to different section via drag-and-drop (supports offline)
        async moveItemToSection(itemId, fromSectionId, toSectionId, targetIndex) {
            this.markLocalAction('item_moved');

            try {
                const url = `/items/${itemId}/move`;
                const options = {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
                    body: `section_id=${encodeURIComponent(toSectionId)}&position=${encodeURIComponent(targetIndex)}`
                };

                const response = await this.offlineFetch(url, options, 'move_item');

                if (response.offline) {
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

                // Replace item with server-rendered HTML (updated dropdown) and update counters
                const html = await response.text();
                const itemEl2 = document.getElementById(`item-${itemId}`);
                if (itemEl2 && html) {
                    Alpine.destroyTree(itemEl2);
                    itemEl2.outerHTML = html.trim();
                }
                if (fromSectionId) {
                    const fromSection = document.getElementById(`section-${fromSectionId}`);
                    if (fromSection) this.updateSectionCounter(fromSection);
                }
                const toSection = document.getElementById(`section-${toSectionId}`);
                if (toSection) {
                    toSection.classList.remove('hidden');
                    this.updateSectionCounter(toSection);
                }
                this.refreshStats();

                const toSectionEl = document.getElementById(`section-${toSectionId}`);
                const toSortMode = toSectionEl?.dataset?.sortMode || 'manual';
                if (toSortMode !== 'manual') {
                    this.$nextTick(() => this.refreshSection(parseInt(toSectionId)));
                }
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
                    // Refresh list and stats without full page reload
                    const appEl = document.querySelector('[x-data="shoppingList()"]');
                    if (appEl && window.Alpine) {
                        const data = Alpine.$data(appEl);
                        if (data) {
                            data.refreshList();
                            data.refreshStats();
                        }
                    }
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
            const sectionId = itemEl ? itemEl.dataset.sectionId : null;

            // Reuse shared offline toggle logic
            const alpineData = Alpine.$data(document.querySelector('[x-data="shoppingList()"]'));
            if (alpineData && sectionId) {
                alpineData._applyOfflineToggle(itemId, sectionId);
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
                // Toggle uncertain styling
                const currentlyUncertain = itemEl.classList.contains('bg-amber-50/50');
                itemEl.classList.toggle('bg-amber-50/50');

                // Update ? icon
                const innerDiv = itemEl.querySelector('.flex.items-center.gap-2');
                if (innerDiv) {
                    const questionMark = innerDiv.querySelector('.text-amber-500.text-xs:not(.offline-sync-badge)');
                    if (!currentlyUncertain && !questionMark) {
                        const span = document.createElement('span');
                        span.className = 'text-amber-500 text-xs';
                        span.textContent = '?';
                        innerDiv.insertBefore(span, innerDiv.firstChild);
                    } else if (currentlyUncertain && questionMark) {
                        questionMark.remove();
                    }
                }

                // Add sync indicator
                itemEl.classList.add('border-l-2', 'border-rose-400');
                if (!itemEl.querySelector('.offline-sync-badge')) {
                    const contentDiv = itemEl.querySelector('.flex-1.min-w-0');
                    const badge = document.createElement('span');
                    badge.className = 'offline-sync-badge inline-flex items-center gap-1 px-2 py-0.5 rounded-full text-xs font-medium bg-rose-100 text-rose-600 ml-2';
                    badge.innerHTML = `<svg class="w-3 h-3 animate-spin" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15"></path></svg>${t('status.syncing')}`;
                    if (contentDiv) contentDiv.after(badge);
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

    // Mark local actions for HTMX-initiated requests (forms, buttons)
    // so WebSocket doesn't trigger redundant refreshes
    document.body.addEventListener('htmx:beforeRequest', function(event) {
        const verb = (event.detail.requestConfig?.verb || '').toLowerCase();
        const path = event.detail.requestConfig?.path || '';
        const appEl = document.querySelector('[x-data]');
        const data = appEl ? Alpine.$data(appEl) : null;
        if (!data?.markLocalAction) return;

        if (verb === 'post' && path === '/items') {
            data.markLocalAction('item_created');
        } else if (verb === 'put' && path.match(/^\/items\/\d+$/)) {
            data.markLocalAction('item_updated');
        } else if (verb === 'delete' && path.match(/^\/items\/\d+$/)) {
            data.markLocalAction('item_deleted');
        } else if (verb === 'post' && path.match(/^\/items\/\d+\/toggle$/)) {
            data.markLocalAction('item_toggled');
        } else if (verb === 'post' && path.match(/^\/items\/\d+\/uncertain$/)) {
            data.markLocalAction('item_updated');
        } else if (verb === 'post' && path === '/sections') {
            data.markLocalAction('section_created');
        }
    });

    document.body.addEventListener('htmx:beforeSwap', function(event) {
        const redirectUrl = event.detail.xhr.getResponseHeader('HX-Redirect');
        if (redirectUrl) {
            window.location.href = redirectUrl;
            event.detail.shouldSwap = false;
        }

        // Capture existing item IDs before swap (for section or full list swaps)
        const targetId = event.detail.target?.id || '';
        if (targetId === 'sections-list' || targetId.startsWith('section-')) {
            const scope = targetId === 'sections-list' ? document : event.detail.target;
            existingItemIds = new Set(
                [...scope.querySelectorAll('[id^="item-"]')].map(el => el.id)
            );
        }
    });

    document.body.addEventListener('htmx:afterSwap', function(event) {
        // Restore correct page title after swap (prevents HTMX from resetting to hardcoded title)
        document.title = window.translations[window.currentLang]?.list?.title || 'Koffan Shopping List';

        // Animate only new items after swap (works for both full list and section swaps)
        const targetId = event.detail.target?.id || '';
        if (targetId === 'sections-list' || targetId.startsWith('section-')) {
            // For outerHTML swaps, event.detail.target may be detached from DOM
            // so look up the fresh element by ID
            const scope = targetId === 'sections-list'
                ? document
                : (document.getElementById(targetId) || event.detail.target);
            scope.querySelectorAll('[id^="item-"]').forEach(el => {
                if (!existingItemIds.has(el.id)) {
                    el.classList.add('item-enter');
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
