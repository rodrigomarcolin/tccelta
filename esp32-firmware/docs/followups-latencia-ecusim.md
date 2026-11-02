# Follow-ups do teste de latência variável do ECUSim

Origem: roteiro de testes Parte 4 (J-00 a J-12), executado em bancada em 2 nov 2026 com o ECUSim
(`simulador/`, latência configurável por PID) e o dongle em `collector_twai_debug`. A latência do
ECUSim foi validada; o teste expôs dois comportamentos do **dongle** que não fazem parte daquela
entrega e ficam aqui como pendências. Nenhum deles foi corrigido.

Ambos só aparecem quando a ECU responde **depois do Tmax** (padrão 50 ms). Com uma ECU dentro do
prazo, nenhum dos dois ocorre.

## 1. O prazo de timeout é renovado por frames que não são a resposta esperada

### O que se observou (J-09, latência fixa de 55 ms e de 110 ms)

O Tmax é 50 ms, então uma leitura sem resposta deveria terminar em `dur` = 50. Mas:

| Rodada | Leituras `ok=0` | `dur` dos timeouts |
|---|---|---|
| 55 ms | 2324 de 2324 | 50 em ~28%; o resto entre 52 e 56, com pico em 55 e 56 |
| 110 ms | 650 de 655 | 50 em ~55%; muitos em 59 a 61; cauda até 99 |

Exemplo (rodada de 110 ms): `READ pid=10 ok=0 ... dur=96`.

### Causa (lida em `src/can/isotp/IsoTpClient.cpp`, `Client::requestAll`)

O prazo é de silêncio, não de tempo desde o pedido:

```cpp
uint32_t quietDeadline = millis() + timeoutMs;          // linha 192
...
if (!inRange(frame.id, respIdMin, respIdMax)) continue;  // linha 253
const uint8_t pciType = frame.data[0] & 0xF0;
quietDeadline = millis() + timeoutMs;                    // linha 256: renova o prazo
...
if (!matchesPrefix(&frame.data[1], len, expectedReply)) continue;   // só aqui descarta
```

Qualquer frame com ID na faixa de resposta de ECU renova o prazo para mais `timeoutMs`, **antes** de
o código conferir se o frame é a resposta do serviço/PID pedido. Um frame de outro PID, ou a
resposta tardia de um pedido anterior, é descartado depois, mas já estendeu a espera.

- Rodada de 55 ms: a resposta tardia do pedido anterior chega ~5 ms depois do início da nova
  leitura e a renova: 5 + 50 = 55 (o pico de 55 e 56 ms).
- Rodada de 110 ms: chegam várias respostas tardias na mesma leitura, cada uma renova o prazo, e o
  timeout chega perto de dois Tmax (99 ms).

A renovação foi pensada para o broadcast, em que várias ECUs respondem e o cliente espera o
silêncio depois da última. Para isso, renovar a cada resposta **válida** faz sentido; renovar por
um frame que será descartado não.

### Impacto

- O Tmax não é um limite rígido quando a ECU está lenta.
- O escalonador planeja a ocupação do barramento (`u=782` no `CONFIG`, que é a soma
  `leituras/s × Tmax`) supondo que uma leitura que falha ocupe no máximo o Tmax. Com leituras de até
  ~99 ms a ocupação real passa do planejado e o `late` das outras leituras cresce (visto: `late=86`
  num `DECIDE`).
- Só ocorre com a ECU acima do Tmax, ou seja, em operação degradada.

### Proposta

Renovar `quietDeadline` somente depois de o frame passar nos filtros (`matchesPrefix` para SF e FF,
e a correspondência de slot ativo para CF). Frames de outro PID ou fora da faixa não renovam.

Cuidados: o prazo de silêncio entre quadros consecutivos de uma mensagem multi-frame (`cfDeadline`)
já é tratado à parte; conferir que respostas negativas (`negativeService`) continuam funcionando.
Os testes nativos de `IsoTpClient` precisam de um caso novo: um frame de resposta com prefixo
errado não pode estender o prazo.

### Como validar

Repetir o J-09 (55 e 110 ms) com `analisar_latencia.py` e o `grep` do `dur` dos `ok=0` (o comando
está no cartão J-09 do artefato): o esperado é `dur` = 50 em praticamente todas as falhas.

