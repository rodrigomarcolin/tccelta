# FE1 — Front com login USP e tela de análise, com dados mockados

> **Status: especificação de algo a desenvolver.** Esta entrega cria o `frontend/`: login do operador, tela de processar e tela de análise completa. Não existe API real nem autenticação na API. Os dados vêm de um mock no servidor do Next.js. As decisões de [`04-arquitetura.md`](../04-arquitetura.md) não são rediscutidas aqui. A tela segue [`01`](../01-tela-mapa-velocidade.md) e os JSON seguem [`02`](../02-calculos-e-contrato-json.md).

## Objetivo

Entregar o front final, navegável de ponta a ponta com viagens de exemplo. A [INT1](INT1-front-integrado.md) troca o mock pela API real sem mudar as telas.

## Escopo e fora de escopo

**Dentro:** login Google; tela de processar; espera com polling; tela de análise (seções 1 a 7 da spec 01); mock dos três endpoints; tema escuro e claro; impressão; testes.

**Fora (ver "Complementos" da arquitetura):**

| Item da spec 01 | Motivo |
|---|---|
| Botão e painel **Ajustes** (combustível e avançado) | Combustível vem da criação; parâmetros ficam nos padrões |
| Seleção de trecho: arrastar, "Trecho m:ss–m:ss · limpar", esmaecer o resto, resumo do trecho, duplo clique para restaurar | Depende de `de_t_ms`/`ate_t_ms` |
| Estado "resumo anterior esmaecido ao atualizar trecho" | Mesmo motivo |
| Critérios 9 (parte de seleção) e 10 | Complemento da arquitetura |
| Cabeçalho com botão Ajustes | Substituído pelo seletor de tema |

Também fora: ranking, exclusão, lista de boots, personalizado (AFR e densidade), API real, client credentials.

## Dependências

Nenhuma entrega anterior. Pré-requisitos externos: projeto OAuth no Google com redirect URI do ambiente, e o zip `App OBD2 design brief.zip` (fora do repositório).

## Tecnologia

| Tema | Escolha | Motivo |
|---|---|---|
| Framework | Next.js estável vigente, App Router, TypeScript, runtime Node, versão fixada | Decisão da arquitetura |
| Login | Auth.js v5 (`next-auth@5`), provedor Google, sessão JWT sem banco | Padrão para o App Router; não precisa de banco |
| Mapa | Leaflet, só no cliente (`dynamic`, `ssr: false`), `preferCanvas` | Polilinhas coloridas por segmento sem custo de licença ou token |
| Mapa-base | Tiles raster CARTO `light_all` e `dark_all` (cinza, com atribuição visível) | Já são cinza; tema escuro e claro sem estilo próprio |
| Gráficos | uPlot, com `uPlot.sync` para o cursor | Séries densas e `null` como lacuna; a ferramenta já usa |
| Barras e mapa de calor | HTML e SVG próprios | Poucos elementos; dispensa biblioteca |
| Fontes | `next/font/google` (Space Grotesk, JetBrains Mono), empacotadas no build | Sem internet no estande; `tokens/fonts.css` do zip **não** é copiado |
| Testes | Vitest, Testing Library, jsdom | Unidade e tela sem navegador |

## Estrutura

```text
frontend/
  src/
    app/
      entrar/page.tsx              login
      processar/page.tsx           protegida
      viagem/[id]/page.tsx         protegida: espera, análise e erros
      api/viagens/route.ts         POST (protegida)
      api/viagens/[id]/route.ts    GET status (protegida)
      api/viagens/[id]/analise/route.ts   GET análise (protegida)
      api/auth/[...nextauth]/route.ts
    auth.ts                        Auth.js e allowlist (função pura em auth/operadores.ts)
    contrato/                      tipos TypeScript do JSON (spec 02 + 04)
    servidor/viagens/
      cliente.ts                   interface ViagensCliente + escolha da implementação
      mock/                        implementação mock, gerador e cenários
    componentes/{ui,icones,analise,processar}/
    formato/                       números, durações, horas, faixas (funções puras)
    estilos/tokens/                cópia adaptada do zip
  public/                          sem imagens de marca
```

### Camada de dados

`ViagensCliente` é a única porta para dados. Só os route handlers e server components a importam.

