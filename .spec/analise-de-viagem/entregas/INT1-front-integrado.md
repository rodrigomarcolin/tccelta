# INT1 — Front integrado à API de viagem, sem client credentials

> **Status: especificação de algo a desenvolver.** Esta entrega liga o front da [FE1](FE1-front-mockado.md) aos endpoints da [API2](API2-endpoints-viagem.md), com a API em `ANALISE_SEM_AUTH=true`. As decisões de processo, endpoints e login estão em [`04-arquitetura.md`](../04-arquitetura.md) e não são rediscutidas aqui.

## Objetivo

Um operador entra com e-mail USP, cria uma viagem e vê a análise calculada pelo servidor, com dados reais de um dongle (ou do simulador). As telas da FE1 não mudam. Muda a origem dos dados.

## Escopo e fora de escopo

| Dentro | Fora |
|---|---|
| Implementação real da camada de acesso a dados no servidor do Next.js | Client credentials, Bearer, JWKS (INT2, API3) |
| URL da API em variável de runtime | Ranking, exclusão de viagem, lista de boots |
| Erros e estados reais, polling, recálculo de viagem provisória | Ajustes e seleção de trecho |
| Sessão conferida no servidor em todas as rotas | Alterar a API ou as telas da FE1 |
| Integração local de ponta a ponta e roteiro manual | Publicação e HTTPS definitivo do estande |

## Dependências

- **FE1:** telas, login USP e a interface `ViagensCliente` (`frontend/src/servidor/viagens/cliente.ts`), com a implementação mock.
- **API2:** `POST /v1/viagens`, `GET /v1/viagens/{id}` e `GET /v1/viagens/{id}/analise`, descritos em `04-arquitetura.md`, seção "Endpoints".
- **Backend:** `make up` em `backend/` (ver `backend/docs/ambientes.md`).

## Integração

### Camada de acesso a dados

- Uma interface, duas implementações: **mock** (FE1) e **real** (INT1). Só o servidor do Next.js as usa. O navegador fala apenas com as rotas do Next.js.
- A escolha vem de `ANALISE_FONTE_DADOS` (`api` ou `mock`), lida em runtime, com a regra da FE1: sem a variável, vale `mock` em desenvolvimento, e em `production` ela é obrigatória.
- A implementação real devolve o JSON da spec 02 sem reescrever campos e segue o retorno da FE1: `{ status, corpo }`, com o problema da API no corpo, mais `retryAfter` quando a API o enviar. Os route handlers repassam `Retry-After`. Falha de rede ou timeout vira um `503` sintético com `codigo: api_indisponivel`.
- Todo `fetch` ao backend tem timeout e `cache: "no-store"`.

### Variáveis (lidas em runtime, sem `NEXT_PUBLIC_`)

| Variável | Uso | Padrão em dev |
|---|---|---|
| `ANALISE_FONTE_DADOS` | `api` ou `mock` | `mock` |
| `ANALISE_API_URL` | Base da API, sem barra final | `http://127.0.0.1:8080` |
| `ANALISE_TIMEOUT_MS` | Timeout de cada chamada | `10000` |
| `ANALISE_DONGLE_ID` | Valor inicial do campo dongle da tela de processar (fixo na feira, mas editável) | sem padrão: obrigatória com `api` |
| `OPERATOR_EMAILS`, variáveis do NextAuth | Já definidas pela FE1 | — |

O servidor falha na subida se `ANALISE_FONTE_DADOS=api` e faltar `ANALISE_API_URL` ou `ANALISE_DONGLE_ID`. Não há `ANALISE_SEM_AUTH` no front: quem a liga é o backend.

## Comportamento

### Mapeamento de erros

