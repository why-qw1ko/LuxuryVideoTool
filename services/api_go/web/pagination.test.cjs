const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const test = require('node:test');
const vm = require('node:vm');

const source = fs.readFileSync(path.join(__dirname, 'static/app.js'), 'utf8');
const start = source.indexOf('function renderPager(');
const end = source.indexOf('/* ---------- 后台仪表盘 ---------- */', start);

function harness() {
  const document = {body: {matches() { return false; }}};
  document.activeElement = document.body;
  const current = {matches() { return false; }, focus() { document.activeElement = this; }};
  const el = {
    dataset: {}, attributes: {}, renders: 0,
    contains(node) { return node === current; },
    querySelector() { return current; },
    setAttribute(key, value) { this.attributes[key] = value; },
    set innerHTML(markup) {
      this.markup = markup;
      this.firstElementChild = {};
      this.renders++;
    },
    replaceChildren() {
      this.markup = '';
      this.firstElementChild = null;
      document.activeElement = document.body;
    },
  };
  const context = vm.createContext({document, icon: name => name});
  vm.runInContext(source.slice(start, end), context);
  const render = (...args) => context.renderPager(el, ...args);
  return {render, el, current, document};
}

test('分页展示有限的相邻页码和首尾页，跳页校验范围并保留翻页后的焦点', () => {
  const {render, el, current, document} = harness(), targets = [];
  render(50, 100, page => targets.push(page));
  const pageNumbers = [...el.markup.matchAll(/data-pager="(\d+)" aria-label="第 \d+ 页"/g)].map(match => Number(match[1]));
  assert.deepEqual(pageNumbers, [1, 49, 50, 51, 100]);
  assert.equal(el.markup.match(/pager-ellipsis/g).length, 2);
  assert.match(el.markup, /data-pager="50"[^>]+aria-current="page"/);
  assert.match(el.markup, /data-pager-jump[^>]+max="100"/);

  function jumpTo(valueAsNumber) {
    el.onsubmit({target: {matches: () => true, querySelector: () => ({valueAsNumber})}, preventDefault() {}});
  }
  jumpTo(101);
  jumpTo(50.5);
  jumpTo(50);
  assert.deepEqual(targets, []);

  document.activeElement = current;
  render(50, 100, page => targets.push(page));
  assert.equal(el.renders, 1);
  jumpTo(100);
  assert.deepEqual(targets, [100]);
  el.replaceChildren();
  render(100, 100, page => targets.push(page));
  assert.match(el.markup, /aria-label="下一页"[^>]+disabled/);
  assert.equal(document.activeElement, current);
});

test('可调整每页条数的分页在单页保留控件并支持原有 25 条设置', () => {
  const {render, el} = harness(), sizes = [];
  render(1, 1, () => {});
  assert.equal(el.markup, '');

  render(1, 1, () => {}, {pageSize: 25, onPageSizeChange: size => sizes.push(size), label: 'CDK 分页'});
  assert.equal(el.attributes['aria-label'], 'CDK 分页');
  assert.match(el.markup, /data-pager-size/);
  assert.match(el.markup, /<option value="25" selected>25 条\/页<\/option>/);
  assert.match(el.markup, /aria-label="上一页"[^>]+disabled/);
  assert.doesNotMatch(el.markup, /data-pager-jump/);
  for (const value of ['25', '99', '50']) {
    const target = {matches: () => true, value};
    el.onchange({target});
    if (value === '50') assert.equal(target.value, '25');
  }
  assert.deepEqual(sizes, [50]);
});