| Método | Imita | Retorno |
|---|---|---|
| `criar(entrada)` | `POST /v1/viagens` | `{ status: 202, corpo }` ou problema |
| `status(id)` | `GET /v1/viagens/{id}` | `200` ou `404 viagem_desconhecida` |
| `analise(id)` | `GET /v1/viagens/{id}/analise` | `200`, `409 analise_indisponivel` com `viagem_status`, ou `404` |

- O retorno é `{ status, corpo }`, com o JSON e o status HTTP que a API terá. Erros seguem `application/problem+json` de `backend/internal/http/problem.go` (`type`, `title`, `status`, `codigo`, `detail`).
- Os route handlers só repassam status e corpo, com `Cache-Control: no-store`. O navegador nunca chama outro host.
- `ANALISE_FONTE_DADOS` escolhe a implementação. Nesta entrega, só existe `mock`; `api` falha ao subir com "Cliente da API ainda não implementado (INT1)". Sem a variável, vale `mock` em desenvolvimento. Em `production`, ela é obrigatória, para o estande nunca mostrar dados de exemplo por falta de configuração.
- O mock guarda as viagens em memória (`globalThis`, para sobreviver ao recarregamento em dev).

### Mock

- **Progressão:** `pendente` até 2 s após `criada_em`, `aguardando_dados` até 5 s, depois `pronta`. Cenários podem trocar esses tempos.
- **Regras:** as validações e os erros do `POST` são os da [API2](API2-endpoints-viagem.md), inclusive limites de tamanho e categoria gravada em minúsculas. Nome repetido na categoria (sem diferença de maiúsculas e espaços) dá `409 motorista_repetido`. Entrada inválida dá `400 entrada_invalida`. `dongle_id` diferente de 1 dá `404 dongle_desconhecido`.
- **Escolha do cenário** (decisão desta spec, para o operador poder exercitar tudo pelo formulário):

| Entrada | Cenário |
|---|---|
| boot 8 | Boot normal |
| boot 9 | Sem GPS |
| boot 10 | GPS com falhas |
| boot 11 | Sem hora absoluta (`relogio`, `inicio_em`, `fim_em` nulos; aviso `sem_hora_absoluta`) |
| boot 12 | Sem leituras (aviso `sem_leituras`; fica `pronta` e `provisoria`, depois não provisória após 30 s) |
| boot 13 | Provisória aguardando dados: `pronta`, `provisoria: true`, sem leituras; após 30 s vira completa com dados |
| boot 14 | PID ausente: sem MAF (`0x10`) e sem nível do tanque (`0x2F`); aviso `cobertura_maf_baixa` |
| boot 15 | `dados_parciais`: com dados, `provisoria: true`, aviso `dados_parciais` |
| qualquer intervalo | Duas partes (modelo do JSON de intervalo da spec 02), com pausa longa |
| outro `boot_id` | Boot normal |

- **Viagens de exemplo fixas:** uma por cenário, com UUID fixo e nomes de motorista `Exemplo — <cenário>`, já `pronta`, para abrir por URL sem passar pelo formulário. A lista de UUIDs fica em `mock/cenarios.ts`.
- **Gerador:** um percurso sintético determinístico (semente fixa, perto de `-25.43, -49.27`) gera `series`, `rota`, `eventos`, `marchas`, `distribuicoes` e `qualidade` coerentes entre si (arrays paralelos, soma das faixas em 100%). Cada cenário é uma transformação da viagem normal. Os pontos separadores `null` entre partes seguem a spec 02.

## Variáveis de ambiente

Todas lidas em runtime, só no servidor, sem `NEXT_PUBLIC_`. As páginas que dependem delas usam renderização dinâmica (`dynamic = "force-dynamic"`), para que o build não as congele.

| Variável | Uso |
|---|---|
| `AUTH_SECRET` | Assina a sessão |
| `AUTH_GOOGLE_ID`, `AUTH_GOOGLE_SECRET` | Cliente OAuth do Google |
| `OPERATOR_EMAILS` | Lista separada por vírgula; espaços e maiúsculas ignorados; vazia ou ausente não libera ninguém |
| `ANALISE_FONTE_DADOS` | `mock` nesta entrega; a INT1 acrescenta `api` |

Um `frontend/.env.example` lista as cinco, sem valores. O fuso de exibição (`America/Sao_Paulo`) é constante em `config.ts`, porque a feira é local. O valor inicial do campo dongle também é (1), até a INT1 trocá-lo por `ANALISE_DONGLE_ID`.

