# INT2 — Front com client credentials

> **Status: especificação de algo a desenvolver.** Esta entrega faz o servidor do Next.js obter um token por client credentials e chamar a API com Bearer. Ao fim, o modo sem autenticação deixa de existir. Decisões de base: [`04-arquitetura.md`](../04-arquitetura.md), seções "Autenticação", "Bloqueio progressivo" e "Rotas abertas enquanto API3 e INT2 não chegam".

## Objetivo

Toda chamada do Next.js às rotas de viagem da API leva `Authorization: Bearer <token>`. O segredo e o token ficam só no servidor. Com segredo errado, a tela diz isso com clareza e o Next.js não insiste.

## Escopo e fora de escopo

**Dentro:** módulo de token só de servidor, cliente HTTP da API com Bearer, tratamento de `401`, `429`, timeout e indisponibilidade, retirada de `ANALISE_SEM_AUTH`, operação do segredo, testes.

**Fora:** login do operador (vem da FE1), mudança no contrato da API, ranking, exclusão de viagem.

## Dependências

| Entrega | O que a INT2 usa |
|---|---|
| INT1 | Cliente da API nos route handlers, estados de tela (carregando, erro, API fora do ar) e polling de 1 s |
| [API3](API3-client-credentials.md) | `POST /v1/auth/token` com HTTP Basic e `grant_type=client_credentials`, resposta com `expires_in` (30 min por padrão), `401 invalid_client`, `429` com `Retry-After`, `make seed-client` e os comandos `client create` e `client rotate` |

## Configuração e segredo

| Variável | Conteúdo | Regra |
|---|---|---|
| `ANALISE_API_URL` | Endereço da API | Só servidor. Já existe desde a INT1 |
| `ANALISE_CLIENT_ID` | `client_id` | Só servidor, lida em runtime |
| `ANALISE_CLIENT_SECRET` | `client_secret` | Só servidor, lida em runtime |

- Nenhuma das três tem `NEXT_PUBLIC_`. Elas são lidas de `process.env` na primeira chamada, não em tempo de build, para que a imagem não carregue o segredo.
- Variável ausente ou vazia gera erro de configuração claro, sem chamar a API.
- O módulo do token e o cliente da API começam com `import "server-only"`. Importá-los de um componente de cliente quebra o build.
- Os route handlers que chamam a API declaram `export const runtime = "nodejs"`. O Edge não é usado, porque o cache em memória e o `server-only` dependem do Node.

## Gerenciador de token

Um módulo único, `getToken()` e `invalidate()`, com estado no escopo do módulo (cache em memória).

| Regra | Comportamento |
|---|---|
| Cache | Guarda o token e o instante de expiração, calculado com `expires_in` da resposta |
| Folga | Renova quando faltam menos de 60 s para o `exp`, ou 20% da validade se esta for menor que 5 min |
| Deduplicação | Pedidos simultâneos compartilham uma só `Promise` de renovação. Ela é limpa ao terminar, com sucesso ou erro |
| `401` da API | Chama `invalidate()`, renova uma vez e repete o pedido original uma vez. Segundo `401` vira erro, sem novo ciclo |
| Pedido repetido | Só se repete o pedido que recebeu `401`. Não há repetição por timeout nem por `5xx` |
| Várias instâncias | Cada processo tem o próprio cache e o próprio token. Isso é aceito: cada token é válido por si, e o bloqueio é por `(client_id, IP)`. Na feira, usa-se uma instância. Com `N` instâncias, o IP é o mesmo e as falhas somam |
| Desenvolvimento | O estado do módulo fica em `globalThis`, para que o recarregamento a quente não perca o cache nem duplique renovações |

**Rotação da chave `cli-v<n>`:** o front não faz nada além do que já faz. A API valida pelo `kid`, e um token antigo continua válido até o `exp` (API3, critério 9). A renovação seguinte já vem assinada pela chave nova. O front não lê o JWKS nem o `kid`. Isso é verificado em teste (critério 8).

