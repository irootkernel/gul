import assert from 'node:assert/strict';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import {fileURLToPath} from 'node:url';
import {validateRepository} from './check-sot.mjs';

const scriptDir = path.dirname(fileURLToPath(import.meta.url));
const sourceRoot = path.resolve(scriptDir, '..');
const historicalRoadmap = `${fs.readFileSync(path.join(sourceRoot, 'docs', 'roadmap.md'), 'utf8')}
Historical allocations: E0-T1..T3, E2-T4..T5, E5-T4, E6-T4, E7-T4..T7,
E9-T4, E10-T1..T5, and E11-T1..T3.`;
const baselineRegistry = JSON.parse(fs.readFileSync(path.join(sourceRoot, 'docs', 'task-identities.json'), 'utf8'));

function fixture() {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), 'gul-sot-'));
  fs.cpSync(path.join(sourceRoot, 'docs'), path.join(root, 'docs'), {recursive: true});
  return root;
}

function write(root, relativePath, transform) {
  const target = path.join(root, relativePath);
  fs.writeFileSync(target, transform(fs.readFileSync(target, 'utf8')));
}

function run(root, baseline = baselineRegistry) {
  return validateRepository(root, {
    gitEvidence: {mode: 'git', baselineRegistry: baseline, historicalRoadmaps: [historicalRoadmap]},
  });
}

function expectFailure(name, mutate, pattern, baseline = baselineRegistry) {
  const root = fixture();
  try {
    mutate(root);
    const result = run(root, baseline);
    assert(result.errors.some(error => pattern.test(error)), `${name}: expected ${pattern}, got:\n${result.errors.join('\n')}`);
  } finally {
    fs.rmSync(root, {recursive: true, force: true});
  }
}

{
  const root = fixture();
  try {
    assert.deepEqual(run(root).errors, []);
  } finally {
    fs.rmSync(root, {recursive: true, force: true});
  }
}

{
  const root = fixture();
  try {
    const result = validateRepository(root);
    assert.deepEqual(result.errors, []);
    assert.equal(result.warnings.length, 1);
    assert.match(result.warnings[0], /historical Task ID nondeletion was not verified/);
    assert.match(result.summary, /historical nondeletion unverified/);
  } finally {
    fs.rmSync(root, {recursive: true, force: true});
  }
}

function gitValidationFixture(runGit) {
  const root = fixture();
  fs.mkdirSync(path.join(root, '.git'));
  try {
    return validateRepository(root, {gitRunner: runGit});
  } finally {
    fs.rmSync(root, {recursive: true, force: true});
  }
}

{
  const result = gitValidationFixture(args => {
    if (args[0] === 'ls-tree') return '';
    if (args[0] === 'rev-list') return 'roadmap-revision\n';
    if (args[0] === 'show') return historicalRoadmap;
    throw new Error(`Unexpected Git command: ${args.join(' ')}`);
  });
  assert.deepEqual(result.errors, []);
  assert.match(result.summary, /committed identity history verified/);
}

{
  const result = gitValidationFixture(args => {
    if (args[0] === 'ls-tree') return '100644 blob deadbeef\tdocs/task-identities.json\n';
    if (args[0] === 'show' && args[1] === 'HEAD:docs/task-identities.json') return '{malformed';
    if (args[0] === 'rev-list') return 'roadmap-revision\n';
    if (args[0] === 'show') return historicalRoadmap;
    throw new Error(`Unexpected Git command: ${args.join(' ')}`);
  });
  assert(result.errors.some(error => /malformed JSON/.test(error)));
}

{
  const result = gitValidationFixture(args => {
    if (args[0] === 'ls-tree') return '100644 blob deadbeef\tdocs/task-identities.json\n';
    if (args[0] === 'show' && args[1] === 'HEAD:docs/task-identities.json') throw new Error('unreadable');
    if (args[0] === 'rev-list') return 'roadmap-revision\n';
    if (args[0] === 'show') return historicalRoadmap;
    throw new Error(`Unexpected Git command: ${args.join(' ')}`);
  });
  assert(result.errors.includes('Cannot read the existing committed Task identity registry through Git'));
}

{
  const result = gitValidationFixture(args => {
    if (args[0] === 'ls-tree') return '';
    if (args[0] === 'rev-list') throw new Error('history unavailable');
    throw new Error(`Unexpected Git command: ${args.join(' ')}`);
  });
  assert(result.errors.includes('Cannot read committed roadmap history through Git'));
}

{
  const result = gitValidationFixture(args => {
    if (args[0] === 'ls-tree') return '';
    if (args[0] === 'rev-list') return 'roadmap-revision\n';
    if (args[0] === 'show') return '# malformed roadmap history without identities\n';
    throw new Error(`Unexpected Git command: ${args.join(' ')}`);
  });
  assert(result.errors.includes('Committed roadmap history contains no Task identities'));
}

