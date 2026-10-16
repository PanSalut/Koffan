// Koffan Offline Storage - IndexedDB wrapper
class OfflineStorage {
    constructor() {
        this.dbName = 'koffan-offline';
        this.dbVersion = 5;  // Must be >= existing version in browser
        this.db = null;
        this.initPromise = null;
    }

    async init() {
        if (this.db) return this.db;
        if (this.initPromise) return this.initPromise;

        this.initPromise = new Promise((resolve, reject) => {
            const request = indexedDB.open(this.dbName, this.dbVersion);

            request.onerror = () => {
                console.error('[OfflineStorage] Failed to open database:', request.error);
                reject(request.error);
            };

            request.onsuccess = () => {
                this.db = request.result;
                const db = this.db;
                const resetConnection = () => {
                    if (this.db === db) this.db = null;
                };
                db.onversionchange = () => {
                    db.close();
                    resetConnection();
                };
                db.onclose = resetConnection;
                console.log('[OfflineStorage] Database opened successfully');
                resolve(this.db);
            };

            request.onupgradeneeded = (event) => {
                const db = event.target.result;
                console.log('[OfflineStorage] Upgrading database...');

                // Sections cache store
                if (!db.objectStoreNames.contains('sections')) {
                    const sectionsStore = db.createObjectStore('sections', { keyPath: 'id' });
                    sectionsStore.createIndex('sort_order', 'sort_order');
                }

                // Offline queue store
                if (!db.objectStoreNames.contains('offline_queue')) {
                    const queueStore = db.createObjectStore('offline_queue', {
                        keyPath: 'id',
                        autoIncrement: true
                    });
                    queueStore.createIndex('timestamp', 'timestamp');
                }

                // Sync metadata store
                if (!db.objectStoreNames.contains('sync_metadata')) {
                    db.createObjectStore('sync_metadata', { keyPath: 'key' });
                }

                // Suggestions store for auto-completion
                if (!db.objectStoreNames.contains('suggestions')) {
                    const suggestionsStore = db.createObjectStore('suggestions', { keyPath: 'name' });
                    suggestionsStore.createIndex('usage_count', 'usage_count');
                }
            };
        }).finally(() => {
            this.initPromise = null;
        });
        return this.initPromise;
    }

    // ===== OFFLINE QUEUE METHODS =====

    async queueAction(action) {
        if (!this.db) await this.init();

        return new Promise((resolve, reject) => {
            const tx = this.db.transaction('offline_queue', 'readwrite');
            const store = tx.objectStore('offline_queue');

            const request = store.add({
                ...action,
                // Preserve the time of the tap when persistence waits for startup.
                timestamp: action.timestamp ?? Math.floor(Date.now() / 1000)
            });

            tx.oncomplete = () => {
                console.log('[OfflineStorage] Action queued:', action.type);
                resolve(request.result);
            };
            request.onerror = () => reject(request.error);
            tx.onabort = () => reject(tx.error || new Error('Offline queue write aborted'));
        });
    }

    async getQueuedActions() {
        if (!this.db) await this.init();

        return new Promise((resolve, reject) => {
            const tx = this.db.transaction('offline_queue', 'readonly');
            const store = tx.objectStore('offline_queue');
            const index = store.index('timestamp');

            const request = index.getAll();
            request.onsuccess = () => resolve(request.result || []);
            request.onerror = () => reject(request.error);
        });
    }

    async clearAction(id) {
        if (!this.db) await this.init();

        return new Promise((resolve, reject) => {
            const tx = this.db.transaction('offline_queue', 'readwrite');
            const store = tx.objectStore('offline_queue');

            const request = store.delete(id);
            tx.oncomplete = () => resolve();
            request.onerror = () => reject(request.error);
            tx.onabort = () => reject(tx.error || new Error('Offline queue deletion aborted'));
        });
    }

    async clearAllActions() {
        if (!this.db) await this.init();

        return new Promise((resolve, reject) => {
            const tx = this.db.transaction('offline_queue', 'readwrite');
            const store = tx.objectStore('offline_queue');

            const request = store.clear();
            tx.oncomplete = () => resolve();
            request.onerror = () => reject(request.error);
            tx.onabort = () => reject(tx.error || new Error('Offline queue clear aborted'));
        });
    }

