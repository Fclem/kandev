/**
 * Requests for the app-level workspace-move form. Menus are transient (a
 * listing row disappears when its task leaves the workspace), so the form is
 * owned by one host above every surface and receives a captured subject.
 */
export type TaskWorkspaceMoveSubject = {
  taskId: string;
  workspaceId: string;
  /** Element that receives focus when the form closes. */
  returnFocus: HTMLElement | null;
};

type Listener = () => void;

class TaskWorkspaceMoveRequests {
  private subject: TaskWorkspaceMoveSubject | null = null;
  private readonly listeners = new Set<Listener>();

  current(): TaskWorkspaceMoveSubject | null {
    return this.subject;
  }

  open(subject: TaskWorkspaceMoveSubject): void {
    this.subject = subject;
    this.emit();
  }

  close(): void {
    if (!this.subject) return;
    this.subject = null;
    this.emit();
  }

  subscribe(listener: Listener): () => void {
    this.listeners.add(listener);
    return () => this.listeners.delete(listener);
  }

  private emit(): void {
    for (const listener of this.listeners) listener();
  }
}

const requests = new WeakMap<object, TaskWorkspaceMoveRequests>();

export function taskWorkspaceMoveRequests(owner: object): TaskWorkspaceMoveRequests {
  let registry = requests.get(owner);
  if (!registry) {
    registry = new TaskWorkspaceMoveRequests();
    requests.set(owner, registry);
  }
  return registry;
}

/** Delay that lets a closing menu release focus and pointer capture first. */
const MENU_CLOSE_DELAY_MS = 300;

/** Opens the workspace-move form for one task after its menu has closed. */
export function requestTaskWorkspaceMove(owner: object, taskId: string, workspaceId: string): void {
  window.setTimeout(() => {
    const active = document.activeElement;
    taskWorkspaceMoveRequests(owner).open({
      taskId,
      workspaceId,
      returnFocus: active instanceof HTMLElement && active !== document.body ? active : null,
    });
  }, MENU_CLOSE_DELAY_MS);
}