## Design

### Origem dos tokens e ícones

O zip fica fora do repositório. Quem implementa extrai o zip em uma pasta temporária e copia:

1. `tokens/colors.css`, `typography.css`, `spacing.css` e `effects.css` para `frontend/src/estilos/tokens/`. Não copiar `fonts.css`. Em `typography.css`, `--font-ui` e `--font-mono` passam a apontar para as variáveis do `next/font`.
2. O SVG de cada ícone de `guidelines/iconography.card.html` para um componente em `componentes/icones/` (outline, traço 1,6 a 2, pontas e junções redondas, `currentColor`). Usados: `local`, `recarregar`, `info`, `sinal`, `check`, `chevron`, `dongle`, `voltar`.
3. A moldura de celular (`--radius-screen`, `--shadow-device`, `--bg-frame`) **não** é usada. Nada de `Gauge` nem `TabBar`.
4. `Button`, `StatusBadge` e `StatCard` são reescritos em TSX a partir de `components/*/*.prompt.md`, com altura de 40 px no desktop e 44 px abaixo de 640 px.

Faltam no conjunto os ícones de tema (sol e lua). Desenhar no mesmo estilo; nenhuma biblioteca de ícones e nenhum emoji.

### Tokens essenciais

Tema escuro (padrão), valores do design system:

| Token | Valor | Uso |
|---|---|---|
| `--bg-screen` | `#0A0E14` | Fundo |
| `--surface-card` / `--surface-list` | `#141B24` / `#10161E` | Cartões e linhas |
| `--text-primary` / `-secondary` / `-tertiary` | `#E8EDF2` / `#8B97A6` / `#5A6675` | Texto |
| `--border-hairline` / `--border-strong` | `rgba(255,255,255,.05)` / `.10` | Bordas de 1 px |
| `--accent` | `#22D3EE` | Ação principal, cursor, foco |
| `--warning`, `--danger` | `#FFB020`, `#F4385A` | Avisos e erros |
| `--font-ui`, `--font-mono` | Space Grotesk, JetBrains Mono | Texto e dados |
| Raios | 8, 12, 16 | Distintivo, linha, cartão |
| Espaços | 8, 10, 12, 14, 20, 28 | Ritmo denso |

**Tema claro** (decisão desta spec; o design system não tem um). Mesma lógica: neutros frios, hairlines, sem sombra, cor só com significado:

| Token | Valor |
|---|---|
| `--bg-screen` | `#F2F5F8` |
| `--surface-card` / `--surface-list` | `#FFFFFF` / `#F8FAFC` |
| `--text-primary` / `-secondary` / `-tertiary` | `#0F1720` / `#46525F` / `#5F6B79` |
| `--border-hairline` / `--border-strong` | `rgba(10,14,20,.08)` / `.16` |
| `--accent` / `--accent-on` | `#0A7C93` / `#FFFFFF` |
| `--warning`, `--danger` | `#9A5B00`, `#C0213F` |

Os valores do tema claro são ponto de partida: o critério 28 manda verificar o contraste. Os dois temas ficam em `[data-theme="escuro"]` e `[data-theme="claro"]`.

### Regras de aplicação