| Origem | Código na API | Mensagem ao usuário | Ação |
|---|---|---|---|
| `POST` | `400 entrada_invalida` | Como na FE1: o `detail` acima do botão | Corrigir e reenviar |
| `POST` | `404 dongle_desconhecido` | Erro no campo dongle: "Dongle não cadastrado." | Corrigir o campo ou avisar a equipe |
| `POST` | `409 motorista_repetido` | Mensagem da FE1, no campo motorista | Foco no campo; o formulário é preservado |
| `GET` análise | `409 analise_indisponivel` | Não é erro: `viagem_status` decide a tela (ver "Estados") | — |
| `GET` | `404 viagem_desconhecida` | "Viagem não encontrada." | Link para a tela de processar |
| Qualquer | `429` | "Servidor ocupado. Tentando de novo em N s." | Respeita `Retry-After`; no polling, adia o próximo ciclo |
| Qualquer | `503` | "Servidor sobrecarregado." | Polling continua com recuo; o `POST` mostra "Tentar de novo" |
| Qualquer | API fora do ar ou timeout | "Não foi possível falar com o servidor." | "Tentar de novo"; no polling, ver "Rede" |
| Qualquer | `5xx` sem `codigo` ou corpo inválido | "Erro inesperado no servidor." | "Tentar de novo". O detalhe técnico vai só ao log do servidor |

O `POST` **não** é repetido sozinho: repetir criaria uma segunda viagem ou um falso `409`. Quem decide é o operador. O corpo de erro da API é `application/problem+json` (`internal/http/problem.go`, campo `codigo`); a camada lê `codigo`, nunca `title` ou `detail` para decidir.

### Estados da tela de viagem

| `status` da API | `provisoria` | Tela |
|---|---|---|
| `pendente` ou `aguardando_dados` | — | Espera, com polling de 1 s |
| `pronta` | `true`, com dados | Análise completa e faixa de resultado provisório da FE1 |
| `pronta` | `true`, sem dados (aviso `sem_leituras`) | "Aguardando dados" (spec 04, "Viagem vazia") |
| `pronta` | `false`, sem dados (aviso `sem_leituras`) | "Sem dados para esta viagem" (spec 01) |
| `pronta` | `false`, com dados | Análise completa |
| `falhou` | — | Mensagem da FE1. O nome fica livre (spec 04) |

Os textos são os da FE1, inclusive o de `dados_parciais`, na faixa de avisos, pois as telas não mudam. Os estados de GPS e sinais da spec 01 valem sem mudança.

### Polling

- **Intervalo:** 1 s, no navegador, contra `GET` do Next.js (que chama `GET /v1/viagens/{id}`). Ciclos não se sobrepõem: o próximo só é agendado após a resposta anterior.
- **Para quando:** `pronta`, `falhou`, `404`, o limite de espera, ou a aba sair da tela.
- **Aba oculta:** o polling pausa (`visibilitychange`) e faz uma consulta imediata ao voltar.
- **Sair da tela:** desmontar cancela o timer e a requisição em voo. A viagem continua no servidor; reabrir o link retoma.
- **Rede:** falha de rede ou `5xx` conta como tentativa falha. Três seguidas mostram "Sem conexão com o servidor. Tentando novamente…" sem zerar a tela. O intervalo cresce 1, 2, 4, até 5 s, e volta a 1 s no primeiro sucesso. O evento `online` do navegador força uma tentativa.
- **Limite de espera:** 60 s em `pendente` (o limite final do worker é 20 s, spec 04). Passado isso, o polling para e a tela diz: "A viagem ainda não foi processada. O processamento no servidor pode estar parado." Botões: "Continuar esperando" (mais 60 s) e "Voltar". Em `aguardando_dados` o mesmo limite vale, contado do início da espera. **Decisão:** 60 s é 3 vezes o limite final, folga para a fila sem prender o visitante.
- **Recálculo de viagem provisória:** a tela de análise reconsulta `GET /v1/viagens/{id}` a cada 5 s enquanto `provisoria = true`, por no máximo 5 min, e só com a aba visível. Se `calculada_em` mudar, busca a análise de novo e a troca inteira, com aviso discreto "Resultado atualizado às HH:MM". Se `provisoria` virar `false` sem mudança em `calculada_em`, só remove a faixa. A tela nunca fica em branco entre duas versões. **Decisão:** 5 s acompanha o ticker do worker e a FE1; 5 min cobre o atraso típico de BLE e rede. Depois disso, a faixa passa a "Resultado provisório. Recarregue para verificar".

### Sessão no servidor

