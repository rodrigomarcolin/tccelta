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
