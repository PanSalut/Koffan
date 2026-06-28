(function(root, factory) {
    const api = factory();

    if (typeof module === 'object' && module.exports) {
        module.exports = api;
    }

    root.UIScale = api;
})(typeof globalThis !== 'undefined' ? globalThis : this, function() {
    const SCALES = {
        small: '14px',
        normal: '16px',
        large: '18px',
        huge: '20px'
    };
    const DEFAULT_SCALE = 'normal';

    function resolveScale(stored) {
        return Object.prototype.hasOwnProperty.call(SCALES, stored) ? stored : DEFAULT_SCALE;
    }

    function scaleToFontSize(scale) {
        return SCALES[resolveScale(scale)];
    }

    return {
        SCALES,
        DEFAULT_SCALE,
        resolveScale,
        scaleToFontSize
    };
});
