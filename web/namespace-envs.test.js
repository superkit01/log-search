const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const vm = require('node:vm');
const path = require('node:path');

test('namespace environments preserve valid selection and fall back for saved or shared values', () => {
  let namespace = 'default';
  const tabs = [
    ['default', 'dev'], ['default', 'test'], ['ems', 'prod'], ['ems', 'test'],
  ].map(([ns, value]) => {
    const classes = new Set();
    return {
      dataset: { namespace: ns, value }, hidden: false, classes,
      classList: { add: value => classes.add(value), remove: value => classes.delete(value) },
    };
  });
  const source = fs.readFileSync(path.join(__dirname, 'static/app.js'), 'utf8');
  const start = source.indexOf('function refreshNamespaceEnvs(');
  const end = source.indexOf('\nlet currentLines', start);
  const context = vm.createContext({
    $: () => ({}),
    activeTabValue: id => id === 'namespace-tabs' ? namespace :
      (tabs.find(tab => tab.classes.has('active'))?.dataset.value || ''),
    document: { querySelectorAll: () => tabs },
  });
  vm.runInContext(source.slice(start, end), context);
  const selected = () => tabs.filter(tab => tab.classes.has('active')).map(tab => [tab.dataset.namespace, tab.dataset.value]);
  context.refreshNamespaceEnvs('test');
  assert.deepEqual(selected(), [['default', 'test']]);
  namespace = 'ems';
  context.refreshNamespaceEnvs();
  assert.deepEqual(selected(), [['ems', 'test']]);
  context.refreshNamespaceEnvs('dev');
  assert.deepEqual(selected(), [['ems', 'prod']]);
  assert.ok(tabs.filter(tab => tab.dataset.namespace === 'default').every(tab => tab.hidden));
});
