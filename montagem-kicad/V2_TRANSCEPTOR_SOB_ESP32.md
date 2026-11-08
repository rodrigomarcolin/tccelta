# Versão 2: transceptor CAN deitado sob o ESP32

A v2 muda **uma coisa**: o módulo SN65HVD230 (Waveshare "SN65HVD230 CAN Board") deixa de ficar em pé num soquete e
passa a ficar **deitado sob o ESP32**, entre as duas fileiras de soquetes, soldado baixo na placa. O resto do
circuito é o mesmo da v1. A v1 continua intacta em [`tccelta/`](tccelta/); a v2 está em [`tccelta_v2/`](tccelta_v2/)
e é gerada por `tccelta_v2/scripts/gerar_placa_v2.py`.

**Custo:** o módulo precisa de uma modificação **irreversível** (veja "Retrabalho do módulo").

## O que muda em relação à v1 e por quê

| | v1 | v2 |
|---|---|---|
| Placa | 54,1 x 72,4 mm | **54,1 x 63,5 mm** (9 mm mais curta, mesma largura) |
| Módulo SN65 | em pé, em soquete fêmea 1x4, ~37 mm de altura | deitado sob o ESP32, ~4 mm de altura |
| CAN_H / CAN_L até o módulo | borne -> J3 -> 2 fios dupont trançados -> borne do módulo | **só cobre**, direto do borne da placa aos furos do módulo |
| J3 e jumpers dupont | sim | **não existem** |
| CAN_TX / CAN_RX | subiam da borda de baixo até o D21 e o D22 | **9,6 mm e 12,5 mm**, do módulo direto ao D21 e ao D22 |
| Altura da montagem | dominada pelo módulo em pé (~37 mm) | dominada pelo LM2596 no soquete (~20 a 25 mm, conforme o módulo) |
| Furos de fixação | 3 | **4** (o 4º fica embaixo à direita, perto do borne) |
| F1, D1, C1 | F1 e D1 embaixo, ao lado do borne; C1 em cima, ao lado do ESP32, colado no IN+ | **coluna da esquerda**, ao lado do ESP32 (veja abaixo) |

Por que vale a pena:

- **CAN por cobre.** Não há mais fio solto, conector que se solta nem jumper que desencaixa. É o ponto mais frágil da v1
  com o carro vibrando. O par CAN_H/CAN_L agora é um par de trilhas lado a lado (1,6 mm entre centros, sem
  cruzamento), com cerca de 43 mm (CAN_H) e 41 mm (CAN_L) do módulo ao borne.
- **Mais baixo e mais rígido.** O mais alto passa a ser o LM2596 no soquete; depois vêm o ESP32, o F1 (~12 mm), o C1 (~11 mm) e o borne (~10 mm). O
  módulo, de ~4 mm, fica preso por 6 pinos soldados na placa, e não mais plugado num soquete.
- **Placa menor.** Sem o soquete do módulo e sem o J3, a faixa de baixo só precisa do borne, do R1 e do JP1.
- **Coluna de proteção na esquerda.** Como na v1, o C1 fica colado no IN+ do buck (~8 mm de trilha). Aqui a coluna
  da esquerda sobe em ordem: **F1 (embaixo, a 8,2 mm da via +12V do borne, o mínimo de cobre sem proteção) -> D1 -> C1 (em cima, a ~8 mm
  do IN+ do LM2596)**. O capacitor de entrada fica colado no módulo que ele alimenta.
- A energia (coluna da esquerda, trilhas de 1,5 mm) não cruza nem anda rente ao par CAN.

### Onde fica cada coisa (mm; origem = pino 1 do ESP32, x cresce para leste, y para sul)

