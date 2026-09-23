import fs from 'node:fs';
import path from 'node:path';
import {execFileSync} from 'node:child_process';
import {fileURLToPath} from 'node:url';

const documentNames = [
  'architecture-decision-records.md', 'architecture.md',
  'implementation-memo.md', 'required-specs.md', 'roadmap.md',
];
const taskPattern = /^E\d+-T\d+$/;
const states = new Set(['Planned', 'In Progress', 'In Review', 'Completed', 'Blocked', 'Deferred', 'Retired']);
const phases = new Set(['Historical', 'Pre-release', 'Post-release', 'Deferred']);
const registrySchema = 'gul.task-identities/v1';
const presenceValues = new Set(['required', 'reserved']);

function compareTaskIds(left, right) {
  const [, leftEpic, leftTask] = left.match(/^E(\d+)-T(\d+)$/).map(Number);
  const [, rightEpic, rightTask] = right.match(/^E(\d+)-T(\d+)$/).map(Number);
  return leftEpic - rightEpic || leftTask - rightTask;
}

export function extractAllocatedTaskIds(text) {
  const ids = new Set();
  for (const match of text.matchAll(/\bE(\d+)-T(\d+)\.\.T?(\d+)\b/g)) {
    const epic = Number(match[1]);
    const first = Number(match[2]);
    const last = Number(match[3]);
    if (first <= last) for (let task = first; task <= last; task += 1) ids.add(`E${epic}-T${task}`);
  }
  for (const match of text.matchAll(/\bE\d+-T\d+\b/g)) ids.add(match[0]);
  return ids;
}

export function readGitEvidence(root, runGit = null) {
  const result = {
    mode: 'git', baselineRegistry: null, historicalRoadmaps: [], errors: [], warnings: [],
  };
  if (!fs.existsSync(path.join(root, '.git'))) {
    result.mode = 'source-export';
    result.warnings.push('Source export has no Git metadata; current structure was checked, but historical Task ID nondeletion was not verified.');
    return result;
  }
  const execute = runGit || (args => execFileSync('git', args, {
    cwd: root, encoding: 'utf8', stdio: ['ignore', 'pipe', 'ignore'],
  }));

  let registryEntry = null;
  try {
    registryEntry = execute(['ls-tree', 'HEAD', '--', 'docs/task-identities.json']);
  } catch {
    result.errors.push('Cannot inspect the committed Task identity registry through Git');
  }
  if (registryEntry && registryEntry.trim()) {
    let baseline = null;
    try {
      baseline = execute(['show', 'HEAD:docs/task-identities.json']);
    } catch {
      result.errors.push('Cannot read the existing committed Task identity registry through Git');
    }
    if (baseline !== null) {
      try {
        result.baselineRegistry = JSON.parse(baseline);
      } catch (error) {
        result.errors.push(`Committed Task identity registry is malformed JSON: ${error.message}`);
      }
    }
  }

  let revisions = [];
  try {
    revisions = execute(['rev-list', 'HEAD', '--', 'docs/roadmap.md']).trim().split('\n').filter(Boolean);
  } catch {
    result.errors.push('Cannot read committed roadmap history through Git');
  }
  if (!result.errors.some(error => error === 'Cannot read committed roadmap history through Git') && !revisions.length) {
    result.errors.push('Committed roadmap history is unavailable');
  }
  for (const revision of revisions) {
    try {
      result.historicalRoadmaps.push(execute(['show', `${revision}:docs/roadmap.md`]));
    } catch {
      result.errors.push(`Cannot read committed roadmap revision ${revision} through Git`);
    }
  }
  return result;
}

