export async function register(): Promise<void> {
  if (process.env.NEXT_RUNTIME === "nodejs") {
    // Fails on startup if the data source is misconfigured.
    const { chooseSource } = await import("@/server/trips/datasource");
    chooseSource();
  }
}
