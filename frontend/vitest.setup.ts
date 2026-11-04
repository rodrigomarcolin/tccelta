import "@testing-library/jest-dom/vitest";
import { cleanup } from "@testing-library/react";
import { notifyManager } from "@tanstack/react-query";
import { afterEach } from "vitest";

// React Query notifies observers with `setTimeout(0)`, which a fake clock turns into one more
// timer hop per update. Tests that advance fake time would need to flush a chain of them, so
// here it notifies on a microtask, which `advanceTimersByTimeAsync` flushes. Test only.
notifyManager.setScheduler(queueMicrotask);

afterEach(() => cleanup());