- **Layout web denso:** contêiner de até 1280 px, grade de 12 colunas, cartões de 14 px de padding. Abaixo de 900 px, uma coluna, com o mapa acima dos indicadores.
- **Dados em mono:** todo número, hora e PID usa `--font-mono` com `tabular-nums`. Rótulos usam `--font-ui`. Unidades ficam ao lado do valor, em `--text-secondary`.
- **Sem sombra.** Só hairline. Brilho cyan apenas no ponto de status "ao vivo" do polling.
- **Movimento contido:** transições de 0,4 a 0,55 s com `--ease-value`; nada além de `breathe` no ponto de espera. Respeitar `prefers-reduced-motion`.
- **Cor com significado:** cyan é ação e cursor; âmbar e vermelho só em aviso e erro; **verde não é usado em nenhum lugar** da análise (a regra do design system o reserva à zona normal de medidores, que não existe aqui). Série, barra e mapa de calor usam neutros e os matizes abaixo.
- **Tema:** cookie `tema` lido no servidor (sem piscar). Padrão `escuro`. Alternância no cabeçalho, com `aria-pressed`.
- **Rota:** rampas de cores e rótulos da spec 01, sem alteração. Contorno branco nos dois temas. Trecho sem dados é linha cinza fina. Marcadores de evento não usam âmbar nem laranja, para não se confundir com a rampa: círculo cheio com `--text-primary` (frenagem) e círculo vazio, de fundo de superfície e contorno `--text-primary` (aceleração), com legenda abaixo do mapa. O cabeçalho da análise é fixo e, com o mapa fora da tela, um minimapa no canto superior direito acompanha o cursor dos gráficos. Parada é círculo cinza de raio 6 a 14 px, proporcional à duração.
- **Marchas:** cores categóricas de até 6 marchas escolhidas com o skill `dataviz`, nunca verde-amarelo-vermelho; ponto morto em cinza.
- **Mapa de calor rotação × pedal:** um só matiz azul-acinzentado (o dos neutros frios), do mais claro ao mais escuro sobre a superfície, com legenda de tempo.
- **Impressão:** `@media print` força os tokens do tema claro, A4 paisagem, esconde cabeçalho de navegação e alternância, mostra a seção 7 aberta (o conteúdo é sempre renderizado, o recolhimento é só de tela) e mantém mapa e resumo na primeira página. Os tiles claros são usados na impressão.
- **Texto:** pt-BR, tom "honesto sobre o estado", sentenças curtas, sem emoji. Sem "infração", "excesso" ou "nota". Números no padrão pt-BR (`3,5 km`, `1.240 rpm`).

## Comportamento

### Telas e rotas

| Rota | Acesso | Conteúdo |
|---|---|---|
| `/entrar` | Aberta | Botão "Entrar com Google". Mostra o motivo se a conta foi recusada |
| `/processar` | Operador | Formulário e envio |
| `/viagem/[id]` | Operador | Espera, análise e erros |
| `/` | Operador | Redireciona para `/processar` |

Todo o front exige login. Só `/entrar` e as rotas do Auth.js ficam abertas. O visitante vê a própria viagem na tela do estande, com a sessão do operador.

### Login

- O callback `signIn` aceita só `email_verified`, domínio `usp.br` e e-mail em `OPERATOR_EMAILS` (regras da arquitetura). A função de decisão é pura e testada.
- A sessão é conferida em cada página (server component) e em cada rota de `/api/viagens`, não só no middleware. Sem sessão: a página redireciona para `/entrar` e volta ao endereço pedido depois do login; a rota responde `401` com problema.
- Se a sessão expirar durante o polling, o `401` leva a `/entrar`, e a viagem continua no servidor.
- Conta recusada volta a `/entrar?erro=conta_nao_liberada`: "Esta conta não está liberada para processar viagens. Use um e-mail USP autorizado."

### Processar

| Campo | Regra |
|---|---|
| Motorista | Texto obrigatório |
| Categoria | Texto obrigatório; valor inicial `feira` |
| Combustível | Seleção com os 6 presets da spec 02, mesmos nomes; inicial `gasolina_e30` |
| Dongle | Número; vem preenchido com 1 e pode ser editado |
| Entrada | Boot (`boot_id` obrigatório, `boot_generation` opcional) ou intervalo (`t0` e `t1` em data e hora; vão com fuso `-03:00`) |

- Validação no cliente: campos vazios e `t1 ≤ t0`. A do servidor continua valendo.
- `409 motorista_repetido`: erro no campo motorista, "Já existe uma viagem de «nome» na categoria «categoria». Use outro nome." Nada é perdido do formulário.
- `400 entrada_invalida`: mostra o `detail` acima do botão. `404 dongle_desconhecido`: erro no campo dongle. `401`: vai para `/entrar`. Outro erro: "Não foi possível criar a viagem. Tente de novo."
- `202`: vai para `/viagem/<viagem_id>`. O botão fica desativado durante o envio, para não duplicar.

### Espera e análise

`/viagem/[id]` consulta `GET /api/viagens/{id}` a cada 1 s enquanto o status for `pendente` ou `aguardando_dados`, com o ponto de status pulsando e a frase "Processando a viagem de «nome»". Ao ficar `pronta`, busca a análise e a mostra. `falhou` mostra o estado de falha. `404` mostra "Viagem não encontrada".

Uma viagem `provisoria` continua sendo consultada a cada 5 s, por no máximo 5 min. A tela mostra a faixa "Resultado provisório: ainda podem chegar dados" e atualiza sozinha quando a viagem deixa de ser provisória.

