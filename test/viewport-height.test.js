const test = require('node:test');
const assert = require('node:assert/strict');

const {
    getVisibleViewportHeight,
    syncViewportHeight
} = require('../static/viewport.js');

test('getVisibleViewportHeight prefers visualViewport height when available', () => {
    const win = {
        innerHeight: 780,
        visualViewport: {
            height: 512
        }
    };

    assert.equal(getVisibleViewportHeight(win), 512);
});

test('getVisibleViewportHeight falls back to innerHeight when visualViewport is unavailable', () => {
    const win = {
        innerHeight: 780
    };

    assert.equal(getVisibleViewportHeight(win), 780);
});

test('syncViewportHeight writes the viewport css variable in pixels', () => {
    const doc = {
        documentElement: {
            style: {
                values: {},
                setProperty(name, value) {
                    this.values[name] = value;
                }
            }
        }
    };

    const win = {
        innerHeight: 900,
        visualViewport: {
            height: 640
        }
    };

    const height = syncViewportHeight(win, doc);

    assert.equal(height, 640);
    assert.equal(
        doc.documentElement.style.values['--app-viewport-height'],
        '640px'
    );
});
