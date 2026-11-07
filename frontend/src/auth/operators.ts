export type GoogleProfile = { email?: string | null; email_verified?: boolean | null };

/** Parses the operator list: comma-separated, trimmed and lowercased. */
export function operatorList(raw: string | undefined | null): string[] {
  return (raw ?? "")
    .split(",")
    .map((e) => e.trim().toLowerCase())
    .filter((e) => e.length > 0);
}

/**
 * Decides whether the Google account may sign in: `email_verified`, a single `@` and
 * e-mail in the operator list. An empty or missing list lets nobody in.
 */
export function accountAllowed(
  profile: GoogleProfile | null | undefined,
  raw: string | undefined | null,
): boolean {
  if (!profile || profile.email_verified !== true || typeof profile.email !== "string")
    return false;
  const email = profile.email.trim().toLowerCase();
  if (email.indexOf("@") !== email.lastIndexOf("@")) return false;
  return operatorList(raw).includes(email);
}

/** Accepts only internal paths, for the return after sign-in. */
export function safeReturnPath(value: string | undefined | null): string {
  if (!value || !value.startsWith("/") || value.startsWith("//") || value.includes("\\"))
    return "/processar";
  return value;
}
