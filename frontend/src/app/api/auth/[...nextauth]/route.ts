import { NextRequest } from "next/server";
import { handlers } from "@/auth";
import { BASE_PATH } from "@/config";

/**
 * Next strips `basePath` from the URL it hands to route handlers, but Auth.js takes its own
 * basePath from the pathname of AUTH_URL (`/dashboard/api/auth`) and rejects any request whose
 * path does not start with it (`UnknownAction`). Put the prefix back before delegating, so the
 * URLs Auth.js emits (callback, sign-in) keep `/dashboard` and the requests still parse.
 */
function withBasePath(req: NextRequest): NextRequest {
  const url = new URL(req.url);
  if (url.pathname === BASE_PATH || url.pathname.startsWith(`${BASE_PATH}/`)) return req;
  url.pathname = `${BASE_PATH}${url.pathname}`;
  return new NextRequest(url, req);
}

export const GET = (req: NextRequest) => handlers.GET(withBasePath(req));
export const POST = (req: NextRequest) => handlers.POST(withBasePath(req));