### Análise

Seções, ordem e conteúdo da spec 01, sem os itens fora de escopo. Só o novo:

| Elemento | Regra |
|---|---|
| Cabeçalho | Identificação por tipo de entrada (spec 01), motorista, categoria e alternância de tema. Em boot sem hora absoluta, omite data e hora |
| Eixo X | Hora absoluta (`deslocamento_ms + t_ms`) quando há `relogio`; senão, tempo desde `inicio_t_ms` em `m:ss` |
| Pausa entre partes | O eixo comprime cada pausa a 5% da soma das durações das partes; o rótulo "Sem dados · duração" mantém o tempo real. O cursor mostra a hora correta dos dois lados (transformação linear por trechos, com inversa) |
| Cursor e mapa | O cursor na rota localiza o ponto de `rota` mais próximo no tempo; o cursor no gráfico marca no mapa a posição desse instante, se houver GPS |
| Tooltip | Hora, velocidade, rotação, marcha e pedal do instante mais próximo; `—` onde for `null` |
| Legenda e tabela | A legenda é a distribuição (spec 01). A tabela alternativa (faixa, % da distância, tempo) abre pela legenda |
| Ver no mapa | Centraliza o evento e marca o instante na linha do tempo. Sem posição, o botão fica desativado com a dica "Sem posição de GPS" |
| Troca de marcha | Clicar no ponto marca o instante na linha do tempo |
| Seção 7 | Recolhida em tela; mostra `parametros` com o rótulo "limiares do projeto" |

**Avisos** (`avisos[].codigo`) aparecem em uma faixa abaixo do cabeçalho:

| Código | Texto |
|---|---|
| `sem_gps` | Sem posições de GPS nesta viagem: a rota não pode ser mostrada |
| `sem_hora_absoluta` | Esta viagem não tem hora absoluta. O eixo mostra o tempo desde o início |
| `boot_sem_hora` | Um boot ficou de fora por não ter hora absoluta (`detalhe`) |
| `partes_sobrepostas` | Duas partes se sobrepõem no tempo; a anterior foi cortada no início da seguinte |
| `sem_leituras` | Sem dados para esta viagem |
| `motor_nunca_ligado` | O motor não ligou nesta viagem. Os valores usam todos os dados |
| `cobertura_maf_baixa` | O consumo foi estimado com poucas leituras do sensor de ar |
| `consumo_pode_estar_subestimado` | Consumo pode estar subestimado: houve muito tempo com pedal alto |
| `marcha_lenta_reserva` | A rotação de marcha lenta usada é um valor de reserva |
| `dados_parciais` | Calculada com os dados que chegaram em 20 s. Pode estar incompleta |

Código desconhecido aparece com o texto "Aviso: «codigo»", sem quebrar a tela.

## Estados

| Estado | Comportamento |
|---|---|
| Espera (`pendente`, `aguardando_dados`) | Ponto de status, frase com o nome do motorista, sem dados parciais |
| Carregando a análise | Estrutura da página com áreas de espera |
| Erro ao carregar | Mensagem e botão "Tentar de novo" |
| `falhou` | "Não foi possível calcular esta viagem. O nome do motorista foi liberado." |
| Sem nenhum GPS | Spec 01 |
| GPS com falhas | Spec 01 |
| Mapa-base indisponível | Fundo liso cinza e legenda pequena "Mapa-base indisponível" no canto do mapa. Vale depois de falhar o carregamento dos tiles; nada cobre a tela |
| Sem hora absoluta | Spec 01 |
| Sem leituras, não provisória | "Sem dados para esta viagem", no lugar do resumo e dos gráficos |
| Provisória sem leituras | "Aguardando dados", com a faixa de resultado provisório |
| Sem leituras de um sinal | Gráfico com "Sem leituras" e indicadores `—`, nunca zero |
| Sessão ausente | Redireciona para `/entrar` |

## Critérios de aceite

