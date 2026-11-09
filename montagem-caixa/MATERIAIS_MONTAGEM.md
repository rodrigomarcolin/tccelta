# Materiais e montagem da caixa

O que comprar, imprimir e fazer para montar a caixa e prender a PCB nela. O projeto da caixa está no
[README](README.md) e as decisões em [`.spec/caixa-pcb/README.md`](../.spec/caixa-pcb/README.md). Os materiais da PCB
estão em [`montagem-kicad/MATERIAIS.md`](../montagem-kicad/MATERIAIS.md).

## Qual variante imprimir

| Variante | Pasta | Serve para | Tamanho externo (L x C x A) |
|---|---|---|---|
| `universal` | `universal/` | v1 **e** v2 (troca a placa sem reimprimir) | 66,9 x 92,4 x 52,1 mm |
| `v1` | `v1/` | só a v1 | 66,9 x 92,4 x 52,1 mm |
| `v2` | `v2/` | só a v2 (menor e mais baixa) | 66,9 x 83,5 x 39,6 mm |

Para a feira com as duas placas, imprima a `universal`. Os STL já saem na orientação de impressão.

## Peças impressas

| Peça | Arquivo | Qtd | Orientação na mesa | Suporte |
|---|---|---|---|---|
| Base | `base.stl` | 1 | fundo para baixo (como sai do STL) | **sim, só a partir da mesa** (ver abaixo) |
| Tampa | `tampa.stl` | 1 (+1 reserva para a feira) | teto para baixo (o "TCCELTA" fica nas primeiras camadas) | não |
| Inserto do cabo | `inserto.stl` | 1 | face externa na mesa | não |
| Espaçadores | `espacador_H*.stl` | 1 por furo usado | em pé | não |

Espaçadores por placa: v1 = H1, H2v1, H3 (3); v2 = H1, H2v2, H3, H4 (4). Na `universal` há os 5; imprima todos para poder
trocar de placa (cada um tem 0,1 g). O nome do arquivo diz o furo, mas os espaçadores são iguais entre si.

**Suporte na base:** as duas pontes das abraçadeiras ficam 4 mm abaixo do fundo, então a base apoia na mesa só por elas e o
fundo fica em balanço. Use suporte "somente tocando a mesa" e confira que o fatiador não pôs suporte dentro dos alojamentos
sextavados das porcas. Se preferir imprimir sem suporte, peça para tirar as pontes (o fundo fica plano e a caixa é presa
no carro com velcro ou fita dupla face).

### Filamento e configuração

| Item | Valor |
|---|---|
| Material | **PETG** (ou ASA). O PLA amolece a partir de ~55 °C, e o carro parado no sol passa disso. Todas as peças no mesmo filamento, opaco |
| Quantidade | `universal` ou `v1`: ~75 g. `v2`: ~60 g. Corpo de prova: ~20 g. Tampa reserva: ~40 g (`universal`) ou ~27 g (`v2`). Um carretel de 1 kg sobra |
| Bico / camada | 0,4 mm / 0,2 mm |
| Paredes | 5 perímetros (as paredes de 2 mm saem maciças, inclusive nos braços das travas) |
| Preenchimento | 30 % (quase não sobra volume para ele) |
| Texto em duas cores (opcional) | troca de filamento na camada 5 (0,8 mm) da tampa |

### Corpo de prova (antes da caixa inteira)

Imprima primeiro um trecho da parede da frente com a porta, o inserto, o furo e o berço do botão e uma trava (com o
pedaço de tampa correspondente). Confira e, se precisar, ajuste no topo de `scripts/gerar_caixa.py` e gere de novo:

| Conferir | Parâmetro |
|---|---|
| O inserto desliza nos trilhos sem folga | `FOLGA_ENCAIXE` |
| A tampa fecha com estalo e abre puxando com a mão | `ANG_SAIDA` (mais íngreme segura mais) |
| A porca entra no alojamento sob pressão e não gira | `ALOJ_SF` (5,65 mm entre faces) |
| O cabo passa justo pelo furo do inserto | `CABO_D` (8 mm) |
| A capinha passa pelo furo sem raspar | `BOTAO_CAPA_D` (11 mm) |

## Itens comprados

### Fixação da PCB

| Qtd | Item | Observação |
|---|---|---|
| 3 (v1) / 4 (v2) / 5 (`universal`) + reserva | **Porca sextavada M3** (DIN 934, 5,5 mm entre faces, 2,4 mm de altura) | fica presa por baixo do fundo, no alojamento sextavado |
| 3 / 4 / 5 + reserva | **Parafuso M3 x 10**, cabeça cilíndrica Allen (DIN 912, cabeça de 5,5 mm) | desce por cima da PCB. A cabeça de 5,5 mm é a que o README da PCB considera no furo H2, ao lado do F1. Phillips de cabeça panela (~6 mm) também serve se não encostar no F1 |
| (opcional) | Espaçador de nylon M3 de 4 mm | no lugar dos impressos (que têm 4,4 mm, porque 0,4 mm ficam no rebaixo do boss). Com o de 4 mm a placa fica 0,4 mm mais baixa, sem problema |