    async getQueueLength() {
        if (!this.db) await this.init();

        return new Promise((resolve, reject) => {
            const tx = this.db.transaction('offline_queue', 'readonly');
            const store = tx.objectStore('offline_queue');

            const request = store.count();
            request.onsuccess = () => resolve(request.result);
            request.onerror = () => reject(request.error);
        });
    }

    // ===== SECTIONS CACHE METHODS =====

    async saveSections(sections, listId = null) {
        if (!this.db) await this.init();

        return new Promise((resolve, reject) => {
            const tx = this.db.transaction('sections', 'readwrite');
            const store = tx.objectStore('sections');

            if (listId === null || listId === undefined) {
                store.clear();
                for (const section of sections) store.add(section);
            } else {
                // The API returns one list at a time. Replace that list inside a
                // single transaction without evicting other lists saved offline.
                const request = store.getAll();
                request.onsuccess = () => {
                    for (const section of request.result || []) {
                        if (String(section.list_id) === String(listId)) store.delete(section.id);
                    }
                    for (const section of sections) store.put({ ...section, list_id: listId });
                };
            }

            tx.oncomplete = () => {
                console.log('[OfflineStorage] Sections cached:', sections.length);
                resolve();
            };
            tx.onerror = () => reject(tx.error);
            tx.onabort = () => reject(tx.error || new Error('Offline cache write aborted'));
        });
    }

    async getSections() {
        if (!this.db) await this.init();

        return new Promise((resolve, reject) => {
            const tx = this.db.transaction('sections', 'readonly');
            const store = tx.objectStore('sections');
            const index = store.index('sort_order');

            const request = index.getAll();
            request.onsuccess = () => resolve(request.result || []);
            request.onerror = () => reject(request.error);
        });
    }

    // ===== SYNC METADATA METHODS =====

    async setMetadata(key, value) {
        if (!this.db) await this.init();

        return new Promise((resolve, reject) => {
            const tx = this.db.transaction('sync_metadata', 'readwrite');
            const store = tx.objectStore('sync_metadata');

            const request = store.put({ key, value });
            tx.oncomplete = () => resolve();
            request.onerror = () => reject(request.error);
            tx.onabort = () => reject(tx.error || new Error('Offline metadata write aborted'));
        });
    }

    // Stores `value` only while the record under `key` still has the revision
    // the caller read (`expectedRevision`, undefined when there was none).
    // Resolves to whether it was written. One transaction, so it is atomic
    // across tabs.
    async compareAndSetMetadata(key, expectedRevision, value) {
        if (!this.db) await this.init();

        return new Promise((resolve, reject) => {
            const tx = this.db.transaction('sync_metadata', 'readwrite');
            const store = tx.objectStore('sync_metadata');
            let written = false;

            const request = store.get(key);
            request.onsuccess = () => {
                if (request.result?.value?.revision === expectedRevision) {
                    store.put({ key, value });
                    written = true;
                }
            };
            request.onerror = () => reject(request.error);
            tx.oncomplete = () => resolve(written);
            tx.onabort = () => reject(tx.error || new Error('Offline metadata write aborted'));
        });
    }

    async getMetadata(key) {
        if (!this.db) await this.init();

        return new Promise((resolve, reject) => {
            const tx = this.db.transaction('sync_metadata', 'readonly');
            const store = tx.objectStore('sync_metadata');

            const request = store.get(key);
            request.onsuccess = () => resolve(request.result?.value);
            request.onerror = () => reject(request.error);
        });
    }

    async getLastSyncTimestamp() {
        return this.getMetadata('last_sync');
    }

    async setLastSyncTimestamp(timestamp) {
        return this.setMetadata('last_sync', timestamp);
    }

    // ===== SUGGESTIONS CACHE METHODS =====

    async saveSuggestions(suggestions) {
        if (!this.db) await this.init();

        return new Promise((resolve, reject) => {
            const tx = this.db.transaction('suggestions', 'readwrite');
            const store = tx.objectStore('suggestions');

            // Clear existing data
            store.clear();

            // Add new data
            for (const suggestion of suggestions) {
                store.add(suggestion);
            }

            tx.oncomplete = () => {
                console.log('[OfflineStorage] Suggestions cached:', suggestions.length);
                resolve();
            };
            tx.onerror = () => reject(tx.error);
            tx.onabort = () => reject(tx.error || new Error('Offline suggestions write aborted'));
        });
    }

