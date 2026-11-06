import { vi } from "vitest";

/** Stand-in for the App Router's `useRouter()` in tests (see `vitest.setup.ts`). */
export const router = {
  push: vi.fn(),
  replace: vi.fn(),
  refresh: vi.fn(),
  back: vi.fn(),
  forward: vi.fn(),
  prefetch: vi.fn(),
};