function parseRegistry(raw, errors, label) {
  if (!raw || typeof raw !== 'object' || Array.isArray(raw)) {
    errors.push(`${label} must be a JSON object`);
    return new Map();
  }
  if (raw.schema !== registrySchema) errors.push(`${label} has unsupported schema ${String(raw.schema)}`);
  if (raw.identityAuthority !== 'Append-only permanent Task ID allocation and non-reuse only.') {
    errors.push(`${label} must declare the permanent identity-only authority`);
  }
  if (raw.lifecycleAuthority !== 'docs/roadmap.md owns Task order, status, phase, dependencies, and activation.') {
    errors.push(`${label} must defer lifecycle authority to docs/roadmap.md`);
  }
  if (!Array.isArray(raw.identities)) {
    errors.push(`${label} identities must be an array`);
    return new Map();
  }
  const entries = new Map();
  let previous = null;
  for (const entry of raw.identities) {
    if (!entry || typeof entry !== 'object' || Array.isArray(entry) || !taskPattern.test(entry.id || '')) {
      errors.push(`${label} has an invalid Task identity entry`);
      continue;
    }
    if (!presenceValues.has(entry.roadmapPresence)) errors.push(`${label} has invalid roadmapPresence for ${entry.id}`);
    if (Object.keys(entry).sort().join(',') !== 'id,roadmapPresence') {
      errors.push(`${label} entry ${entry.id} must contain only id and roadmapPresence`);
    }
    if (entries.has(entry.id)) errors.push(`${label} has duplicate Task identity ${entry.id}`);
    if (previous && compareTaskIds(previous, entry.id) >= 0) errors.push(`${label} identities are not in canonical numeric order at ${entry.id}`);
    entries.set(entry.id, entry.roadmapPresence);
    previous = entry.id;
  }
  if (!entries.size) errors.push(`${label} has no Task identities`);
  return entries;
}

function section(text, start, end, errors) {
  const first = text.indexOf(start);
  const last = text.indexOf(end, first + start.length);
  if (first < 0 || last < 0) {
    errors.push(`Missing required section: ${start}`);
    return '';
  }
  return text.slice(first, last);
}

function uniqueMarkerSection(text, start, end, errors, label) {
  const firstStart = text.indexOf(start);
  const secondStart = firstStart < 0 ? -1 : text.indexOf(start, firstStart + start.length);
  const firstEnd = firstStart < 0 ? -1 : text.indexOf(end, firstStart + start.length);
  const secondEnd = firstEnd < 0 ? -1 : text.indexOf(end, firstEnd + end.length);
  if (firstStart < 0 || firstEnd < 0 || secondStart >= 0 || secondEnd >= 0) {
    errors.push(`Architecture must contain exactly one ${label} marker pair`);
    return '';
  }
  return text.slice(firstStart + start.length, firstEnd);
}