## Erros

| Situação | Comportamento do gerenciador | O que a tela mostra | Log do operador |
|---|---|---|---|
| `POST /v1/auth/token` com `200` | Guarda e usa | Nada | — |
| `401 invalid_client` em `POST /v1/auth/token` | **Não repete.** Entra em estado `credencial_invalida` por 60 s: as chamadas seguintes falham na hora, sem ir à API | "O servidor não aceitou a credencial do sistema. Chame a organização." | `error`: `client_id`, status, "verifique `ANALISE_CLIENT_SECRET`". Nunca o segredo |
| `429` com `Retry-After` | **Não repete.** Guarda `bloqueado_ate = agora + Retry-After` e falha na hora até lá | "Acesso temporariamente bloqueado. Tente de novo em N s." (N a partir de `Retry-After`) | `warn`: `Retry-After` e a causa provável (segredo errado) |
| `429` sem `Retry-After` válido | Usa 60 s | Igual ao anterior | Igual, com nota do valor assumido |
| Timeout do token (5 s) ou API fora do ar | Sem repetição automática. Próxima chamada tenta de novo, no ritmo do polling, no máximo uma tentativa por segundo no processo | Estado "API indisponível" da INT1, com botão de tentar de novo | `error`: causa |
| Timeout de pedido de viagem (10 s) | Sem repetição | Estado de erro da INT1 | `error`: rota e tempo |

- A tela de análise nunca fica em branco: todo erro do servidor vira um JSON de problema com `codigo` próprio (`credencial_invalida`, `bloqueado`, `api_indisponivel`), e a INT1 o converte em estado de tela.
- O polling de 1 s pausa enquanto o estado for `bloqueado` ou `credencial_invalida` e volta quando o prazo termina. Sem isso, o polling manteria o bloqueio.
- O estado `credencial_invalida` é limpo ao reiniciar o processo ou após 60 s. A cada limpeza vale uma única tentativa. Falhando, volta ao estado.
- Logs e respostas ao navegador não trazem segredo, token, nem o corpo cru do pedido de token.

## Fim do modo sem autenticação

1. Remover `ANALISE_SEM_AUTH` do código da API (`config`, `Router`), dos composes e dos `.env` de exemplo. O ramo sem autenticação deixa de existir.
2. Verificação: `grep -r ANALISE_SEM_AUTH` no repositório só pode achar texto histórico em `.spec/`.
   O cenário `trip-http` do `make e2e` passa a obter um token com um cliente de teste, cadastrado no `compose.test.yaml`.
3. Com a variável fora, `GET /v1/viagens/{id}` e `GET /v1/viagens/{id}/analise` sem Bearer respondem `401`, e `POST /v1/viagens` também. A comprovação é por `curl`, na stack real.
4. Todo o front continua protegido pelo login da FE1. O Next.js chama a API com o token do sistema.

## Operação

| Tarefa | Como |
|---|---|
| Cadastrar o cliente | Em dev, `make seed-client` (grava o segredo em `deploy/segredos/analise_cliente_secret`). Nos demais ambientes, `tccelta-kms client create` (API3). O segredo aparece uma vez, e o Vault guarda só o hash |
| Segredo em dev | `frontend/.env.local` (fora do git), com `ANALISE_CLIENT_ID` e `ANALISE_CLIENT_SECRET` do cliente cadastrado na stack de `make up` |
| Segredo na feira | Variável de ambiente do processo do Next.js no ambiente da feira, ou arquivo de segredos do orquestrador, fora do git e fora da imagem. Não vai para `NEXT_PUBLIC_` nem para o `.env` versionado |
| Trocar o segredo | `tccelta-kms client rotate`, atualizar a variável e reiniciar o Next.js logo em seguida. O segredo antigo deixa de valer na hora (API3), mas os tokens já emitidos valem até o `exp`. Sem reiniciar, a próxima renovação falha com `401 invalid_client` |
| Segredo errado em campo | O log mostra `credencial_invalida`. Corrigir a variável e reiniciar. Se houver `429`, esperar o `Retry-After` antes de tentar |

