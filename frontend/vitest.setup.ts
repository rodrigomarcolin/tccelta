import "@testing-library/jest-dom/vitest";
import { cleanup } from "@testing-library/react";
import { notifyManager } from "@tanstack/react-query";
import { afterEach, vi } from "vitest";
import { router } from "@/tests/router";

// `useRouter()` only works inside the mounted App Router; tests get a shared fake instead.
// Files that need other `next/navigation` exports mock the module themselves.
vi.mock("next/navigation", async (importOriginal) => {
  const { router } = await import("@/tests/router");
  return { ...(await importOriginal<typeof import("next/navigation")>()), useRouter: () => router };
});

afterEach(() => {
  for (const fn of Object.values(router)) fn.mockClear();
});

// React Query notifies observers with `setTimeout(0)`, which a fake clock turns into one more
// timer hop per update. Tests that advance fake time would need to flush a chain of them, so
// here it notifies on a microtask, which `advanceTimersByTimeAsync` flushes. Test only.
notifyManager.setScheduler(queueMicrotask);

afterEach(() => cleanup());

// jsdom has `<dialog>` but not `showModal()`/`close()`. This stand-in opens and closes it the way
// the browser does for what the tests observe: the `open` attribute, the `close` event and the
// focus given back to the element that was focused before.
const previousFocus = new WeakMap<HTMLDialogElement, Element | null>();
if (typeof HTMLDialogElement !== "undefined" && !HTMLDialogElement.prototype.showModal) {
  HTMLDialogElement.prototype.showModal = function showModal(this: HTMLDialogElement) {
    previousFocus.set(this, document.activeElement);
    this.setAttribute("open", "");
  };
  HTMLDialogElement.prototype.close = function close(this: HTMLDialogElement) {
    if (!this.open) return;
    this.removeAttribute("open");
    const back = previousFocus.get(this);
    if (back instanceof HTMLElement && back.isConnected) back.focus();
    this.dispatchEvent(new Event("close"));
  };
}
