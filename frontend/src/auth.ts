import NextAuth from "next-auth";
import Google from "next-auth/providers/google";
import { accountAllowed } from "@/auth/operators";
import { BASE_PATH } from "@/config";

/**
 * Auth.js v5, JWT session without a database. The operator list is read on each
 * login (runtime), never at build time.
 */
export const { handlers, auth, signIn, signOut } = NextAuth(() => ({
  providers: [Google({ authorization: { params: { hd: "usp.br" } } })],
  session: { strategy: "jwt" },
  trustHost: true,
  pages: { signIn: `${BASE_PATH}/entrar`, error: `${BASE_PATH}/entrar` },
  callbacks: {
    signIn({ profile }) {
      if (accountAllowed(profile, process.env.OPERATOR_EMAILS)) return true;
      return `${BASE_PATH}/entrar?erro=conta_nao_liberada`;
    },
  },
}));
