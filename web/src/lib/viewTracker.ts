export interface PageViewCommand {
  cmd: 'track';
  event: 'page_view';
  name: string;
  duration_ms: number;
}

export const MAX_VIEW_DURATION_MS = 21_600_000;
export const MAX_VIEW_NAME_LENGTH = 100;

const UNTRACKED_ROOTS = ['/admin', '/login', '/v'];

export function isTrackedPath(path: string): boolean {
  if (!path.startsWith('/') || path.length > MAX_VIEW_NAME_LENGTH) return false;
  return !UNTRACKED_ROOTS.some((root) => path === root || path.startsWith(`${root}/`));
}

export function createViewTracker(
  send: (cmd: PageViewCommand) => void,
  now: () => number = Date.now,
) {
  let path: string | null = null;
  let startedAt: number | null = null;

  function flush() {
    if (path === null || startedAt === null) return;
    const duration = Math.min(Math.max(0, now() - startedAt), MAX_VIEW_DURATION_MS);
    startedAt = null;
    send({ cmd: 'track', event: 'page_view', name: path, duration_ms: Math.round(duration) });
  }

  function enter(next: string) {
    flush();
    if (isTrackedPath(next)) {
      path = next;
      startedAt = now();
    } else {
      path = null;
    }
  }

  function hide() {
    flush();
  }

  function show() {
    if (path !== null && startedAt === null) startedAt = now();
  }

  return { enter, hide, show };
}
