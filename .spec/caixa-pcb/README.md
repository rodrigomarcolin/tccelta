# Caixa da PCB do TCCelta (FreeCAD)

> **Status (2026-11-08): entregas 2 a 6 implementadas** em `montagem-caixa/` (script, STEP, STL, README). Faltam as medidas
> (cabo, capinha), o corpo de prova, a impressão e os renders. Este documento define as peças, a geometria de referência
> (tirada das PCBs), os parâmetros, as medidas que faltam e a ordem das entregas. `BOTAO_X` virou 56 (ver o README da caixa).

## Requisitos

| # | Requisito | Como o plano atende |
|---|---|---|
| R1 | "TCCELTA" escrito na caixa | Baixo-relevo no teto da tampa (seção "Tampa") |
| R2 | Lugar para o botão de pareamento | Furo e berço na **parede da frente da base**, ao lado da saída do cabo (seção "Botão") |
| R3 | Caixa superior removível para mostrar a PCB | Base (bandeja) + **tampa** em forma de caixa, presa por travas que abrem sem ferramenta |
| R4 | Saída do cabo que liga o TCCelta ao carro | Porta na parede da frente, alinhada com o borne, fechada pelo **inserto do cabo** |
| R5 | Cabo preso por abraçadeira; o conector OBD-II macho fica do lado de fora | O inserto tem furo para o cabo e, por dentro, orelha para abraçadeira (alívio de tração). O cabo é cortado curto: pouco dele fica para fora da caixa |
| R6 | Servir à v1 e à v2 | Variante `universal`: as duas placas cabem e usam os mesmos furos de topo |
| R7 | Reduzir para a v2 sem muitas mudanças | Tudo ao sul depende de 1 número e a altura de outro; inserto e espaçadores não mudam |

**Fora de escopo** (decidido em 2026-11-08): encaixar o conector OBD-II macho na caixa. O conector fica na ponta do cabo,
fora da caixa.

## Geometria de referência (tirada dos `.kicad_pcb`)

Coordenadas do KiCad, em mm (x para leste, y para **sul**), lidas de `montagem-kicad/tccelta/tccelta.kicad_pcb` e
`montagem-kicad/tccelta_v2/tccelta_v2.kicad_pcb`. As duas placas usam a **mesma origem**. "Frente" = **sul** (lado do
borne e do cabo).

| Item | v1 | v2 | Observação |
|---|---|---|---|
| Borda oeste / leste da placa | x 7,9 / 62,0 | igual | mesma largura (54,1) |
| Borda norte | y 14,0 | igual | |
| **Borda sul (frente)** | **y 86,39** | **y 77,49** | **única diferença de contorno (8,9 mm)** |
| H1 (M3) | (59,0; 18,0) | igual | |
| H3 (M3) | (59,0; 31,0) | igual | |
| H2 (M3) | (10,5; 83,79) | (10,5; 74,89) | = borda sul - 2,6 nas duas |
| H4 (M3) | não existe | (59,0; 74,89) | = borda sul - 2,6 |
| Borne J1+J2 (5 vias, fios entram pela frente) | x 16,7 a 42,0; y 74,8 a 84,6 | y 65,9 a 75,7 | desloca 8,9 |
| J5 (botão) | (32,0; 68,7), barra vertical | pinos em x 43,9 e 46,4, y 67,2; **barra de 90° apontando para a frente** | ver "Botão" |
| ESP32 (corpo + antena) | x 16,3 a **68,3**; y 37,0 a 65,8 | igual | **antena passa 6,3 mm da borda leste** |
| USB do ESP32 | ponta oeste do ESP32 (x ~16,3), y ~51,4 | igual | |
| LM2596 | x 12,3 a 55,8; y 14,8 a 36,3 | igual | |

### Alturas sobre o topo da PCB

Medidas do usuário (2026-11-08), lidas como **altura da peça sozinha**, somadas à pilha de soquete documentada em
`V2_TRANSCEPTOR_SOB_ESP32.md` (soquete fêmea 8,5 mm + plástico da barra macho 2,5 mm = 11 mm até o fundo do módulo).

| Peça | Medida | Altura sobre a PCB | Versão |
|---|---|---|---|
| ESP32 | 5 mm | 11 + 5 = **16 mm** | v1 e v2 |
| LM2596 | 14 mm | 11 + 14 = **25 mm** (conferir se o LM2596 também tem os 2,5 mm de plástico) | v1 e v2 |
| SN65 em pé | 28 mm | 8,5 (soquete) + 28 + ~1 = **~37,5 mm** | só v1 |
| Borne | | ~10 mm | v1 e v2 |
| USB do ESP32 (centro) | | ~14 mm | v1 e v2 |