## 2. Resposta tardia aceita como resposta de um pedido novo do mesmo PID

### O que se observou (J-09, 110 ms)

5 de 655 leituras vieram com `ok=1` e `dur` de 4 a 10 ms, com a ECU configurada para 110 ms. Uma
resposta em 4 a 10 ms é impossível; são respostas do **pedido anterior** do mesmo PID.

Primeiro caso (tempos em ms do dongle):

- pedido anterior do `0C` começou em 2746309 e estourou aos 50 ms;
- novo pedido do `0C` começou em 2746473;
- a leitura `ok=1` terminou em 2746479, 6 ms depois do novo pedido, ou seja, 170 ms depois do
  pedido anterior (110 ms de latência mais a fila do ECUSim, que é de uma thread só);
- a resposta que casou era a do pedido antigo, e o dongle a contou como a do novo.

Dos 5 casos, 4 foram do `0C` e 1 do `49`; nenhum do `0D`.

### Por que o `grep -v` do cartão não pegou

O cartão procurava leituras `ok=1` com valor diferente do esperado (`2EE0` no RPM, `50` na
velocidade). A resposta tardia é do mesmo PID e tem o mesmo valor (as variáveis são constantes na
bancada), então passa. O sinal é o `dur` baixo demais para a latência configurada.

### É um bug?

É uma **limitação do protocolo**. Uma resposta OBD-II não carrega identificador do pedido; o cliente
só pode conferir serviço e PID. A resposta tardia de um pedido anterior do mesmo PID é
indistinguível da resposta a um pedido novo. Um carro real com ECU lenta teria a mesma ambiguidade.

O `requestAll` já mitiga o caso em que a sobra chegou **antes** do pedido: ele esvazia a fila de
recepção antes de enviar (`while (_can->receive(stale)) {}`, linhas 171 e 172, com um comentário
descrevendo exatamente esse risco para pedidos seguidos do mesmo prefixo). Não há como tratar a
que chega **depois** de o pedido novo sair.

### Impacto

- O valor é real, mas de uns 170 ms antes: para RPM e velocidade o erro é desprezível; para uma
  variável rápida, o valor fica atrasado.
- O `dur` e o instante da leitura ficam errados. Se acontecer durante o `AT#TMAX`, a amostra de
  latência sai artificialmente baixa; nessa situação a ECU já está acima do Tmax, e a medição já
  mostra isso.
- Uma leitura que deveria falhar conta como sucesso e esconde um timeout (0,8% das leituras na
  rodada de 110 ms; a taxa real de falha era 100%).

### Proposta

Não corrigir no cliente ISO-TP. Opções, em ordem de custo:

1. **Documentar** como limitação conhecida (feito nos cartões J-09 e na tabela do roteiro).
2. **Reduzir a chance**: depois de um timeout, esperar um intervalo curto (sobre a ordem da latência
   observada) antes de reenviar o pedido para o mesmo PID, ou priorizar outro PID, para que a
   resposta tardia caia na folga e seja drenada. Custa tempo de leitura e só vale se o item 1 da
   seção anterior for corrigido primeiro; senão a espera prolongada o mascara.
3. **Detectar**: sinalizar no evento `READ` quando a resposta chega "cedo demais" em relação à
   latência recente do PID (por exemplo, `dur` abaixo de uma fração do p50 anterior). É
   heurístico e pode gerar falso positivo.

A seção 1 e a 2 se influenciam: ao corrigir a renovação do prazo, as leituras com timeout passam
a durar 50 ms em vez de até 99 ms, o que muda o intervalo entre pedidos do mesmo PID e, com isso,
quantas respostas tardias caem dentro da janela de outro pedido. Reexecutar o J-09 depois da
correção para medir o efeito.

## Onde estão os dados

- Roteiro: artefato "Estado Real TCCELTA", Roteiro · Parte 4 (casos J-09 e J-10).
- Logs da bancada (não versionados): `esp32-firmware/logs/lat-j09-55.log`, `lat-j09-110.log`.
- Análise: `simulador/analisar_latencia.py` (resumo do `READ dur` e do `TMAX_SAMPLE lat`).