Em `prodlike` e `producao`, a API precisa de mTLS e CN para o KMS (ver [`backend/docs/ambientes.md`](../../../backend/docs/ambientes.md)). Isso é problema da API, não do Next.js, que fala HTTP(S) comum com a API.

## Testes

**Unidade do gerenciador** (relógio falso, `fetch` falso):

- Token dentro da folga é reutilizado; token na folga ou expirado dispara uma renovação.
- 50 chamadas simultâneas com cache vazio geram um só `POST /v1/auth/token`.
- `401` da API: uma renovação, uma repetição; segundo `401` não repete.
- `429` com `Retry-After`: nenhuma segunda tentativa até o prazo, mesmo com chamadas repetidas.
- Falha de credencial: nenhuma segunda tentativa em 60 s.
- Erro de rede na renovação limpa a `Promise` e permite a próxima tentativa.
- Variável ausente: erro de configuração sem chamada de rede.

**Bundle do cliente:** após `next build`, um teste varre `.next/static` e os chunks entregues ao navegador. Falha se achar `ANALISE_CLIENT_SECRET`, `ANALISE_CLIENT_ID`, `NEXT_PUBLIC_` ligado a eles, o valor do segredo de teste ou um JWT. O build do teste usa um segredo sentinela distinto.

**Ponta a ponta, com a stack real** (`make up` mais o Next.js):

1. Cliente cadastrado, segredo certo: processar uma viagem e ver a análise.
2. `curl` sem Bearer nas rotas de viagem: `401`.
3. Segredo errado: a tela mostra a mensagem de credencial, e os logs mostram no máximo uma tentativa a cada 60 s, mesmo com o polling ativo.
4. Bloqueio: provocado o `429`, a tela mostra o tempo restante, e o log do Next.js não registra novos pedidos de token até o prazo.
5. API parada: estado "API indisponível", sem tela em branco; ao voltar, a tela se recupera.
6. Rotação de `cli-v<n>` durante a operação (`clientjwt rotate --now`, da API3): o token atual continua aceito, a renovação seguinte vem com a chave nova, e a tela segue sem erro visível.

## Critérios de aceite

1. `ANALISE_CLIENT_ID` e `ANALISE_CLIENT_SECRET` são lidas em runtime, sem `NEXT_PUBLIC_`, e o módulo do token falha o build se importado no cliente.
2. O teste do bundle não acha nomes, valor do segredo nem JWT nos arquivos do navegador.
3. Toda chamada às rotas de viagem leva Bearer, e os route handlers rodam em `nodejs`.
4. Chamadas simultâneas com cache vazio geram uma só renovação.
5. O token é renovado antes do `exp`, com a folga definida.
6. Um `401` causa uma renovação e uma repetição, nunca mais.
7. Com `429` ou segredo errado, o Next.js não faz nova tentativa de token até o prazo; a tela e o log trazem a mensagem definida.
8. Rotação de `cli-v<n>` com a stack no ar não exige reinício nem mudança de código no front.
9. Com a API ou o token indisponíveis, a tela mostra um estado da INT1 e nunca fica em branco.
10. Nenhum log ou resposta ao navegador contém segredo ou token.
11. `ANALISE_SEM_AUTH` não existe fora do texto histórico da arquitetura, e as rotas de viagem sem Bearer respondem `401`.
12. A operação (cadastro, segredo em dev e na feira, troca) está descrita e foi seguida na verificação ponta a ponta.

## Decisões desta spec

- Folga de 60 s (ou 20% da validade) e timeouts de 5 s no token e 10 s nos pedidos: valores iniciais, a calibrar com a validade que a API3 definir.
- Em falha de credencial, quarentena de 60 s no processo: evita que o polling de 1 s faça cada visita aumentar o bloqueio.
- Sem retry automático em timeout ou `5xx`: cada tentativa extra pesa na API da feira, e o polling já repete.