1. **Allowlist:** `ana@usp.br` na lista com `email_verified` entra; `ana@gmail.com`, `ana@usp.br` fora da lista, `email_verified` falso ou lista vazia não entram. Maiúsculas e espaços na lista são ignorados.
2. **Rotas protegidas:** sem sessão, `/`, `/processar` e `/viagem/[id]` redirecionam para `/entrar`, e o `POST` e os `GET` de `/api/viagens` respondem `401`. Só `/entrar` e as rotas do Auth.js abrem sem sessão.
3. **Runtime:** trocar `OPERATOR_EMAILS` sem refazer o build muda quem entra. Nenhum segredo nem variável chega ao bundle do cliente.
4. **Camada de dados:** nenhuma tela ou componente importa `mock/`; só `cliente.ts` o faz. Em `production`, sem `ANALISE_FONTE_DADOS`, o servidor não sobe.
5. **Contrato:** cada cenário valida contra os tipos do JSON (arrays paralelos de mesmo tamanho, `null` e não `0` onde falta dado, separadores `null` entre partes).
6. **Progressão:** uma viagem criada passa por `pendente`, `aguardando_dados` e `pronta`, e `GET .../analise` antes disso responde `409 analise_indisponivel` com `viagem_status`.
7. **Erros do `POST`:** nome repetido (sem diferença de maiúsculas e espaços) dá `409`; entrada inválida dá `400`; dongle 2 dá `404`.
8. **Formulário:** `409` aparece no campo motorista sem apagar o resto; `400` aparece acima do botão; envio duplo é impedido.
9. **Polling:** a tela consulta o status a cada 1 s e abre a análise ao ficar `pronta`, sem recarregar a página.
10. **Exemplos:** com sessão, os 9 cenários da seção "Mock" abrem por URL e mostram o estado correspondente.
11. **Cabeçalho:** boot mostra `Dongle 1 · boot 8`; intervalo mostra `Dongle 1 · 28/10 08:00–18:00 · 2 partes`; sem hora absoluta, sem data e hora.
12. **Mapa:** com GPS, rota com a cor da faixa de cada segmento, contorno branco e marcadores A e B. Falha de GPS e ligação entre partes são linha cinza fina.
13. **Sem GPS:** sem mapa, resumo em largura total, aviso da tabela acima, seções 3 a 7 funcionam.
14. **Legenda:** faixas em movimento somam 100% (± arredondamento); "Parado" mostra tempo.
15. **Consumo:** todo litro ou km/L mostra "estimado" e o nome do combustível de `parametros.combustivel`.
16. **Linha do tempo:** os 4 painéis compartilham o eixo e o cursor; marcha 0 tem rótulo próprio, sem número e sem troca; marcha `null` fica vazia.
17. **Várias partes:** gráficos não ligam partes, a pausa mostra "Sem dados · duração", e uma pausa de 5 h ocupa 5% do eixo.
18. **Interações:** hover na rota e nos gráficos sincroniza mapa, cursor e tooltip; "ver no mapa" e clique na troca funcionam; botão sem posição fica desativado.
19. **PID ausente:** consumo, litros, km/L e nível do tanque mostram "Sem leituras" ou `—`, nunca zero; o aviso `cobertura_maf_baixa` aparece.
20. **Provisória e `dados_parciais`:** a faixa de resultado provisório aparece; "Aguardando dados" vira análise sozinha após 30 s no mock.
21. **Mapa-base indisponível:** com os tiles bloqueados, a rota aparece sobre fundo liso, sem mensagem cobrindo a tela.
22. **Fora de escopo:** não há botão Ajustes, seleção de trecho, "limpar" nem resumo de trecho.
23. **Linguagem:** nenhum texto usa "infração", "excesso" ou "nota"; nenhum verde na rampa nem na interface.
24. **Marca:** fontes via `next/font`; números em mono com `tabular-nums`; só ícones SVG próprios; nenhuma sombra; nenhum emoji.
25. **Tema:** escuro por padrão; a alternância persiste em cookie, sem piscar; a rota usa a coluna de cores do tema ativo.
26. **Impressão:** em tema claro, mapa e resumo na primeira página, legenda com nomes e porcentagens, seção 7 aberta.
27. **Tela estreita:** abaixo de 900 px, mapa acima dos indicadores e sem rolagem horizontal da página.
28. **Contraste:** com o skill `dataviz`, reverificar as 5 cores de faixa de cada tema contra `#0A0E14` e `#141B24` (escuro) e `#F2F5F8` e `#FFFFFF` (claro), contra os tiles `dark_all` e `light_all`, e contra o contorno branco (mínimo 2:1). Verificar também as cores de marcha, o matiz do mapa de calor e o texto dos tokens claros (mínimo 4,5:1). A rampa da spec 01 não muda: se alguma cor falhar, registrar a falha e ajustar o contorno ou a espessura, e não a cor.
29. **Testes de unidade:** formatação pt-BR (números, `3 min 55 s`, horas no fuso fixo, `—` para `null`), faixa de velocidade nos limites (2,9; 29,9; 60,0; 110,0), soma das faixas, compressão e inversa do eixo, allowlist.
30. **Testes de tela:** um por estado da tabela "Estados" e por cenário de exemplo, com mapa e uPlot trocados por dublês em jsdom. Cobrem também o formulário (`409`, `400`) e o polling com tempo simulado.
31. **Conformidade com a spec 01:** a tela passa nos critérios 1 a 8 e 11 a 17 da spec 01, e no 9 sem a parte de seleção. A tabela de eventos funciona por teclado.