Mais alto: **v1 ~37,5 mm** (SN65 em pé); **v2 25 mm** (LM2596). Se as medidas tiverem sido feitas a partir da PCB, e não da
peça sozinha, a tampa fica 9 a 11 mm mais baixa: basta mudar `ALT_COMP`.

Consequência para o projeto da caixa:

- **Âncora norte.** A placa encosta na parede norte e usa H1/H3, que são comuns. Tudo o que muda entre as versões fica
  **ao sul** e anda junto com a borda sul (`Y_SUL`).
- **Duas dimensões mudam da v1 para a v2:** o comprimento (`Y_SUL`: -8,9 mm) e a altura da tampa (`ALT_COMP`: -12,5 mm).
  A largura, a porta do cabo, o inserto e os espaçadores são iguais.

## Peças

```
            NORTE (âncora: H1, H3)
   +-----------------------------------------+
   | [LM2596]                       H1 o     |
   |                                H3 o     |
   | C1  [========= ESP32 =========]~~~~~~   |  <- antena: fechada; parede de 2 mm
 U |     [                         ]~~~~~~   |     igual às outras, sem detalhes
 S |     [=========================]         |
 B | F1 D1                                   |
   | H2v2 o [  borne 5 vias ] J5 R1 JP1 o H4 |  <- J5 de 90°: pinos para a frente (v2)
   | ......(borda sul v1).....  ||           |
   |       fios do borne        ||  dupont   |
   |            \\              ||           |
   |    [abraçadeira]          [botão]       |
   +----------[cabo]-----------( O )---------+
            FRENTE (sul)
```

| Peça | Função | Muda na v2 reduzida? |
|---|---|---|
| **Base** (bandeja) | fundo, paredes até a linha de divisão, bosses dos furos, porta do cabo, furo e berço do botão, ressaltos das travas, passadores de abraçadeira por baixo | sim (comprimento, bosses) |
| **Tampa** (caixa superior) | paredes e teto, "TCCELTA" no teto, ventilação, recorte do USB, braços das travas | sim (comprimento e altura) |
| **Inserto do cabo** | fecha a porta, deixa só o cabo passar, prende o cabo com abraçadeira | não |
| **Espaçadores** (4x) | tubos de M3 entre o boss e a PCB | não |

### Base

- Fundo de 2,0 mm e paredes de 2,0 mm. Folga de 1,0 mm entre a placa e a parede; a leste, 1,5 mm além da ponta da antena.
- **Bosses baixos (2 mm) em todos os furos da variante**, com rebaixo por baixo para a cabeça do parafuso M3. O parafuso
  sobe pelo fundo, passa pelo espaçador e pela PCB, e a porca fica por cima da placa (o README da PCB já conta com
  cabeça/porca de 5,5 mm em H2).
  - `universal`: H1, H3, H2v1, H2v2 e H4 (5 bosses). Monta-se espaçador só nos furos da placa usada.
  - Como o boss tem só 2 mm e o espaçador é solto, um boss sem uso fica 4 mm abaixo da PCB e não encosta em nada
    (no v1, o H2v2 cairia colado ao pad do D1 se fosse um pilar fixo; por isso os espaçadores são soltos).
- PCB: fundo em z = 8,0, topo em z = 9,6 (do lado de fora do fundo). Sobram ~6 mm para as pontas de solda.
- **Zona da frente** (`ZONA_SUL`, ~15 mm entre a borda sul da placa e a parede da frente): espaço para os fios do borne
  fazerem a curva, a abraçadeira do cabo, o conector dupont do J5 e o corpo do botão. Na `universal` com a v2 essa zona fica
  8,9 mm maior (os fios ficam um pouco mais longos).
- **Linha de divisão** base/tampa em `Z_DIVISAO` (~19 mm). Ela fica acima do furo do botão e abaixo do recorte do USB, para
  os dois caberem inteiros, cada um numa peça. Encaixe por rebordo (*lip*) de 1,0 x 2,0 mm com folga de 0,2 mm. O rebordo
  alinha a tampa; as travas a seguram.