export function validateRepository(root, options = {}) {
  const errors = [];
  const warnings = [];
  const documents = new Map();
  for (const name of documentNames) {
    try {
      documents.set(name, fs.readFileSync(path.join(root, 'docs', name), 'utf8'));
    } catch (error) {
      errors.push(`Cannot read docs/${name}: ${error.message}`);
    }
  }
  let registryRaw = null;
  try {
    registryRaw = JSON.parse(fs.readFileSync(path.join(root, 'docs', 'task-identities.json'), 'utf8'));
  } catch (error) {
    errors.push(`Cannot read docs/task-identities.json: ${error.message}`);
  }
  if (errors.length) return {errors, warnings};

  const roadmap = documents.get('roadmap.md');
  const architecture = documents.get('architecture.md');
  const currentOperationMap = uniqueMarkerSection(
    architecture,
    '<!-- contract-operation-map:start -->',
    '<!-- contract-operation-map:end -->',
    errors,
    'current checked operation map',
  );
  for (const operation of ['GetOrchestratedSession', 'ListOrchestratedSessionResults']) {
    if (!currentOperationMap.includes(`OrchestrationService.${operation}`)) {
      errors.push(`Checked consumer operation map is missing OrchestrationService.${operation}`);
    }
  }
  for (const rpc of [
    'RunService.CreateWriteContinuation',
    'RunService.DeleteRun',
    'RunService.ForkRun',
    'RunService.SetDefaultEffort',
    'RunService.VerifyRun',
    'RuntimeService.ListProfileDiagnostics',
    'WriterService.CancelWriterHandoff',
    'WriterService.CommitWriterHandoff',
    'WriterService.PrepareWriterHandoff',
  ]) {
    if (currentOperationMap.includes(rpc)) errors.push(`Checked consumer operation map contains unavailable RPC ${rpc}`);
  }
  const tasks = new Map();
  for (const line of roadmap.split('\n')) {
    const cells = line.split('|').map(cell => cell.trim());
    if (cells.length !== 7 || !taskPattern.test(cells[1])) continue;
    const [id, phase, state, prerequisites] = cells.slice(1, 5);
    if (tasks.has(id)) errors.push(`Duplicate Task: ${id}`);
    if (!states.has(state) || !phases.has(phase)) errors.push(`Invalid phase/status: ${id}: ${phase}/${state}`);
    if ((phase === 'Deferred') !== (state === 'Deferred')) errors.push(`Deferred phase/status mismatch: ${id}`);
    if (phase === 'Pre-release' && /Provider released/.test(prerequisites)) errors.push(`Pre-release Task ${id} waits for provider release`);
    tasks.set(id, {phase, state, deps: prerequisites.match(/E\d+-T\d+/g) || [], position: tasks.size});
  }
  if (!tasks.size) errors.push('No canonical Task DAG rows found');

  const currentE12Members = [...tasks]
    .filter(([id, task]) => id.startsWith('E12-') && !['Deferred', 'Retired'].includes(task.state))
    .map(([id]) => id);
  if (currentE12Members.length !== 1 || currentE12Members[0] !== 'E12-T1'
      || tasks.get('E12-T1')?.phase !== 'Pre-release' || tasks.get('E12-T1')?.state !== 'Completed') {
    errors.push('E12 must have exactly one current member: completed Pre-release Task E12-T1');
  }
  for (const id of ['E12-T2', 'E12-T3']) {
    const task = tasks.get(id);
    if (task?.phase !== 'Historical' || task?.state !== 'Retired') {
      errors.push(`${id} must remain Historical and Retired`);
    }
  }

  // A topological Task order can still require leaving and re-entering an Epic.
  // Retired and deferred identities are not members of first-release execution.
  const epicBlocks = [];
  for (const [id, task] of tasks) {
    if (['Deferred', 'Retired'].includes(task.state)) continue;
    const epic = id.split('-')[0];
    if (epic === epicBlocks.at(-1)) continue;
    if (epicBlocks.includes(epic)) errors.push(`First-release Epic ${epic} is interleaved across Task blocks`);
    epicBlocks.push(epic);
  }
  const epicSummaries = [...roadmap.matchAll(/^\| (E\d+) \| ([^|]+) \| ([^|]+) \| ([^|]+) \|$/gm)];
  const summaryIds = epicSummaries.map(match => match[1]);
  if (summaryIds.length !== new Set(summaryIds).size) errors.push('Duplicate Epic summary row');
  for (const [, epic, state, , dossier] of epicSummaries) {
    if (!states.has(state.trim())) errors.push(`Invalid Epic summary status: ${epic}: ${state.trim()}`);
    if (![...tasks.keys()].some(id => id.startsWith(`${epic}-`))) errors.push(`Epic summary ${epic} has no Task rows`);
    if (['Planned', 'In Progress', 'In Review'].includes(state.trim())
        && dossier.trim() !== '[Shared](todo/GUL-CONSUMER-REBASELINE.md)') {
      errors.push(`Pending Epic ${epic} must retain the shared implementation dossier link`);
    }
  }
  const releaseSummaryOrder = summaryIds.filter(epic => epicBlocks.includes(epic));
  if (releaseSummaryOrder.join(',') !== epicBlocks.join(',')) {
    errors.push(`Epic summary order must match first-release Task blocks: ${epicBlocks.join(', ')}`);
  }

  const active = [...tasks].filter(([, task]) => ['In Progress', 'In Review'].includes(task.state));
  if (active.length > 1) errors.push(`Multiple active Tasks: ${active.map(([id]) => id).join(', ')}`);
  const activeHeaders = [...roadmap.matchAll(/^\| Active Task \| ([^|]+) \|$/gm)].map(match => match[1].trim());
  const expectedActive = active.length === 1 ? active[0][0] : 'None';
  if (activeHeaders.length !== 1 || activeHeaders[0] !== expectedActive) {
    errors.push(`Active Task header must be exactly ${expectedActive}; found ${activeHeaders.length === 1 ? activeHeaders[0] : `${activeHeaders.length} headers`}`);
  }

  if (!active.length) {
    const nextEligible = [...tasks].find(([, task]) => task.state === 'Planned'
      && task.deps.every(dependency => tasks.get(dependency)?.state === 'Completed'))?.[0];
    const nextHeaders = [...roadmap.matchAll(/^\| Next \| ([^|]+) \|$/gm)].map(match => match[1].trim());
    if (nextEligible && (nextHeaders.length !== 1 || !new RegExp(`\\b${nextEligible}\\b`).test(nextHeaders[0]))) {
      errors.push(`Next header must identify first eligible Task ${nextEligible}`);
    }
  }

  const e12State = epicSummaries.find(match => match[1] === 'E12')?.[2].trim();
  const currentSnapshot = section(architecture, '## 19. Current snapshot', '### 19.1 ', errors);
  if (e12State === 'Completed' && !currentSnapshot.includes('E12 is `Completed`')) {
    errors.push('Architecture current snapshot must identify E12 as Completed');
  }
  if (tasks.get('E1-T1')?.state === 'Completed' && !currentSnapshot.includes('E1-T1 is `Completed`')) {
    errors.push('Architecture current snapshot must identify E1-T1 as Completed');
  }
  if (tasks.get('E1-T1')?.state === 'Completed') {
    for (const boundary of [
      'No Runtime Provider adapter',
      'Production authentication,',
    ]) {
      if (!architecture.includes(boundary)) errors.push(`Architecture must retain the E1-T1 boundary: ${boundary}`);
    }
  }
  if (tasks.get('E1-T4')?.state === 'Planned') {
    if (!architecture.includes('No Gul SQLite schema exists.')) errors.push('Architecture must retain the pre-E1-T4 SQLite absence boundary');
  } else if (!architecture.includes('Gul-only SQLite schema and repositories exist in isolated tests; no production database lifecycle is enabled.')) {
    errors.push('Architecture must distinguish isolated SQLite repositories from production database lifecycle');
  }
  if (tasks.get('E1-T3')?.state === 'Planned') {
    if (!architecture.includes('No ConnectRPC service exists.')) errors.push('Architecture must retain the pre-E1-T3 ConnectRPC absence boundary');
  } else if (!architecture.includes('ConnectRPC services are declared and generated but not registered.')) {
    errors.push('Architecture must distinguish declared from registered ConnectRPC services');
  }
  if (tasks.get('E1-T2')?.state === 'Completed') {
    const architectureCommands = section(architecture, '### 17.1 ', '## 18.', errors);
    const memoCommands = section(documents.get('implementation-memo.md'), '## 10.', '## 11.', errors);
    for (const boundary of [
      'One checked React bundle and shared browser/shell asset delivery exist.',
    ]) {
      if (!architecture.includes(boundary)) {
        errors.push(`E1-T2 shared-bundle boundary is missing: ${boundary}`);
      }
    }
    if (tasks.get('E1-T5')?.state === 'Planned') {
      if (!architecture.includes('No Wails host exists.')) errors.push('Architecture must retain the pre-E1-T5 Wails absence boundary');
    } else if (!architecture.includes('An isolated Wails shell foundation reuses the shared core and checked bundle; authenticated attach is not enabled.')) {
      errors.push('Architecture must distinguish the isolated Wails shell from authenticated attach');
    }
    if (tasks.get('E1-T5')?.state !== 'Planned' && !documents.get('required-specs.md').includes('The Wails shell has no authenticated attach.')) {
      errors.push('Required Specifications must retain the E1-T5 unauthenticated shell boundary');
    }
    if (!documents.get('required-specs.md').includes('not partial promotion of E14-owned REQ-HOST-001/002')) {
      errors.push('E1-T2 requirement non-promotion boundary is missing from Required Specifications');
    }
    const frontendCommandSequence = '`generate-frontend`, `frontend-check`';
    if (!architectureCommands.includes(frontendCommandSequence)) errors.push('Architecture command facade is missing generate-frontend/frontend-check');
    if (!memoCommands.includes(frontendCommandSequence)) errors.push('Implementation memo command contract is missing generate-frontend/frontend-check');
    if (!memoCommands.includes('frontend bundle drift')) {
      errors.push('Implementation memo test-int contract is missing frontend bundle drift');
    }
  } else if (!architecture.includes('No Wails host or frontend exists.')) {
    errors.push('Architecture must retain the pre-E1-T2 frontend absence boundary');
  }
  if (tasks.get('E1-T3')?.state !== 'Planned') {
    const architectureCommands = section(architecture, '### 17.1 ', '## 18.', errors);
    const memoCommands = section(documents.get('implementation-memo.md'), '## 10.', '## 11.', errors);
    const apiCommands = '`generate-api`, `api-check`';
    if (!architectureCommands.includes(apiCommands)) errors.push('Architecture command facade is missing generate-api/api-check');
    if (!memoCommands.includes(apiCommands)) errors.push('Implementation memo command contract is missing generate-api/api-check');
  }

  const registry = parseRegistry(registryRaw, errors, 'Task identity registry');
  const evidence = options.gitEvidence || readGitEvidence(root, options.gitRunner);
  errors.push(...(evidence.errors || []));
  warnings.push(...(evidence.warnings || []));
  let baseline = new Map();
  if (evidence.baselineRegistry) baseline = parseRegistry(evidence.baselineRegistry, errors, 'Committed Task identity registry');
  const historicalIds = new Set();
  for (const historicalRoadmap of evidence.historicalRoadmaps || []) {
    for (const id of extractAllocatedTaskIds(historicalRoadmap)) historicalIds.add(id);
  }
  if (evidence.mode === 'git' && (evidence.historicalRoadmaps || []).length && !historicalIds.size) {
    errors.push('Committed roadmap history contains no Task identities');
  }
  for (const [id, presence] of baseline) {
    if (!registry.has(id)) errors.push(`Committed Task identity was deleted: ${id}`);
    else if (registry.get(id) !== presence) errors.push(`Committed Task identity was changed: ${id}: ${presence} -> ${registry.get(id)}`);
  }
  const hasAllocationEvidence = evidence.mode === 'git';
  for (const id of historicalIds) if (!registry.has(id)) errors.push(`Historical Task identity is not registered: ${id}`);
  for (const [id, presence] of registry) {
    if (presence === 'required' && !tasks.has(id)) errors.push(`Required permanent Task row is missing: ${id}`);
    if (presence === 'reserved' && tasks.has(id)) errors.push(`Reserved Task identity appears in the executable DAG: ${id}`);
    if (presence === 'reserved' && hasAllocationEvidence && !baseline.has(id) && !historicalIds.has(id)) errors.push(`Reserved Task identity has no committed allocation evidence: ${id}`);
  }
  for (const id of tasks.keys()) if (!registry.has(id)) errors.push(`Roadmap Task identity is not registered: ${id}`);

  for (const [id, task] of tasks) {
    for (const dep of task.deps) {
      if (registry.get(dep) === 'reserved') {
        errors.push(`${id} depends on reserved ${dep}`);
        continue;
      }
      const parent = tasks.get(dep);
      if (!parent) {
        errors.push(`${id} depends on missing ${dep}`);
        continue;
      }
      if (parent.state === 'Retired') errors.push(`${id} depends on retired ${dep}`);
      if (task.phase !== 'Deferred' && parent.state === 'Deferred') errors.push(`Required Task ${id} depends on deferred ${dep}`);
      if (task.phase === 'Pre-release' && parent.phase === 'Post-release') errors.push(`Pre-release Task ${id} depends on post-release ${dep}`);
      if (parent.position >= task.position) errors.push(`${id} is ordered before prerequisite ${dep}`);
    }
  }
  const visited = new Set();
  const visiting = new Set();
  function visit(id) {
    if (visiting.has(id)) {
      errors.push(`Task dependency cycle at ${id}`);
      return;
    }
    if (visited.has(id)) return;
    visiting.add(id);
    for (const dep of tasks.get(id).deps) if (tasks.has(dep)) visit(dep);
    visiting.delete(id);
    visited.add(id);
  }
  for (const id of tasks.keys()) visit(id);

  const spec = documents.get('required-specs.md');
  const current = section(spec, '### 5.1 ', '## 6.', errors);
  const deferred = section(spec, '## 6. Deferred State ledger', '## 7.', errors);
  const requirementPattern = /^\| (REQ-[A-Z0-9-]+) \|/gm;
  const currentIds = [...current.matchAll(requirementPattern)].map(match => match[1]);
  const deferredIds = [...deferred.matchAll(requirementPattern)].map(match => match[1]);
	const requirementOwners = new Map();
  for (const [label, ids] of [['active', currentIds], ['deferred', deferredIds]]) {
    if (!ids.length || ids.length !== new Set(ids).size) errors.push(`Missing or duplicate ${label} requirement definitions`);
  }
  for (const id of currentIds) if (deferredIds.includes(id)) errors.push(`Requirement is both active and deferred: ${id}`);
  const deferredOwners = new Set([...roadmap.matchAll(/^\| (Deferred-[A-Za-z0-9-]+) \| Deferred \|/gm)].map(match => match[1]));
  for (const [ledger, isDeferred] of [[current, false], [deferred, true]]) {
    for (const line of ledger.split('\n')) {
      const cells = line.split('|').map(cell => cell.trim());
      if (cells.length < 4 || !/^REQ-[A-Z0-9-]+$/.test(cells[1])) continue;
      const owner = cells[cells.length - 2];
	  requirementOwners.set(cells[1], owner);
      const task = tasks.get(owner);
      if (!task && !(isDeferred && deferredOwners.has(owner))) errors.push(`${cells[1]} has unknown owner ${owner}`);
      if (task && !isDeferred && ['Deferred', 'Retired'].includes(task.state)) errors.push(`Active requirement ${cells[1]} has non-release owner ${owner}`);
    }
  }
	const acceptedState = section(spec, '## 7. Current State ledger', '## 8.', errors);
	for (const id of [...acceptedState.matchAll(requirementPattern)].map(match => match[1])) {
	  const owner = requirementOwners.get(id);
	  if (!owner) {
	    errors.push(`Accepted Current State requirement ${id} has no active requirement owner`);
	  } else if (tasks.get(owner)?.state !== 'Completed') {
	    errors.push(`Accepted Current State requirement ${id} has incomplete owner ${owner}`);
	  }
	}

  const adrs = documents.get('architecture-decision-records.md');
  const index = new Set([...adrs.matchAll(/^\| (ADR-\d{4}) \|/gm)].map(match => match[1]));
  const bodies = [...adrs.matchAll(/^### (ADR-\d{4}):/gm)].map(match => match[1]);
  const bodySet = new Set(bodies);
  if (bodies.length !== bodySet.size || index.size !== bodySet.size || [...index].some(id => !bodySet.has(id))) errors.push('ADR index/body mismatch or duplicate');

  const files = documentNames.map(name => path.join(root, 'docs', name));
  const todoRoot = path.join(root, 'docs', 'todo');
  if (fs.existsSync(todoRoot)) for (const name of fs.readdirSync(todoRoot)) if (name.endsWith('.md')) files.push(path.join(todoRoot, name));
  for (const file of files) {
    const text = fs.readFileSync(file, 'utf8');
    for (const [, destination] of text.matchAll(/\[[^\]]*\]\(([^)\s]+)\)/g)) {
      if (/^[a-z][a-z0-9+.-]*:/i.test(destination) || destination.startsWith('#')) continue;
      const target = decodeURIComponent(destination.split('#')[0]);
      if (target && !fs.existsSync(path.resolve(path.dirname(file), target))) errors.push(`Missing link in ${path.relative(root, file)}: ${destination}`);
    }
  }

  const requiredCount = [...tasks.values()].filter(task => !['Deferred', 'Retired'].includes(task.state)).length;
  const deferredCount = [...tasks.values()].filter(task => task.state === 'Deferred').length;
  const reservedCount = [...registry.values()].filter(presence => presence === 'reserved').length;
  return {
    errors,
    warnings,
    summary: `${requiredCount} first-release Tasks, ${deferredCount} deferred, ${active.length} active, ${currentIds.length} active requirements, ${deferredIds.length} deferred requirements, ${index.size} ADRs, ${registry.size} permanent Task IDs (${reservedCount} reserved); DAG, Epic blocks/order, phases, owners, identities, and links valid; ${evidence.mode === 'source-export' ? 'historical nondeletion unverified' : 'committed identity history verified'}`,
  };
}

function runCli() {
  const root = process.argv[2];
  if (!root) {
    console.error('Usage: node scripts/check-sot.mjs <repository-root>');
    process.exit(2);
  }
  const result = validateRepository(path.resolve(root));
  if (result.errors.length) {
    console.error(`SOT checks failed:\n${result.errors.map(error => `- ${error}`).join('\n')}`);
    process.exit(1);
  }
  for (const warning of result.warnings) console.warn(`SOT check warning: ${warning}`);
  console.log(`SOT checks passed: ${result.summary}`);
}

if (process.argv[1] && fileURLToPath(import.meta.url) === path.resolve(process.argv[1])) runCli();