- Todas as rotas do Next.js que chamam a API conferem a sessão e a lista `OPERATOR_EMAILS` dentro do handler, antes de qualquer chamada. Sem sessão: `401`; fora da lista: `403`. O corpo da requisição não é lido antes disso.
- As rotas de status e de análise só repassam o UUID validado (formato), nunca um caminho livre.
- Sessão expirada durante o preenchimento: o formulário mostra "Sessão expirada" e oferece login sem perder os campos.

## Ambiente

### Subir localmente

1. `cd backend && make up`. Sobe API (portas `127.0.0.1:8080-8081`), worker, banco, KMS, Vault e Redis em HTTP (`compose.yaml`).
2. Na `backend/.env`, defina `ANALISE_SEM_AUTH=true` e suba de novo a API. A variável só vale fora de `AMBIENTE=producao`.
3. No front: `ANALISE_FONTE_DADOS=api`, `ANALISE_API_URL=http://127.0.0.1:8080`, `ANALISE_DONGLE_ID=<id>`.
4. Se o front rodar em contêiner, entre na rede `borda` e use `http://api:8080`. No `prodlike` a API não publica porta no host: o front precisa do Caddy (`https://localhost:8443/api`, certificado de laboratório, `NODE_EXTRA_CA_CERTS`). Esse caminho fica fora da INT1; o alvo é o `dev`.

### Gerar uma viagem real com o simulador

Há dois caminhos, com finalidades diferentes:

| Caminho | PIDs | Serve para |
|---|---|---|
| **Tela cheia:** BeamNG → `esp32-firmware/beamng/socket/tccelta_bridge.py` → ECUSim (`simulador/`) → dongle → app → API | Os sete da spec 02 | Viagem completa, com o fluxo real dos dados |
| **Só integração:** `make scenario C=<nome>` (`backend/docs/cenarios-simulador.md`) | `0x0C`, `0x0D` e `0x04`, com uma posição por bloco (`dongle.go`) | Testar a integração sem hardware e os estados "sem leitura de um sinal" |

**Tela cheia:**

- O coletor do firmware já lê os sete PIDs (`CollectorConfig.cpp`), e o bridge os envia ao ECUSim. Sem BeamNG, os valores podem ser definidos à mão com `simulador/send_payload.py`.
- O BeamNG não expõe MAF: o bridge o estima a partir da rotação e da pressão do coletor, com premissas próprias. O consumo nessa viagem é uma estimativa sobre uma estimativa e não valida o método.
- O GPS vem do app. Para a rota acompanhar o carro simulado, use `esp32-firmware/beamng/gps/tccelta_gps_bridge.py`.
- Para provocar `dados_parciais`, interrompa o envio do app durante a viagem e crie a viagem antes de religar.

Para obter `ANALISE_DONGLE_ID` e `boot_id`, consulte `dongle` e `boot` no banco (`localhost:5432`, papel `tccelta_owner`; ver `backend/README.md`). Dongles do CSV só funcionam em dev, no `prodlike` o KMS não os registra.

## Testes

- **Unidade da camada real**, com servidor HTTP falso: um caso por linha da tabela de erros, incluindo timeout, corpo não JSON e `Retry-After`.
- **Unidade do polling**, com relógio falso: intervalo, recuo, pausa em aba oculta, limite de 60 s, "Continuar esperando" e cancelamento ao desmontar.
- **Contrato:** as respostas de exemplo de `pronta` (completa, provisória, vazia) validam contra o JSON da spec 02. Os mesmos exemplos servem ao mock da FE1, para os dois serem intercambiáveis.
- **Rotas do Next.js:** sem sessão devolvem `401`, fora da lista `403`, e em ambos a API falsa não recebe chamada.
- **E2E local (manual ou Playwright):** os roteiros abaixo, com a stack real.

### Roteiro de verificação manual

