FROM node:22-alpine AS deps
WORKDIR /app
COPY package.json package-lock.json ./
RUN npm ci

FROM node:22-alpine AS build
WORKDIR /app
COPY --from=deps /app/node_modules ./node_modules
COPY . .
# readConfig exige a variavel com NODE_ENV=production; em runtime vale o env do conteiner.
ENV NEXT_TELEMETRY_DISABLED=1 ANALISE_FONTE_DADOS=mock
RUN npm run build

FROM node:22-alpine AS runtime
WORKDIR /app
ENV NODE_ENV=production NEXT_TELEMETRY_DISABLED=1 PORT=3000 HOSTNAME=0.0.0.0
COPY --from=build /app/.next/standalone ./
COPY --from=build /app/.next/static ./.next/static
USER 65532:65532
EXPOSE 3000
CMD ["node", "server.js"]
