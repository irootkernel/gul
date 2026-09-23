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

let negativeCases = 0;

function expectFailure(name, mutate, pattern, baseline = baselineRegistry) {
  const root = fixture();
  try {
    mutate(root);
    const result = run(root, baseline);
    assert(result.errors.some(error => pattern.test(error)), `${name}: expected ${pattern}, got:\n${result.errors.join('\n')}`);
    negativeCases += 1;
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

expectFailure('pre-E1-T2 state requires the frontend absence boundary', root => {
  write(root, 'docs/roadmap.md', text => text
    .replace('| Active Task | None |', '| Active Task | E1-T2 |')
    .replace('| E1-T2 | Pre-release | Completed |', '| E1-T2 | Pre-release | In Review |')
    .replace('| E1-T3 | Pre-release | Completed |', '| E1-T3 | Pre-release | Planned |')
    .replace('| E1-T4 | Pre-release | Completed |', '| E1-T4 | Pre-release | Planned |'));
}, /pre-E1-T2 frontend absence boundary/);

expectFailure('multiple active rows', root => {
  write(root, 'docs/roadmap.md', text => text
    .replace('| E1-T3 | Pre-release | Completed |', '| E1-T3 | Pre-release | In Progress |')
    .replace('| E1-T4 | Pre-release | Completed |', '| E1-T4 | Pre-release | In Progress |'));
}, /Multiple active Tasks/);

expectFailure('active header mismatch', root => {
  write(root, 'docs/roadmap.md', text => text.replace('| E1-T4 | Pre-release | Completed |', '| E1-T4 | Pre-release | In Progress |'));
}, /Active Task header must be exactly E1-T4/);

expectFailure('active row mismatch', root => {
  write(root, 'docs/roadmap.md', text => text.replace('| Active Task | None |', '| Active Task | E1-T4 |'));
}, /Active Task header must be exactly None/);

expectFailure('next header misses first eligible task', root => {
  write(root, 'docs/roadmap.md', text => text
    .replace('| Next | E1-T5 |', '| Next | E13 starting at E13-T1 |'));
}, /Next header must identify first eligible Task E1-T5/);

expectFailure('pending Epic loses shared dossier', root => {
  write(root, 'docs/roadmap.md', text => text.replace(
    /^(\| E1 \| In Progress \|[^\n]+\|) \[Shared\]\(todo\/GUL-CONSUMER-REBASELINE\.md\) \|$/m,
    '$1 None |',
  ));
}, /Pending Epic E1 must retain the shared implementation dossier link/);

expectFailure('architecture retains stale E12 lifecycle', root => {
  write(root, 'docs/architecture.md', text => text.replace(
    'E12 is `Completed`',
    'E12 is in completion review',
  ));
}, /Architecture current snapshot must identify E12 as Completed/);

expectFailure('architecture retains stale E1-T1 lifecycle', root => {
  write(root, 'docs/architecture.md', text => text.replace(
    'E1-T1 is `Completed`',
    'E1-T1 is in completion review',
  ));
}, /Architecture current snapshot must identify E1-T1 as Completed/);

expectFailure('architecture drops the completed E1-T2 shared bundle', root => {
  write(root, 'docs/architecture.md', text => text.replace(
    'One checked React bundle and shared browser/shell asset delivery exist.',
    'Frontend delivery state is unspecified.',
  ));
}, /E1-T2 shared-bundle boundary is missing/);

expectFailure('required specs promote E14-owned bundle requirement early', root => {
  write(root, 'docs/required-specs.md', text => text.replace(
    'not partial promotion of E14-owned REQ-HOST-001/002',
    'promotes E14-owned REQ-HOST-001/002',
  ));
}, /E1-T2 requirement non-promotion boundary is missing/);

expectFailure('architecture drops the frontend generation command', root => {
  write(root, 'docs/architecture.md', text => text.replace('`generate-frontend`, ', ''));
}, /Architecture command facade is missing generate-frontend\/frontend-check/);

expectFailure('implementation memo drops the frontend drift command', root => {
  write(root, 'docs/implementation-memo.md', text => text.replace(
    '`generate-frontend`, `frontend-check`, `generate-api`',
    '`generate-frontend`, `generate-api`',
  ));
}, /Implementation memo command contract is missing generate-frontend\/frontend-check/);

expectFailure('architecture drops the Gul API commands', root => {
  write(root, 'docs/architecture.md', text => text.replace('`generate-api`, `api-check`, ', ''));
}, /Architecture command facade is missing generate-api\/api-check/);

expectFailure('implementation memo drops the Gul API commands', root => {
  write(root, 'docs/implementation-memo.md', text => text.replace('`generate-api`, `api-check`, ', ''));
}, /Implementation memo command contract is missing generate-api\/api-check/);

expectFailure('implementation memo drops frontend drift from test-int', root => {
  write(root, 'docs/implementation-memo.md', text => text.replace('frontend bundle drift, ', ''));
}, /Implementation memo test-int contract is missing frontend bundle drift/);

expectFailure('current state promotes a requirement before its owner completes', root => {
  write(root, 'docs/required-specs.md', text => text.replace(
    '| REQ-HOST-005 | Exact Go toolchain',
    '| REQ-HOST-001 | Premature foundation promotion | E1-T1 |\n| REQ-HOST-005 | Exact Go toolchain',
  ));
}, /Accepted Current State requirement REQ-HOST-001 has incomplete owner E14-T1/);

expectFailure('architecture drops an E1-T1 delivery boundary', root => {
  write(root, 'docs/architecture.md', text => text.replace(
    'ConnectRPC services are declared and generated but not registered.',
    'ConnectRPC delivery state is unspecified.',
  ));
}, /Architecture must distinguish declared from registered ConnectRPC services/);

expectFailure('pre-E1-T3 state requires ConnectRPC absence', root => {
  write(root, 'docs/roadmap.md', text => text
    .replace('| E1-T3 | Pre-release | Completed |', '| E1-T3 | Pre-release | Planned |')
    .replace('| E1-T4 | Pre-release | In Progress |', '| E1-T4 | Pre-release | Planned |')
    .replace('| Active Task | E1-T4 |', '| Active Task | None |')
    .replace('| Next | E1-T4 completion, then E1-T5 |', '| Next | E1 continuing at E1-T3 |'));
}, /pre-E1-T3 ConnectRPC absence boundary/);

expectFailure('E1-T4 storage is not a production database lifecycle', root => {
  write(root, 'docs/architecture.md', text => text.replace(
    'Gul-only SQLite schema and repositories exist in isolated tests; no production database lifecycle is enabled.',
    'Gul database is ready.',
  ));
}, /Architecture must distinguish isolated SQLite repositories from production database lifecycle/);

expectFailure('retired E12 member becomes deferred', root => {
  write(root, 'docs/roadmap.md', text => text.replace(
    '| E12-T2 | Historical | Retired |',
    '| E12-T2 | Deferred | Deferred |',
  ));
}, /E12-T2 must remain Historical and Retired/);

expectFailure('E12 gains a second current member', root => {
  write(root, 'docs/roadmap.md', text => {
    const secondMember = text.match(/^\| E12-T2 \|.*$/m)[0]
      .replace('| Historical | Retired |', '| Pre-release | Completed |');
    return text
      .replace(/^\| E12-T2 \|.*\n/m, '')
      .replace(/^\| E12-T1 \|.*$/m, row => `${row}\n${secondMember}`);
  });
}, /E12 must have exactly one current member/);

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
    '| E1-T1 | Pre-release | Completed | E12-T1 |',
    '| E1-T1 | Pre-release | Completed | E1-T2 |',
  ));
}, /Task dependency cycle at E1-T1|Task dependency cycle at E1-T2/);