| # | Passo | Esperado |
|---|---|---|
| 1 | Login com conta USP real, liberada em `OPERATOR_EMAILS` | Entra. Se o Google bloquear o app, registre a mensagem; o administrador do Workspace da USP precisa liberar o app |
| 2 | Login com conta USP fora da lista e com conta pessoal | Recusado |
| 3 | Conferir o redirect URI no console do Google e na URL pública do front | Idênticos, em HTTPS fixo; o login em `localhost` e no domínio do estande foram testados |
| 4 | Criar viagem por boot com dados do ECUSim (caminho "tela cheia") | `202`, espera, análise em segundos, com os sete PIDs nos gráficos |
| 5 | Repetir o mesmo nome e a mesma categoria | `409`: mensagem no campo, formulário preservado |
| 6 | Dongle inexistente em `ANALISE_DONGLE_ID` | Mensagem de configuração, sem detalhe técnico |
| 7 | `docker compose stop worker` e criar viagem | Aos 60 s, aviso honesto; "Continuar esperando" funciona; religar o worker conclui |
| 8 | `docker compose stop api` durante a espera | "Sem conexão…"; ao religar, a tela se recupera sozinha |
| 9 | Desligar a rede do navegador, religar | Tentativa imediata ao evento `online` |
| 10 | Criar viagem sem dados novos até os 20 s | `pronta` com `dados_parciais`, faixa de provisória |
| 11 | Enviar mais blocos de uma viagem provisória | Em até 10 s a tela atualiza e mostra "Resultado atualizado às HH:MM" |
| 12 | Sair da tela durante a espera e voltar pelo link | Retoma sem criar outra viagem |
| 13 | **Segurança:** a API com `ANALISE_SEM_AUTH=true` não pode ter porta pública | Confirmar que `8080-8081` estão em `127.0.0.1`, que o Caddy do estande não expõe `/api/v1/viagens` à internet, e que o túnel (`up-tunnel`) não está ativo |

## Critérios de aceite

1. Com `ANALISE_FONTE_DADOS=api`, o front cria uma viagem e exibe a análise real, sem alterar nenhuma tela da FE1.
2. Com `mock`, ou sem a variável em desenvolvimento, o front funciona só com o mock e não faz chamada de rede à API. Em `production`, sem a variável, não sobe.
3. Nenhuma variável de API ou de segredo usa `NEXT_PUBLIC_`, e a URL da API não aparece no código entregue ao navegador.
4. Mudar `ANALISE_API_URL` sem recompilar muda o destino das chamadas.
5. As rotas de criação, status e análise devolvem `401` sem sessão e `403` fora de `OPERATOR_EMAILS`, sem chamar a API.
6. Cada linha da tabela de erros tem teste e produz a mensagem e a ação descritas; `409 motorista_repetido` preserva o formulário.
7. O `POST` nunca é repetido automaticamente.
8. O polling faz uma requisição por vez, a cada 1 s, para em `pronta` e `falhou`, e é cancelado ao sair da tela.
9. Três falhas seguidas mostram o aviso de conexão e recuam até 5 s; o primeiro sucesso volta a 1 s.
10. Após 60 s sem sair de `pendente`, o polling para, a mensagem sobre o processamento aparece e "Continuar esperando" retoma por mais 60 s.
11. Uma viagem `provisoria` mostra a faixa e, em até 5 s depois de uma nova versão (`calculada_em` diferente), troca a análise sem tela em branco.
12. Cada linha da tabela de estados tem tela correspondente, inclusive "Aguardando dados" e `dados_parciais`.
13. O roteiro manual passa nos itens 1 a 13, ou os desvios ficam registrados com a causa (em especial o bloqueio da USP no item 1).
14. Uma viagem pelo caminho "tela cheia" mostra dados dos sete PIDs da spec 02 na tela.

## Lista de viagens na integração

- `GET /api/trips?limite=&cursor=&busca=` repassa a `GET /v1/viagens` só esses três parâmetros, já validados no BFF (`limite` de 1 a 50, `busca` até 60 caracteres, `cursor` opaco devolvido como veio). Valor inválido responde `400 entrada_invalida` sem chamar a API. Passa por `withSession` como as demais rotas.
- `useTrips` usa `useInfiniteQuery` (chave `["trips", busca]`, `getNextPageParam` por `proximo_cursor`) e refaz todos os lotes carregados a cada 5 s enquanto houver viagem `pendente` ou `aguardando_dados`. `useCreateTrip` invalida `["trips"]`.
- `409 viagem_repetida` (extras `viagem_id` e `motorista`) é repassado pelo BFF e tratado em `ProcessForm`.
- Verificação manual extra: abrir `/viagens`, rolar até carregar mais de um lote sem repetir itens, abrir uma viagem e voltar (mesma posição), buscar um motorista e tentar processar de novo um boot já processado.
