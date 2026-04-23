(function(root, factory) {
    const api = factory();

    if (typeof module === 'object' && module.exports) {
        module.exports = api;
    }

    root.ViewportHeight = api;
})(typeof globalThis !== 'undefined' ? globalThis : this, function() {
    function getVisibleViewportHeight(win) {
        const visualHeight = win?.visualViewport?.height;
        if (typeof visualHeight === 'number' && visualHeight > 0) {
            return visualHeight;
        }

        const innerHeight = win?.innerHeight;
        if (typeof innerHeight === 'number' && innerHeight > 0) {
            return innerHeight;
        }

        return 0;
    }

    function syncViewportHeight(win, doc) {
        const height = Math.round(getVisibleViewportHeight(win));
        if (!height || !doc?.documentElement?.style?.setProperty) {
            return height;
        }

        doc.documentElement.style.setProperty('--app-viewport-height', `${height}px`);
        return height;
    }

    function initViewportHeightSync(win, doc) {
        if (!win || !doc) {
            return function noop() {};
        }

        const sync = () => syncViewportHeight(win, doc);
        const removeListeners = [];

        const register = (target, eventName) => {
            if (!target?.addEventListener) return;
            target.addEventListener(eventName, sync, { passive: true });
            removeListeners.push(() => target.removeEventListener(eventName, sync));
        };

        sync();
        register(win, 'resize');
        register(win, 'orientationchange');
        register(win.visualViewport, 'resize');
        register(win.visualViewport, 'scroll');

        return function cleanup() {
            removeListeners.forEach(removeListener => removeListener());
        };
    }

    return {
        getVisibleViewportHeight,
        syncViewportHeight,
        initViewportHeightSync
    };
});