| Item | Posição |
|---|---|
| Módulo U3 (contorno 37 x 14) | x -40,5 a -3,5, y 4,43 a 18,43 (centro em -22,0 / 11,43) |
| Barra de 4 pinos | x = -5,5; de sul para norte: CAN_TX (y 15,24), CAN_RX (12,70), GND (10,16), 3V3 (7,62) |
| 2 furos do borne do módulo | x = -36,0; CANH em y 8,89 (norte), CANL em y 13,97 (sul) |
| Borne de 5 vias | linha dos pads em y = 31,1; x = -40,82 / -35,74 / -30,66 / -25,58 / -20,5 (+12V, GND, GND, CANH, CANL) |
| F1 / D1 / C1 | coluna x = -49,0: F1 pino 1 em y 31,1; D1 anodo em y 22,0 e catodo em 14,38; C1 (+) em y 0,0 e (-) em x = -46,5 |
| R1 / JP1 | R1 pino 1 em (-15,5; 28,56) e pino 2 em (-15,5; 31,1); JP1 pino 1 em (-9,5; 28,56) e pino 2 em (-9,5; 31,1) |
| Furos M3 | (-1,0; -22,0), (-1,0; -9,0), (-49,5; 34,9) e (-1,0; 34,9) |
| Placa | x -52,1 a 2,0; y -26,0 a 37,5 |

A antena e o corte de cobre depois de x = 1,0 continuam como na v1. O contorno do módulo, o corpo do ESP32 e a
ordem dos pinos estão desenhados na serigrafia.

## Como o módulo fica montado

- **Lado dos componentes para BAIXO** (virado para a placa). O SOIC do SN65 tem 1,75 mm e cabe no vão de 2,5 mm do
  espaçador.
- **Barra de pinos a leste** (lado do D22, onde o ESP32 tem a antena) e **furos do borne do módulo a oeste** (lado
  do USB).
- Com isso, de sul para norte a barra fica CAN_TX, CAN_RX, GND, 3V3, o que roteia sem nenhum cruzamento: CAN_TX
  vai para o D21 (sudoeste), CAN_RX vai para o D22 (sul), o 3V3 segue para oeste e entra no pino 3V3 do ESP32.
  Se o módulo for montado do outro lado (componentes para cima), a ordem inverte e **as trilhas se cruzariam**:
  não faça isso.
- O fundo da placa do ESP32 fica ~11 mm acima da placa-mãe (soquete fêmea de 8,5 mm + 2,5 mm de plástico). O módulo
  (~4 mm de altura, mais as pontas dos pinos aparadas) passa com folga.
- O módulo **não toca a antena**: a borda leste dele fica em x = -3,5, bem antes da zona proibida (x > 1,0).

## Retrabalho do módulo (irreversível)

Faça num módulo que você aceita estragar para voltar ao original. Use um módulo de reserva se tiver.

1. **Tire o resistor SMD de 120 ohms** (marcado "121"), como na v1 (seção "Antes de soldar" do
   [PASSO_A_PASSO.md](PASSO_A_PASSO.md)). Confira com o multímetro: entre CANH e CANL, resistência alta (kiloohms).
2. **Dessolde o borne de parafuso de 2 vias.** Com alicate de corte rente, corte o corpo plástico para soltar as
   pernas uma a uma; depois aqueça cada perna e puxe com a pinça. Limpe cada furo com malha dessoldadora e,
   se precisar, desentupa com uma agulha ou com a bomba de sucção. Não levante os pads.
3. **Dessolde a barra de 4 pinos de 90 graus**, do mesmo jeito (corte o plástico, solte pino por pino, limpe os
   furos).
4. **Monte os pinos novos.** Use pinos macho retos de 2,54 mm, **com o espaçador de plástico de 2,5 mm** (o plástico
   preto da barra comum):
   - uma barra macho 1x4 para os 4 furos da antiga barra;
   - **dois pinos soltos** (cortados de uma barra) com o espaçador, para os 2 furos do antigo borne (CANH e CANL, passo
     de 5,08 mm; não dá para usar uma barra 1x2 comum se o passo for 5,08).
   - Encaixe cada pino pelo **lado dos componentes** do módulo: o plástico fica entre o módulo e a placa-mãe, e a ponta
     **curta** (~3 mm) sai para baixo. A ponta **longa** atravessa o módulo.
   - Solde pelo outro lado do módulo (lado de cima na montagem), 2 a 3 segundos por pino.
   - **Apare as pontas longas** rente à solda (no máximo 1,5 mm acima do módulo), para não encostarem no ESP32.
5. Confira com o multímetro, com o módulo solto: nenhum pino em curto com o vizinho.