    // Calculate Levenshtein distance between two strings
    _levenshteinDistance(s1, s2) {
        s1 = s1.toLowerCase();
        s2 = s2.toLowerCase();

        if (s1.length === 0) return s2.length;
        if (s2.length === 0) return s1.length;

        const matrix = [];

        // Initialize matrix
        for (let i = 0; i <= s1.length; i++) {
            matrix[i] = [i];
        }
        for (let j = 0; j <= s2.length; j++) {
            matrix[0][j] = j;
        }

        // Fill matrix
        for (let i = 1; i <= s1.length; i++) {
            for (let j = 1; j <= s2.length; j++) {
                const cost = s1[i - 1] === s2[j - 1] ? 0 : 1;
                matrix[i][j] = Math.min(
                    matrix[i - 1][j] + 1,      // deletion
                    matrix[i][j - 1] + 1,      // insertion
                    matrix[i - 1][j - 1] + cost // substitution
                );
            }
        }

        return matrix[s1.length][s2.length];
    }

    // Score a suggestion match (higher is better)
    _scoreSuggestion(name, query) {
        const nameLower = name.toLowerCase();
        const queryLower = query.toLowerCase();

        // Exact match: highest score
        if (nameLower === queryLower) {
            return 1000;
        }

        // Prefix match: high score
        if (nameLower.startsWith(queryLower)) {
            return 500;
        }

        // Contains match: medium score
        if (nameLower.includes(queryLower)) {
            return 200;
        }

        // Fuzzy match: score based on Levenshtein distance
        if (query.length >= 3) {
            const distance = this._levenshteinDistance(nameLower, queryLower);
            const maxDistance = Math.floor(query.length / 2); // Allow ~50% typos

            if (distance <= maxDistance) {
                return 100 - distance * 20;
            }

            // Check if any word in the name fuzzy matches
            const words = nameLower.split(/\s+/);
            for (const word of words) {
                const wordDist = this._levenshteinDistance(word, queryLower);
                if (wordDist <= maxDistance) {
                    return 80 - wordDist * 15;
                }
            }
        }

        return 0; // No match
    }

    async getSuggestions(query) {
        if (!this.db) await this.init();

        return new Promise((resolve, reject) => {
            const tx = this.db.transaction('suggestions', 'readonly');
            const store = tx.objectStore('suggestions');

            const request = store.getAll();
            request.onsuccess = () => {
                let results = request.result || [];

                // Score and filter suggestions
                if (query && query.length > 0) {
                    const scored = results
                        .map(s => ({
                            suggestion: s,
                            score: this._scoreSuggestion(s.name, query) + Math.floor(s.usage_count / 10)
                        }))
                        .filter(item => item.score > 0);

                    // Sort by score descending, then by usage_count descending
                    scored.sort((a, b) => {
                        if (a.score !== b.score) return b.score - a.score;
                        return b.suggestion.usage_count - a.suggestion.usage_count;
                    });

                    results = scored.slice(0, 10).map(item => item.suggestion);
                } else {
                    // No query - just sort by usage_count
                    results.sort((a, b) => b.usage_count - a.usage_count);
                    results = results.slice(0, 10);
                }

                resolve(results);
            };
            request.onerror = () => reject(request.error);
        });
    }

    // ===== OPTIMISTIC UPDATES =====

    async updateItemInCache(itemId, updates) {
        return this.modifyCachedItem(itemId, (items, index) => {
            items[index] = { ...items[index], ...updates };
        });
    }

    async removeItemFromCache(itemId) {
        return this.modifyCachedItem(itemId, (items, index) => items.splice(index, 1));
    }

    async modifyCachedItem(itemId, modify) {
        if (!this.db) await this.init();

        // Keep the read and write in one transaction so simultaneous taps cannot
        // overwrite each other's cached state with an earlier sections snapshot.
        return new Promise((resolve, reject) => {
            const tx = this.db.transaction('sections', 'readwrite');
            const store = tx.objectStore('sections');
            const request = store.getAll();
            let modified = false;
            request.onsuccess = () => {
                for (const section of request.result || []) {
                    const index = section.items?.findIndex(item => item.id === itemId) ?? -1;
                    if (index !== -1) {
                        modify(section.items, index);
                        store.put(section);
                        modified = true;
                        break;
                    }
                }
            };
            tx.oncomplete = () => resolve(modified);
            tx.onerror = () => reject(tx.error);
            tx.onabort = () => reject(tx.error || new Error('Offline item update aborted'));
        });
    }
}

// Create global instance
window.offlineStorage = new OfflineStorage();