Comprimento do parafuso: PCB 1,6 + espaçador 4,4 + ~1,1 de plástico + porca 2,4 = 9,5 mm, então o M3 x 10 atravessa a porca
inteira. Com M3 x 12 a ponta passa ~2 mm do fundo da caixa. Não use M3 x 8: não chega à porca.

### Botão de pareamento

| Qtd | Item | Observação |
|---|---|---|
| 1 | Botão táctil 12 x 12 x 7,3 mm | vai no berço por dentro da parede da frente |
| 1 | Capinha redonda Ø 11 mm | passa pelo furo de 11,6 mm, ~1 mm recuada |
| 2 | Fio dupont fêmea, ~15 cm | soldados em pernas de lados opostos do botão; ligam ao J5. 15 cm alcança o J5 da v1 (no meio da placa) e da v2 |
| 1 | Barra de pinos macho 1x2, 2,54 mm, **90°** (só v2) | J5 da v2, com os pinos apontando para a frente. Na v1 o J5 é a barra vertical |
| - | Termo-retrátil fino | isola as soldas nas pernas do botão |

### Cabo e fixação no carro

| Qtd | Item | Observação |
|---|---|---|
| 1 | Cabo OBD-II com conector J1962 macho, Ø 8 mm | cortado curto; o conector fica fora da caixa. Fios nos pinos 16, 4, 5, 6 e 14 (ver o README da PCB) |
| 1 | Abraçadeira de nylon até **2,5 mm de largura**, ~100 mm | alívio de tração: passa pelas duas fendas da orelha do inserto (3 x 1,6 mm) e aperta o cabo |
| 2 | Abraçadeira até **5 mm de largura** e 1,5 mm de espessura, 200 a 300 mm | prende a caixa no carro pelas duas pontes de baixo (túnel de 5,4 x 2 mm) |
| (opcional) | Velcro ou fita dupla face automotiva | alternativa às abraçadeiras; cole fora das pontes |

## Ferramentas

| Ferramenta | Para quê |
|---|---|
| Impressora 3D com PETG (mesa a ~80 °C) | as peças |
| Chave Allen 2,5 mm (DIN 912) ou Phillips | parafusos M3 |
| Alicate de bico | segurar a porca enquanto ela entra |
| Ferro de solda | botão e, na v2, a barra de 90° do J5 |
| Estilete e lixa fina | rebarbas, restos de suporte nos alojamentos |
| Alicate de corte | aparar as abraçadeiras |
| Paquímetro | conferir o corpo de prova |

## Montagem

1. **Limpe a base:** tire o suporte de baixo e confira que os alojamentos sextavados estão limpos.
2. **Porcas:** encaixe uma porca M3 por baixo em cada furo que a sua placa usa. Se estiver dura, coloque a porca na boca do
   alojamento, passe um parafuso por cima e aperte: ele puxa a porca para dentro. Tire o parafuso depois. Se ficar frouxa, uma
   gota de cola instantânea na borda (não na rosca).
3. **Espaçadores:** um sobre cada furo usado; o rebaixo do boss os centra.
4. **PCB:** com os componentes virados para cima, apoie a placa nos espaçadores e aperte os parafusos M3 x 10 por cima, sem
   forçar (o plástico sob a porca só trabalha em compressão, mas aperto demais racha a PCB).
5. **Cabo:** passe o cabo pelo furo do inserto, ligue os fios no borne, passe a abraçadeira fina pelas fendas da orelha e aperte
   sobre o cabo, deixando folga nos fios até o borne. Desça o inserto pelos trilhos da porta.
6. **Botão:** encaixe o botão por cima no berço, com a capinha pelo furo da frente, e ligue os fios dupont no J5.
7. **Tampa:** empurre até estalar nas quatro travas. Para abrir, puxe com a mão.
8. **No carro:** passe as abraçadeiras largas pelas pontes de baixo, ou use velcro. A caixa não deve ficar pendurada só pelo
   cabo. Evite encostar a parede leste (lado da antena) em metal.

Lembretes da PCB: JP1 com jumper na bancada e sem jumper no carro; nunca USB e 12 V ao mesmo tempo (o entalhe da parede
oeste dá acesso ao USB). Para trocar o ESP32 na feira, veja o [README](README.md#na-feira-trocar-o-esp32).
