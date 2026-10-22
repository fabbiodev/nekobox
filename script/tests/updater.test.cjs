const { readFileSync } = require('node:fs');
const { runInNewContext } = require('node:vm');
const assert = require('node:assert/strict');
const source = readFileSync('res/public/check_new_release.js', 'utf8');
const digest = 'sha256:' + 'a'.repeat(64);

function check({ hash = digest, verified = false, corrupt = false, downloadError = '', response } = {}) {
    const calls = [];
    let valid = verified;
    const release = { tag_name: '5.11.28.5', assets: [{ name: 'nekobox-5.11.28.5-windows64-installer.exe',
        digest: hash, browser_download_url: 'https://example.invalid/update.exe' }], body: 'test' };
    const context = {
        configs: {}, env: {}, GlobalMap: {}, APPLICATION_DIR_PATH: '/app',
        NKR_VERSION: '5.11.28.4', NKR_SOFTWARE_NAME: 'NekoBox', ButtonClicked: true,
        UpdaterExists: true, search: 'windows64',
        HTTPResponse: function () { return { error: '', text: response ?? JSON.stringify([release]) }; },
        log() {}, info() {}, warning(message) { calls.push(['warning', message]); },
        translate: x => x, file_exists: () => false, curdir_path: x => '/app/' + x,
        ask: () => 2, open_url() {},
        window: { verify_file: () => valid },
        download() { calls.push(['download']); valid = !corrupt; return downloadError; },
    };
    // First dialog chooses Update; confirmation chooses Yes.
    let asks = 0;
    context.ask = () => asks++ === 0 ? 2 : 1;
    runInNewContext(source, context);
    return { context, calls };
}

let result = check();
assert.equal(result.context.is_newer, true);
assert.equal(result.context.options.sha256, 'a'.repeat(64));
assert.equal(result.calls.filter(x => x[0] === 'download').length, 1);
result = check({ verified: true });
assert.equal(result.context.is_newer, true);
assert.equal(result.calls.filter(x => x[0] === 'download').length, 0);
for (const options of [{ hash: '' }, { hash: 'bad' }, { corrupt: true }, { downloadError: 'HTTP 404' }, { response: '{}' }, { response: 'not JSON' }]) {
    result = check(options);
    assert.equal(result.context.is_newer, false, JSON.stringify(options));
}
const version = check({ verified: true }).context;
assert.equal(version.isNewerVersion('5.11.28.4', 'v5.11.28.5'), true);
assert.equal(version.isNewerVersion('5.11.28.5', '5.11.28.4'), false);
assert.equal(version.isNewerVersion('5.11.28.4', 'garbage'), false);
assert.equal(version.isNewerVersion('5.11.28-beta.1', '5.11.28'), true);
console.log('Updater version selection, digest validation and failure paths passed');