- **Ressaltos das travas:** 4 ressaltos em rampa na face externa do rebordo, nas paredes leste e oeste, perto das pontas
  norte e sul. Ficam fora da faixa da antena (y 37 a 66) e longe do recorte do USB.
- Por baixo: dois passadores de abraçadeira e uma área plana para velcro/fita dupla face, para prender a caixa no carro.
  Com o cabo cortado curto, a caixa fica perto da porta OBD e não deve ficar pendurada só pelo cabo.

### Porta do cabo e inserto

- **Porta:** janela em U na parede da frente, aberta para cima, com dois trilhos verticais, **alinhada com o borne**
  (x ~29, centro do borne de 5 vias). O inserto desce pelos trilhos e a parede da frente da tampa o prende por cima.
- **Inserto do cabo:** placa com furo para o cabo OBD (diâmetro do cabo + 0,4 mm), centrado em z ~11. Por dentro, uma
  orelha para abraçadeira: a abraçadeira aperta o cabo contra a orelha, e puxar o cabo não puxa o borne.
- O inserto é separado da base para trocar só ele (~5 g) se o cabo for outro ou se a folga do furo não ficar boa. Ele é
  **igual em todas as variantes**.
- O cabo é cortado curto: pouco dele fica para fora, com o conector OBD-II macho na ponta.

### Botão

- Botão táctil **12 x 12 x 7,3 mm** com capinha redonda de **11 mm**, num **berço impresso por dentro da parede da frente da
  base**, ao lado da porta do cabo, logo à frente do J5 (decidido em 2026-11-08).
- Na v2, o **J5 é uma barra de 90° com os pinos apontando para fora da placa** (para a frente). O conector dupont dos fios
  do botão encaixa na horizontal e passa ~8 mm da borda sul. Pela posição do borne J2 (courtyard até x 42,0) e do R1 (desde
  x 49,0), a barra de 90° e o conector dupont cabem entre os dois.
- **Posição:** centro do botão em x ~54 (o conector dupont do J5 ocupa x ~42,6 a 47,7 e não fica atrás do botão), z ~11
  (quase na altura dos pinos de 90° do J5, z ~12). Bem longe da antena (>25 mm ao sul do ESP32).
- **Furo da capinha:** 11,6 mm (11 + 0,6 de folga), com a capinha **~1 mm recuada** em relação à face da parede, para não
  ser apertada sem querer (segura-se 5 s para parear).
- **Berço:** duas guias em L por dentro da parede seguram o corpo de 12 x 12 pelas laterais. Um batente por trás impede que
  o botão recue quando é apertado. As pernas e os fios soldados ficam livres atrás do batente.
- Na v1 o J5 é vertical, no meio da placa: os fios dupont vão por cima da placa até o mesmo berço.
- Fica na **base**: abrir a tampa não solta os fios do J5.

### Tampa

- Caixa aberta embaixo, com teto de 2,0 mm. Altura interna = topo da PCB + `ALT_COMP` + 3 mm de folga.
  Estimativa: **~52 mm** de altura externa total na `universal`/v1 e **~40 mm** na v2.
- **"TCCELTA" em baixo-relevo de 0,8 mm no teto**, ao longo do comprimento, com altura de letra proporcional ao
  comprimento da tampa (encolhe sozinho na v2). A tampa é impressa de **teto para baixo**: o texto fica nas primeiras
  camadas e sai sem suporte. Para duas cores, basta trocar o filamento na camada 5 (0,8 mm).
- Fonte: TTF em negrito (ex.: `C:/Windows/Fonts/arialbd.ttf`), ou a do `branding/`, se houver. Opcional: o ícone
  `branding/tccelta-icon.svg` ao lado do texto.
- **Ventilação:** fendas nas paredes norte e oeste, perto do LM2596. O F1 (PTC 0,4 A) segura menos corrente no calor
  (`ESCOLHAS_DO_PROJETO.md`), então a caixa não pode virar uma estufa.
- **Recorte do USB** (decidido, `JANELA_USB = True`): entalhe aberto na borda de baixo da parede oeste, na altura do
  conector micro-USB do ESP32 (y ~51,4; centro ~14 mm acima do topo da PCB, z ~23,5), para gravar o firmware sem abrir a
  caixa. O ESP32 fica ~9 mm para dentro da parede, então o entalhe tem de deixar entrar o corpo plástico do plugue
  (~12 x 9 mm, conferir com o cabo usado). O C1 (y até 43,2) e o D1 da v2 (baixo, ~3 mm) não atrapalham o plugue. Vale
  o aviso do README da PCB: **nunca USB e 12 V ao mesmo tempo**.