{
  const root = fixture();
  try {
    write(root, 'docs/roadmap.md', text => text
      .replace('| Active Task | None |', '| Active Task | E12-T1 |')
      .replace('| E12-T1 | Pre-release | Planned |', '| E12-T1 | Pre-release | In Review |'));
    assert.deepEqual(run(root).errors, []);
  } finally {
    fs.rmSync(root, {recursive: true, force: true});
  }
}

expectFailure('duplicate requirement is not normalized away', root => {
  write(root, 'docs/required-specs.md', text => text.replace(
    /^\| REQ-API-007 \|.*$/m, match => `${match}\n${match}`,
  ));
}, /Missing or duplicate active requirement definitions/);

expectFailure('multiple active rows', root => {
  write(root, 'docs/roadmap.md', text => text
    .replace('| Active Task | None |', '| Active Task | E12-T1 |')
    .replace('| E12-T1 | Pre-release | Planned |', '| E12-T1 | Pre-release | In Progress |')
    .replace('| E1-T1 | Pre-release | Planned |', '| E1-T1 | Pre-release | In Review |'));
}, /Multiple active Tasks/);

expectFailure('active header mismatch', root => {
  write(root, 'docs/roadmap.md', text => text.replace('| Active Task | None |', '| Active Task | E12-T1 |'));
}, /Active Task header must be exactly None/);

expectFailure('active row mismatch', root => {
  write(root, 'docs/roadmap.md', text => text.replace(
    '| E12-T1 | Pre-release | Planned |',
    '| E12-T1 | Pre-release | In Progress |',
  ));
}, /Active Task header must be exactly E12-T1/);

expectFailure('missing retired row', root => {
  write(root, 'docs/roadmap.md', text => text.replace(/^\| E0-T5 \|.*\n/m, ''));
}, /Required permanent Task row is missing: E0-T5/);

expectFailure('row and registry deletion', root => {
  write(root, 'docs/roadmap.md', text => text.replace(/^\| E0-T5 \|.*\n/m, ''));
  const target = path.join(root, 'docs', 'task-identities.json');
  const registry = JSON.parse(fs.readFileSync(target, 'utf8'));
  registry.identities = registry.identities.filter(entry => entry.id !== 'E0-T5');
  fs.writeFileSync(target, `${JSON.stringify(registry, null, 2)}\n`);
}, /Committed Task identity was deleted: E0-T5/);

expectFailure('first adoption historical deletion', root => {
  const target = path.join(root, 'docs', 'task-identities.json');
  const registry = JSON.parse(fs.readFileSync(target, 'utf8'));
  registry.identities = registry.identities.filter(entry => entry.id !== 'E10-T5');
  fs.writeFileSync(target, `${JSON.stringify(registry, null, 2)}\n`);
}, /Historical Task identity is not registered: E10-T5/, null);

expectFailure('missing registered ID', root => {
  const target = path.join(root, 'docs', 'task-identities.json');
  const registry = JSON.parse(fs.readFileSync(target, 'utf8'));
  registry.identities = registry.identities.filter(entry => entry.id !== 'E12-T3');
  fs.writeFileSync(target, `${JSON.stringify(registry, null, 2)}\n`);
}, /Roadmap Task identity is not registered: E12-T3/);

expectFailure('dependency cycle', root => {
  write(root, 'docs/roadmap.md', text => text.replace(
    '| E1-T1 | Pre-release | Planned | E12-T1 |',
    '| E1-T1 | Pre-release | Planned | E1-T2 |',
  ));
}, /Task dependency cycle at E1-T1|Task dependency cycle at E1-T2/);

expectFailure('retired dependency', root => {
  write(root, 'docs/roadmap.md', text => text.replace(
    '| E12-T1 | Pre-release | Planned | E0-T7; Contract ready |',
    '| E12-T1 | Pre-release | Planned | E0-T5 |',
  ));
}, /E12-T1 depends on retired E0-T5/);

expectFailure('reserved dependency', root => {
  write(root, 'docs/roadmap.md', text => text.replace(
    '| E12-T1 | Pre-release | Planned | E0-T7; Contract ready |',
    '| E12-T1 | Pre-release | Planned | E10-T1 |',
  ));
}, /E12-T1 depends on reserved E10-T1/);

expectFailure('invalid owner', root => {
  write(root, 'docs/required-specs.md', text => text.replace(/\| E12-T1 \|/, '| E99-T99 |'));
}, /has unknown owner E99-T99/);

expectFailure('duplicate task', root => {
  write(root, 'docs/roadmap.md', text => text.replace(
    /^\| E0-T5 \|.*$/m,
    match => `${match}\n${match}`,
  ));
}, /Duplicate Task: E0-T5/);

console.log('SOT validator fixtures passed: Git baseline, first-adoption, source-export, and 17 negative cases');