expectFailure('retired dependency', root => {
  write(root, 'docs/roadmap.md', text => text.replace(
    '| E1-T1 | Pre-release | Completed | E12-T1 |',
    '| E1-T1 | Pre-release | Completed | E0-T5 |',
  ));
}, /E1-T1 depends on retired E0-T5/);

expectFailure('reserved dependency', root => {
  write(root, 'docs/roadmap.md', text => text.replace(
    '| E1-T1 | Pre-release | Completed | E12-T1 |',
    '| E1-T1 | Pre-release | Completed | E10-T1 |',
  ));
}, /E1-T1 depends on reserved E10-T1/);

expectFailure('invalid owner', root => {
  write(root, 'docs/required-specs.md', text => text.replace(/\| E12-T1 \|/, '| E99-T99 |'));
}, /has unknown owner E99-T99/);

expectFailure('duplicate task', root => {
  write(root, 'docs/roadmap.md', text => text.replace(
    /^\| E0-T5 \|.*$/m,
    match => `${match}\n${match}`,
  ));
}, /Duplicate Task: E0-T5/);

expectFailure('topological tasks still interleave an Epic', root => {
  write(root, 'docs/roadmap.md', text => {
    const moved = text.match(/^\| E1-T5 \|.*\n/m)[0];
    return text.replace(moved, '').replace(/^\| E13-T1 \|.*\n/m, row => `${row}${moved}`);
  });
}, /First-release Epic E1 is interleaved across Task blocks/);

expectFailure('Epic summary order differs from execution', root => {
  write(root, 'docs/roadmap.md', text => {
    const first = text.match(/^\| E12 \|.*$/m)[0];
    const second = text.match(/^\| E1 \|.*$/m)[0];
    return text.replace(/^\| (E12|E1) \|.*$/gm, row => row === first ? second : first);
  });
}, /Epic summary order must match first-release Task blocks/);

expectFailure('missing Epic summary', root => {
  write(root, 'docs/roadmap.md', text => text.replace(/^\| E13 \|.*\n/m, ''));
}, /Epic summary order must match first-release Task blocks/);

expectFailure('migrated fake requirement cannot retain retired owner', root => {
  write(root, 'docs/required-specs.md', text => text.replace(
    /^(\| REQ-CONSUMER-002 \|.*)\| E13-T1 \|$/m, '$1| E12-T2 |',
  ));
}, /Active requirement REQ-CONSUMER-002 has non-release owner E12-T2/);

expectFailure('missing checked operation-map boundary', root => {
  write(root, 'docs/architecture.md', text => text.replace(
    '<!-- contract-operation-map:start -->',
    '<!-- missing-operation-map:start -->',
  ));
}, /exactly one current checked operation map marker pair/);

expectFailure('unavailable RPC leaks into checked map', root => {
  write(root, 'docs/architecture.md', text => text.replace(
    '<!-- contract-operation-map:end -->',
    '| `DeleteRun` | `RunService.DeleteRun` |\n<!-- contract-operation-map:end -->',
  ));
}, /Checked consumer operation map contains unavailable RPC RunService.DeleteRun/);

expectFailure('checked operation map loses aggregate read', root => {
  write(root, 'docs/architecture.md', text => text.replace(
    /^\| `GetOrchestratedSession` \|.*\n/m,
    '',
  ));
}, /Checked consumer operation map is missing OrchestrationService.GetOrchestratedSession/);

console.log(`SOT validator fixtures passed: Git baseline, first-adoption, source-export, and ${negativeCases} negative cases`);