Não precisa isolar nada na face de cima da placa-mãe: ali é fenolite, sem cobre.

## Medidas a conferir com o paquímetro ANTES de corroer

Todas estão no topo de `tccelta_v2/scripts/gerar_placa_v2.py`, marcadas com `CONFERIR COM PAQUIMETRO`. Meça no módulo
já retrabalhado (ou no original, olhando os furos):

| Constante | Valor atual | O que medir |
|---|---|---|
| `SN65_COMPRIMENTO` | 37,0 mm | comprimento do módulo |
| `SN65_LARGURA` | 14,0 mm | largura do módulo |
| `SN65_BARRA_DIST_BORDA` | 2,0 mm | do centro dos furos da barra à borda curta do lado da barra |
| `SN65_BORNE_DIST_BORDA` | 4,5 mm | do centro dos furos do borne à borda curta do lado do borne |
| `SN65_BORNE_PASSO` | 5,08 mm | distância centro a centro dos 2 furos do borne (pode ser 5,0 mm) |
| `SN65_CANH_NORTE` | `True` | com o lado dos componentes para baixo e a barra a leste, o furo CANH é o do **norte** (veja abaixo) |
| `SN65_ORDEM_PINOS` | CAN_TX, CAN_RX, GND, 3V3 (sul -> norte) | ordem dos 4 pinos na montagem |

Além disso, confirme:

- A barra de 4 e o par de furos do borne estão **centrados na largura** do módulo (o desenho assume isso).
- Como saber qual furo é CANH: a serigrafia do módulo marca "CANH" e "CANL" no borne. Segure o módulo na posição de
  montagem (componentes para baixo, barra para a direita/leste, furos do borne à esquerda/oeste) e veja se o
  CANH está no furo de cima do desenho (norte). Se for o de baixo, mude `SN65_CANH_NORTE = False` e rode o script:
  as vias 4 e 5 do borne e o R1/JP1 trocam sozinhos (o rótulo do borne muda para CANL, CANH), sem cruzar trilhas.
- A ordem dos pinos da barra: segundo a documentação do módulo, com o lado dos componentes para você e a barra embaixo,
  da esquerda para a direita é 3V3, GND, CAN RX, CAN TX. Se o seu for diferente, o script para com erro: o
  roteamento precisa ser revisto.
- O **fundo do ESP32 não tem componentes no meio**. A maioria dos DevKits V1 tem só as pontas dos pinos soldados nas
  bordas, mas confira com a mão. As pontas desses pinos não podem ficar a menos de ~4 mm do módulo.

### Teste em papel 1:1 sobre o módulo retrabalhado

1. Imprima `tccelta_v2/fabricacao/teste_papel_1x1.pdf` em **100 %**.
2. Apoie o módulo retrabalhado no papel, **na posição de montagem** (componentes para baixo, os pinos apontando para o
   papel). O contorno fino do módulo está desenhado sob o contorno do ESP32.
3. Os **6 pinos** (4 da barra e 2 do borne) têm de entrar nos 6 furos do papel ao mesmo tempo. Se não entrarem, corrija
   as constantes acima e rode o script de novo.
4. Meça o papel: a placa deve ter 54,2 x 63,6 mm.

## Lista de materiais: diferenças em relação à v1

| Item | v1 | v2 |
|---|---|---|
| Barra macho 1x2 "J3" | 1 | **0** |
| Jumpers dupont fêmea trançados | 2 | **0** |
| Barra fêmea 1x4 (soquete do módulo) | 1 | **0** |
| Barra macho 1x4 reta com espaçador de 2,5 mm | 0 | **1** (nos 4 furos da antiga barra do módulo) |
| Pinos macho retos soltos com espaçador | 0 | **2** (nos 2 furos do antigo borne do módulo) |
| Parafuso M3 + porca | 3 | **4** |
| Fenolite de face simples | ~60 x 76 mm | **~60 x 68 mm** |

O resto é igual: ESP32 e sua barra fêmea 1x15 + 1x14, LM2596 com 4 soquetes e 4 pinos, R1, JP1 com jumper de encaixe,
F1, D1, C1 e o borne de 5 vias.