- **Fixação na base: travas de pressão, abre sem ferramenta** (decidido). 4 braços flexíveis na tampa descem por fora do
  rebordo da base e o dente engata nos ressaltos dela.
  - O dente tem rampa de entrada suave (~30°) e rampa de saída mais íngreme (~50°, parâmetro `ANG_SAIDA_TRAVA`): a tampa
    fecha empurrando e **abre puxando com a mão**, sem apertar trava nenhuma. O ângulo de saída regula a força: mais
    íngreme segura mais contra a vibração.
  - O braço é um trecho da própria parede, solto por dois rasgos verticais (comprimento ≥ 12 mm, deflexão ≤ 0,8 mm).
    Impresso em pé, ele flexiona contra as camadas: a deflexão pequena e o braço largo (~8 mm) evitam que ele rache.
- **Antena do ESP32 fechada dentro da caixa**, sem abertura. Ela passa 6,3 mm da borda leste da PCB; a parede leste fica
  1,5 mm além da ponta dela. A parede ali é igual às outras (2 mm, mesmo acabamento); a única regra é não pôr nada
  nesse trecho: nenhuma nervura, trava, texto, inserto metálico ou tinta metálica na faixa da antena (y 37 a 66). 2 mm de PETG/PLA quase não atenuam o sinal de 2,4 GHz;
  o que atrapalha o BLE é metal perto da antena, como o lugar onde a caixa for presa no carro.
- **Opaca, no mesmo material das outras peças** (decidido). A PCB é mostrada abrindo a tampa.

### Material

Todas as peças no mesmo filamento, opaco. **PETG** é o recomendado: o PLA amolece a partir de ~55 °C, e o carro
parado no sol passa disso. O PETG também aguenta melhor a flexão repetida das travas. Se a impressora fizer ASA, ele é
ainda melhor no calor e no sol. O texto pode sair em uma cor só (o baixo-relevo aparece pela sombra) ou em duas, trocando
o filamento na camada 5.

## Parâmetros (fonte da verdade no script)

| Parâmetro | universal | v1 | v2 reduzida | Origem |
|---|---|---|---|---|
| `VARIANTE` | `universal` | `v1` | `v2` | escolha |
| `Y_SUL` (borda sul da placa) | 86,39 | 86,39 | 77,49 | `.kicad_pcb` |
| `FUROS` | H1, H3, H2v1, H2v2, H4 | H1, H3, H2v1 | H1, H3, H2v2, H4 | `.kicad_pcb` |
| `ALT_COMP` (mais alto sobre a PCB) | 37,5 | 37,5 | 25 | medido (ver "Alturas") |
| `ZONA_SUL` (borda sul da placa até a parede da frente) | 15 | 15 | 15 | conferir com os fios |
| `PAREDE`, `FUNDO`, `TETO` | 2,0 | | | |
| `FOLGA_ENCAIXE` | 0,2 | | | calibrar na prova |
| `Z_DIVISAO` | ~19 | | | entre o furo do botão e o USB |
| `BOTAO_CORPO` | 12 x 12 x 7,3 | | | medido |
| `BOTAO_CAPA_D` | 11 | | | medido |
| `BOTAO_X`, `BOTAO_Z` | ~54, ~11 | | | |
| `CABO_D` | ? | | | **medir** |
| `JANELA_USB` | sim | sim | sim | decidido |
| `ANG_SAIDA_TRAVA` | 50° | | | calibrar na prova |
| `DEFLEXAO_TRAVA` | 0,8 | | | calibrar na prova |

Tamanho externo estimado (largura x comprimento x altura): **universal/v1 ~67 x 92 x 52 mm**; **v2 ~67 x 83 x 40 mm**.

Para reduzir para a v2: `VARIANTE = "v2"` e rodar o script de novo. A base e a tampa saem 9 mm mais curtas e a tampa
12,5 mm mais baixa; o inserto e os espaçadores são reaproveitados.

## Como será feito no FreeCAD

- **Script gerador** `montagem-caixa/scripts/gerar_caixa.py`, rodado no `freecadcmd` do FreeCAD 1.1
  (`~/AppData/Local/Programs/FreeCAD 1.1/bin/freecadcmd.exe`). É o mesmo modelo do `gerar_placa.py`: o script é a
  fonte da verdade, o `.FCStd` é binário e regerado.
