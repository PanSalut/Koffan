const test = require('node:test');
const assert = require('node:assert/strict');

const { resolveScale, scaleToFontSize, DEFAULT_SCALE } = require('../static/ui-scale.js');

test('resolveScale returns the stored value when it is a known preset', () => {
    assert.equal(resolveScale('small'), 'small');
    assert.equal(resolveScale('normal'), 'normal');
    assert.equal(resolveScale('large'), 'large');
    assert.equal(resolveScale('huge'), 'huge');
});

test('resolveScale falls back to the default for unknown or missing values', () => {
    assert.equal(resolveScale('gigantic'), DEFAULT_SCALE);
    assert.equal(resolveScale(null), DEFAULT_SCALE);
    assert.equal(resolveScale(undefined), DEFAULT_SCALE);
});

test('DEFAULT_SCALE is normal', () => {
    assert.equal(DEFAULT_SCALE, 'normal');
});

test('scaleToFontSize maps each preset to its pixel size', () => {
    assert.equal(scaleToFontSize('small'), '14px');
    assert.equal(scaleToFontSize('normal'), '16px');
    assert.equal(scaleToFontSize('large'), '18px');
    assert.equal(scaleToFontSize('huge'), '20px');
});

test('scaleToFontSize falls back to the normal pixel size for unknown values', () => {
    assert.equal(scaleToFontSize('bogus'), '16px');
});
