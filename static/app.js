// Shopping List Alpine.js Component
function shoppingList() {
    return {
        // WebSocket
        ws: null,
        connected: false,
        reconnectAttempts: 0,
        maxReconnectAttempts: 5,

        // Modals
        showManageSections: false,
        showAddItem: false,
        showEditModal: false,

        // Section management
        selectMode: false,
        selectedSections: [],

        // Mobile helper ('button' or 'progress')
        mobileHelper: window.initialPreferences?.mobileHelper || 'button',

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

        init() {
            this.initWebSocket();

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
        },

        initWebSocket() {
            this.connect();

            document.addEventListener('visibilitychange', () => {
                if (document.visibilityState === 'visible' && !this.connected) {
                    this.connect();
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
                        // Sekcje się zmieniły - przeładuj całą stronę aby odświeżyć selecty
                        window.location.reload();
                        break;
                    case 'item_created':
                    case 'item_deleted':
                    case 'item_moved':
                    case 'items_reordered':
                        // Wymaga pełnego odświeżenia listy
                        this.refreshList();
                        this.refreshStats();
                        break;
                    case 'item_toggled':
                    case 'item_updated':
                        // Odśwież listę i stats dla wszystkich klientów (włącznie z remote)
                        this.refreshList();
                        this.refreshStats();
                        break;
                    case 'preferences_updated':
                        if (message.data && message.data.mobile_helper) {
                            this.mobileHelper = message.data.mobile_helper;
                        }
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
            const sectionsList = document.getElementById('sections-list');
            if (sectionsList) {
                htmx.ajax('GET', '/', {
                    target: '#sections-list',
                    swap: 'innerHTML',
                    select: '#sections-list > *'
                });
            }

            const manageSectionsList = document.getElementById('manage-sections-list');
            if (manageSectionsList) {
                htmx.ajax('GET', '/sections/list', {
                    target: '#manage-sections-list',
                    swap: 'innerHTML'
                });
            }
        },

        async refreshStats() {
            try {
                const response = await fetch('/stats');
                if (response.ok) {
                    const data = await response.json();
                    // JSON używa snake_case
                    this.stats = {
                        total: data.total_items || 0,
                        completed: data.completed_items || 0,
                        percentage: data.percentage || 0
                    };
                }
            } catch (error) {
                console.error('Failed to refresh stats:', error);
            }
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

            const confirmed = confirm(`Usunac ${this.selectedSections.length} wybranych sekcji?`);
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
                    // Przeładuj stronę aby odświeżyć selecty
                    window.location.reload();
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
            try {
                const response = await fetch(`/items/${this.mobileActionItem.id}/uncertain`, {
                    method: 'POST'
                });
                if (response.ok) {
                    this.mobileActionItem.uncertain = !this.mobileActionItem.uncertain;
                    this.refreshList();
                }
            } catch (error) {
                console.error('Failed to toggle uncertain:', error);
            }
        },

        async moveToSection(sectionId) {
            if (!this.mobileActionItem) return;
            try {
                const response = await fetch(`/items/${this.mobileActionItem.id}/move`, {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
                    body: `section_id=${sectionId}`
                });
                if (response.ok) {
                    this.mobileActionItem = null;
                    this.refreshList();
                }
            } catch (error) {
                console.error('Failed to move item:', error);
            }
        },

        async deleteItem() {
            if (!this.mobileActionItem) return;
            const confirmed = confirm(`Usunac "${this.mobileActionItem.name}"?`);
            if (!confirmed) return;

            try {
                const response = await fetch(`/items/${this.mobileActionItem.id}`, {
                    method: 'DELETE'
                });
                if (response.ok) {
                    this.mobileActionItem = null;
                    this.refreshList();
                    this.refreshStats();
                }
            } catch (error) {
                console.error('Failed to delete item:', error);
            }
        },

        // Edit Item
        editItem(item) {
            this.editingItem = item;
            this.editItemName = item.name;
            this.editItemDescription = item.description || '';
            this.$nextTick(() => {
                const input = document.querySelector('[x-model="editItemName"]');
                if (input) input.focus();
            });
        },

        async submitEditItem() {
            if (!this.editItemName.trim() || !this.editingItem) return;

            try {
                const response = await fetch(`/items/${this.editingItem.id}`, {
                    method: 'PUT',
                    headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
                    body: `name=${encodeURIComponent(this.editItemName.trim())}&description=${encodeURIComponent(this.editItemDescription.trim())}`
                });
                if (response.ok) {
                    this.editingItem = null;
                    this.editItemName = '';
                    this.editItemDescription = '';
                    this.refreshList();
                }
            } catch (error) {
                console.error('Failed to save edit:', error);
            }
        },

        // Mobile Helper Toggle
        async toggleMobileHelper() {
            try {
                const response = await fetch('/preferences/toggle-mobile-helper', {
                    method: 'POST'
                });
                if (response.ok) {
                    const data = await response.json();
                    this.mobileHelper = data.mobile_helper;
                }
            } catch (error) {
                console.error('Failed to toggle mobile helper:', error);
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

    // Po dodaniu sekcji - przeładuj stronę
    document.body.addEventListener('htmx:afterRequest', function(event) {
        if (event.detail.pathInfo.requestPath === '/sections' && event.detail.successful) {
            setTimeout(() => window.location.reload(), 100);
        }
    });
});