- O script **lê o contorno e os furos direto dos `.kicad_pcb`** (sem números copiados à mão). Se a PCB mudar, a caixa
  acompanha.
- Modelagem por sólidos do Part (caixas, cortes, uniões, filetes); texto com `Part.makeWireString` extrudado.
  Um Spreadsheet `Parametros` no documento mostra os valores usados.
- **Fantasmas de verificação:** placa + envelope de cada componente (do courtyard e das alturas medidas), o botão com a
  capinha, a barra de 90° do J5 com o conector dupont e o cabo, para v1 e v2. O script confere e imprime, como o DRC da PCB:
  - interferência zero entre cada peça e os fantasmas (v1 e v2 na `universal`; v2 na `v2`);
  - furos da PCB alinhados com os bosses;
  - antena sem parede a menos de 1,5 mm;
  - espessura mínima de parede e ausência de balanço sem suporte na orientação de impressão.
- Saídas em `montagem-caixa/<variante>/`: `caixa_tccelta.FCStd`, STEP da montagem, STL/3MF de cada peça já na
  orientação de impressão, renders PNG.

## Medidas

| O que | Valor | Para quê |
|---|---|---|
| ESP32 sozinho | 5 mm | `ALT_COMP`, fantasmas |
| LM2596 sozinho | 14 mm | `ALT_COMP` da v2 |
| SN65 em pé | 28 mm | `ALT_COMP` da v1 |
| Botão | 12 x 12 x 7,3 mm, capinha Ø 11 mm | furo e berço |
| **Diâmetro do cabo OBD** | **falta** | furo do inserto |
| Altura da capinha (do topo do corpo ao topo da capinha) | falta | recuo da capinha no furo |
| Altura do LM2596 no soquete: confirmar se há o plástico de 2,5 mm nos pinos | falta | `ALT_COMP` da v2 |
| Pontas de solda sob a PCB | falta (assumido ≤ 4 mm) | altura do espaçador |
| Comprimento dos parafusos M3 disponíveis | falta | espaçador + boss + PCB + porca |
| Corpo plástico do plugue micro-USB do cabo usado | falta (assumido ~12 x 9) | tamanho do recorte do USB |
| Impressora: bico e se imprime PETG (ou ASA) | falta | folgas e travas |

## Entregas

1. **Medidas que faltam** (tabela acima).
2. **Esqueleto do script:** leitura dos `.kicad_pcb`, parâmetros, fantasmas v1/v2 e relatório de verificação, ainda
   sem peças.
3. **Base** com bosses, rebordo, ressaltos das travas, porta do cabo, furo e berço do botão.
4. **Inserto do cabo** e espaçadores.
5. **Tampa** com texto, ventilação, recorte do USB e travas.
6. **Verificação completa** nas três variantes e exportação (STEP, STL/3MF, renders).
7. **Corpo de prova impresso:** só um trecho da parede da frente com a porta, o inserto, o furo e o berço do botão e
   **uma trava** (com o pedaço de tampa correspondente), ~20 g, para calibrar `FOLGA_ENCAIXE`, `ANG_SAIDA_TRAVA`, o furo
   do cabo e o furo da capinha antes de imprimir a caixa inteira.
8. **Impressão da `universal`**, montagem com a v1, depois com a v2; ajustes.
9. **README** em `montagem-caixa/` (impressão, montagem, troca do inserto) e linha no `montagem-kicad/MATERIAIS.md`.
   Atualizar o BOM da v2: **J5 passa a ser barra macho 1x2 de 90°** (mesmos furos, footprint inalterado).

## Decisões

| Decisão | Escolha | Situação |
|---|---|---|
| Fixação da tampa | travas de pressão, abre puxando, sem ferramenta | decidido (2026-11-08) |
| Janela USB | sim, entalhe na parede oeste da tampa | decidido (2026-11-08) |
| Tampa | opaca, mesmo material das outras peças | decidido (2026-11-08) |
| Conector OBD-II | fora da caixa, na ponta do cabo; a caixa só tem o inserto do cabo com abraçadeira | decidido (2026-11-08) |
| Lugar do botão | parede da frente da base, ao lado do cabo, à frente do J5 | decidido (2026-11-08) |
| J5 na v2 | barra de 90° com os pinos para a frente | decidido (2026-11-08) |
| Material | PETG (ou ASA) em todas as peças | recomendação; depende da impressora |