Pinos a soldar na placa: 29 (ESP32) + 4 (LM2596) + 6 (módulo) + 2 (JP1) + 2 (J5) + 2 (R1) + 2 (F1) + 2 (D1) + 2 (C1) +
5 (borne) = 56, mais os 4 furos M3 sem solda. Total de furos: 60 (veja `tccelta_v2/fabricacao/furacao.txt`).

## Diferenças na montagem

A fabricação (teste em papel, toner, corrosão, furação, serigrafia, proteção) é igual à da v1: siga o
[PASSO_A_PASSO.md](PASSO_A_PASSO.md) e o [PASSO_A_PASSO_SERIGRAFIA.md](PASSO_A_PASSO_SERIGRAFIA.md), trocando os
arquivos pelos de `tccelta_v2/fabricacao/`. Furos: 1,0 mm para os 6 furos do módulo, como os dos soquetes.

Ordem de soldagem (passo 12 do PASSO_A_PASSO):

1. D1, R1, JP1 (não há mais J3).
2. **O módulo U3, antes das barras do ESP32.** Encaixe os 6 pinos curtos nos furos, com o módulo **componentes para
   baixo**, as bordas paralelas ao contorno da serigrafia e o espaçador encostado na placa. Solde pelo lado do cobre.
   Confira que nenhum componente do módulo encosta na placa.
3. Barras fêmea do ESP32 (mesmo truque do ESP32 encaixado enquanto solda). Como o módulo já está no lugar, o ESP32
   encaixa por cima dele.
4. Soquetes e pinos do LM2596, C1, F1 (deixe o disco uns 3 a 4 mm acima da placa, como na v1: a cabeça do parafuso do furo
   de baixo à esquerda passa por baixo da borda do disco) e o borne de 5 vias.

Para inspecionar ou mexer no módulo depois de pronto, **é preciso tirar o ESP32**.

### Energização

Igual à v1, **sem os passos do transceptor em pé e dos jumpers dupont** (não existem mais). A conferência com o
multímetro da terminação continua a mesma, entre CAN_H e CAN_L no borne:

- só o dongle, com o JP1: cerca de **120 ohms** (o R1);
- bancada (simulador + JP1): cerca de **60 ohms**;
- carro, sem o JP1: cerca de **60 ohms** (os dois resistores do próprio carro).

Antes de encaixar o ESP32, com o módulo já soldado, meça também: sem o JP1, resistência **alta** entre CANH e CANL
(o resistor "121" saiu); e 3V3 do módulo sem curto com o GND.

## Cuidados e perdas

- **Retrabalho irreversível** do módulo (o borne e a barra originais são destruídos).
- **Módulo escondido sob o ESP32:** para ver ou trocar, tira-se o ESP32. Se o módulo queimar, trocar significa
  dessoldar 6 pinos da placa.
- Se as medidas do módulo (principalmente a posição dos furos) não baterem com as constantes, a placa precisa ser
  refeita antes de corroer. Por isso o teste em papel é obrigatório.
- A trilha de 3V3 ficou mais longa (~43 mm: do módulo, sob o ESP32, até o pino 3V3 do ESP32, no outro lado). A
  corrente do transceptor é de poucos mA; não é problema, mas é um caminho a lembrar se um dia o CAN falhar.
- No cobre, os rótulos do borne (+12V, GND, CANH, CANL) da v1 foram retirados da face de baixo: o par CAN passa onde
  eles ficavam. Os rótulos continuam na serigrafia.
- O plano de GND ficou com uma ilha grande sem cobre (entre o par CAN e o ESP32, a oeste do módulo): é normal, não há
  pad de GND ali e o script remove ilhas sem ligação.

## Como regenerar

```
C:/Users/davif/AppData/Local/Programs/KiCad/10.0/bin/python.exe montagem-kicad/tccelta_v2/scripts/gerar_placa_v2.py
```

Gera símbolos, footprints, esquemático, PCB e `tccelta_v2/fabricacao/`, e imprime o resultado do ERC, do DRC (com
paridade esquemático x PCB) e a conferência de escala 1:1 dos PDFs. Resultado atual: ERC 0, DRC 0 violações,
0 ligações pendentes, 0 divergências de paridade.
