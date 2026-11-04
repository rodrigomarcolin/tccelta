import NextAuth from "next-auth";
import Google from "next-auth/providers/google";
import { contaLiberada } from "@/auth/operadores";

/**
 * Auth.js v5, sessão JWT sem banco. A lista de operadores é lida a cada login
 * (runtime), nunca no build.
 */
export const { handlers, auth, signIn, signOut } = NextAuth(() => ({
  providers: [Google({ authorization: { params: { hd: "usp.br" } } })],
  session: { strategy: "jwt" },
  trustHost: true,
  pages: { signIn: "/entrar", error: "/entrar" },
  callbacks: {
    signIn({ profile }) {
      if (contaLiberada(profile, process.env.OPERATOR_EMAILS)) return true;
      return "/entrar?erro=conta_nao_liberada";
    },
  },
}));
