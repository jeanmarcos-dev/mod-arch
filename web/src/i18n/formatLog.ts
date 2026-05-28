import { i18n } from './index';

// formatLog produces the event-log lines consumed by cpu.ts's pushLog calls.
// Unicode prefix glyphs live here (not in the locale files) so a translator
// cannot accidentally break the monospaced grid of the EVENT_LOG panel.
//
// Each helper accepts the raw payload fields cpu.ts already has on hand and
// returns the final string. Hex formatting also lives here because it's part
// of the technical identifier — never translated.

function t(key: string, params?: Record<string, unknown>): string {
  // The exported i18n.global has dynamic generics in vue-i18n v11; we use a
  // narrow runtime call signature here so formatLog stays untyped at the
  // call site while still routing through the live i18n instance.
  const tr = (i18n.global as unknown as { t: (k: string, p?: Record<string, unknown>) => string }).t;
  return tr(key, params);
}

function hex(n: number, width: number): string {
  return `0x${(n >>> 0).toString(16).padStart(width, '0')}`;
}

function modeLabel(mode: number): string {
  return mode === 3 ? 'USER' : 'KERNEL';
}

export const formatLog = {
  instruction(eip: number, mnemonic: string): string {
    return t('logEvents.instruction', {
      eip: eip.toString(16).padStart(6, '0'),
      mnemonic,
    });
  },
  modeSwitch(from: number, to: number, cause: string): string {
    return `▸ ${t('logEvents.modeSwitch', { from: modeLabel(from), to: modeLabel(to), cause })}`;
  },
  irq(vector: number, source: string): string {
    return `◉ ${t('logEvents.irq', { vector: hex(vector, 2), source })}`;
  },
  pageFault(addr: number, permNeeded: string, mode: number): string {
    return `✗ ${t('logEvents.pageFault', { addr: `0x${addr.toString(16)}`, perm: permNeeded, mode })}`;
  },
  halt(): string {
    return `▸ ${t('logEvents.halt')}`;
  },
  loaded(name: string, count: number, pids: number[]): string {
    return `▸ ${t('logEvents.loaded', { name, count, pids: pids.join(',') })}`;
  },
  reset(): string {
    return `▸ ${t('logEvents.reset')}`;
  },
  pcbSave(pid: number, eip: number): string {
    return `◍ ${t('logEvents.pcbSave', { pid, eip: `0x${eip.toString(16)}` })}`;
  },
  pcbRestore(pid: number, eip: number): string {
    return `◌ ${t('logEvents.pcbRestore', { pid, eip: `0x${eip.toString(16)}` })}`;
  },
  schedule(fromPid: number, toPid: number, reason: string | undefined, readyQueue: number[]): string {
    const reasonTag = reason ? ` (${reason})` : '';
    return `⇄ ${t('logEvents.schedule', {
      from: fromPid,
      to: toPid,
      reason: reasonTag,
      ready: readyQueue.join(','),
    })}`;
  },
  syscall(name: string, args: number[], result: number, pid: number): string {
    return `⚙ ${t('logEvents.syscall', {
      name,
      a0: args[0],
      a1: `0x${(args[1] >>> 0).toString(16)}`,
      a2: args[2],
      result,
      pid,
    })}`;
  },
  coreAssign(coreId: number, pid: number): string {
    return `◈ ${t('logEvents.coreAssign', { core: coreId, pid })}`;
  },
  coreRelease(coreId: number, pid: number, reason?: string): string {
    if (reason) {
      return `◇ ${t('logEvents.coreReleaseReason', { core: coreId, pid, reason })}`;
    }
    return `◇ ${t('logEvents.coreRelease', { core: coreId, pid })}`;
  },
  mutexCreated(id: number, pid: number): string {
    return `🔒 ${t('logEvents.mutexCreated', { id, pid })}`;
  },
  mutexAcquired(id: number, pid: number): string {
    return `🔒 ${t('logEvents.mutexAcquired', { id, pid })}`;
  },
  mutexWaiting(id: number, pid: number): string {
    return `⏸ ${t('logEvents.mutexWaiting', { id, pid })}`;
  },
  mutexReleased(id: number, pid: number): string {
    return `🔓 ${t('logEvents.mutexReleased', { id, pid })}`;
  },
  mutexHandoff(id: number, fromPid: number, toPid: number): string {
    return `↪ ${t('logEvents.mutexHandoff', { id, from: fromPid, to: toPid })}`;
  },
  mutexRejected(id: number, pid: number): string {
    return `✗ ${t('logEvents.mutexRejected', { id, pid })}`;
  },
  semCreated(id: number, count: number): string {
    return `◯ ${t('logEvents.semCreated', { id, count })}`;
  },
  semWait(id: number, pid: number, count: number): string {
    return `↓ ${t('logEvents.semWait', { id, pid, count })}`;
  },
  semBlocked(id: number, pid: number): string {
    return `⏸ ${t('logEvents.semBlocked', { id, pid })}`;
  },
  semSignal(id: number, pid: number, count: number): string {
    return `↑ ${t('logEvents.semSignal', { id, pid, count })}`;
  },
  semHandoff(id: number, fromPid: number, toPid: number): string {
    return `↪ ${t('logEvents.semHandoff', { id, from: fromPid, to: toPid })}`;
  },
  forkSpawn(parentPid: number, childPid: number, isVfork: boolean): string {
    const key = isVfork ? 'logEvents.vforkSpawn' : 'logEvents.forkSpawn';
    return `⤴ ${t(key, { parent: parentPid, child: childPid })}`;
  },
  reparent(pid: number, oldParent: number, newParent: number): string {
    return `↳ ${t('logEvents.reparent', { pid, old: oldParent, new: newParent })}`;
  },
  waitCall(pid: number, targetPid: number): string {
    const target = targetPid === -1 ? '−1' : targetPid.toString();
    return `⏳ ${t('logEvents.waitCall', { pid, target })}`;
  },
  waitReap(parentPid: number, childPid: number, exitCode: number, byInit: boolean): string {
    const exit = (exitCode >>> 0).toString(16).padStart(8, '0').toUpperCase();
    const key = byInit ? 'logEvents.initReap' : 'logEvents.waitReap';
    return `☠ ${t(key, { parent: parentPid, child: childPid, exit })}`;
  },
  threadCreate(tid: number, tgid: number, creatorPid: number): string {
    return `⤳ ${t('logEvents.threadCreate', { tid, tgid, creator: creatorPid })}`;
  },
  threadExit(tid: number, tgid: number, retval: number): string {
    const ret = (retval >>> 0).toString(16).padStart(8, '0').toUpperCase();
    return `⏏ ${t('logEvents.threadExit', { tid, tgid, retval: ret })}`;
  },
  wsDisconnected(): string {
    return `⚠ ${t('logEvents.wsDisconnected')}`;
  },
  wsError(): string {
    return `⚠ ${t('logEvents.wsError')}`;
  },
  wsJsonErr(err: unknown): string {
    return `⚠ ${t('logEvents.wsJsonErr', { err: String(err) })}`;
  },
  spectatorMode(label: string): string {
    return `▸ ${t('logEvents.spectatorMode', { label })}`;
  },
  subscriberErr(err: unknown): string {
    return `⚠ ${t('logEvents.subscriberErr', { err: String(err) })}`;
  },
  genericErr(message: string): string {
    return `⚠ ${t('logEvents.genericErr', { message })}`;
  },
  // backendError renders an error frame received from the Go server. The
  // backend ships `code` + `params` so the frontend can translate the message
  // against its own catalog and keep it in sync with the active locale. The
  // `fallback` is the raw `message` from the payload; we use it when the code
  // is not in the SPA catalog (forward compat with newer server codes).
  backendError(code: string, params: Record<string, unknown> | undefined, fallback?: string): string {
    const key = `backend.errors.${code}`;
    const translated = t(key, params);
    // vue-i18n returns the key itself when no translation is found. We treat
    // that as "missing code in catalog" and fall back to whatever the server
    // sent in `message`.
    if (translated === key && fallback) {
      return `⚠ ${fallback}`;
    }
    return `⚠ ${translated}`;
  },
};