## Decisões desta spec

| Decisão | Motivo |
|---|---|
| `viagem.motorista` na análise é `{ "id": "<viagem_id>", "nome": "..." }`, como na API2 | O mock e a API precisam ser intercambiáveis |
| Fuso de exibição fixo em `America/Sao_Paulo` | Feira local; evita depender do fuso do navegador |
| Marcadores de evento sem cor de aviso | O âmbar se confundiria com a rampa laranja; a forma já distingue |
| Provisória consultada a cada 5 s | Necessário para "Aguardando dados" virar análise; a arquitetura não define o intervalo |
| Seção 5 não compartilha cursor | A spec 01 só pede cursor compartilhado na linha do tempo |
| Pausa comprimida a 5% | A spec 01 pede "largura fixa e pequena" sem valor |

## Follow-ups

- **Licença dos tiles:** conferir o uso do CARTO na feira e a necessidade de internet no estande; trocar o provedor é mudar uma constante.
- **Sessão `Auth.js` v5:** reavaliar a versão estável ao iniciar a implementação.
- **INT1:** implementar `ViagensCliente` real e usar `ANALISE_FONTE_DADOS=api` no ambiente da feira.

## Lista de viagens (`/viagens`)

Acréscimo posterior a esta entrega. Telas e rotas novas: `/viagens` (lista) e `GET /api/trips` (BFF). `/` continua indo para `/processar`.

- **Navegação:** `AppNav` ("Viagens" e "Nova viagem", com `aria-current`) ao lado do botão de tema em `/viagens`, `/processar` e no cabeçalho de `/viagem/[id]`, sem aparecer na impressão. O link de voltar da análise passa a ser "Viagens" e o "não encontrada" aponta para a lista.
- **Lista:** cabeçalho fixo com título, total, atribuição do OpenStreetMap e busca por motorista (debounce de 300 ms, `?busca=` na URL, volta ao topo). Cards horizontais (minimapa de 220 px à esquerda, 110 px no celular), grade `auto-fill` de 2 colunas no desktop e 1 em telas menores, `content-visibility: auto` com tamanho intrínseco de 132 px. O card inteiro é um link para `/viagem/{id}`, sem prefetch.
- **Rolagem infinita:** `IntersectionObserver` num sentinela com `rootMargin` de 600 px, lotes de 24 por cursor, `role="feed"`. Falha de lote: "Tentar de novo" sem limpar a lista (3 tentativas automáticas antes). Sem `IntersectionObserver`: botão "Carregar mais". No fim: "Todas as N viagens foram carregadas."
- **Minimapa sem Leaflet:** tiles `<img loading="lazy">` (no máximo 4) com o trajeto em SVG por cima; projeção e posição dos tiles em `domain/trip/preview.ts`. Sem rota: "Sem GPS", "Processando" ou "Falhou".
- **Estados:** esqueletos, vazia ("Nenhuma viagem ainda"), busca sem resultado ("Limpar busca"), erro no primeiro lote (`ErrorState`), erro numa atualização (`ConnectionBanner`), 401 para `/entrar`.
- **Mock:** o datasource semeia 40 viagens além dos 9 exemplos (boots a partir de 100, rotas giradas) para a lista passar de um lote, e devolve `409 viagem_repetida` (com `viagem_id` e `motorista`) antes de `motorista_repetido`. Os boots 8 a 15 (geração 0) já pertencem aos exemplos.
- **Formulário:** `409 viagem_repetida` mostra "Esta viagem já foi processada para «nome»." com "Abrir a viagem" e "Ver todas as viagens".
