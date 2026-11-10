# Caixa da PCB do TCCelta

Gerada por `scripts/gerar_caixa.py` (FreeCAD 1.1, `freecadcmd`), que lê o contorno e os furos direto dos `.kicad_pcb`.
Plano e decisões: [`.spec/caixa-pcb/README.md`](../.spec/caixa-pcb/README.md). O que comprar, imprimir e como montar:
[`MATERIAIS_MONTAGEM.md`](MATERIAIS_MONTAGEM.md).

## Gerar

```bash
CAIXA_VARIANTE=universal "$HOME/AppData/Local/Programs/FreeCAD 1.1/bin/freecadcmd.exe" montagem-caixa/scripts/gerar_caixa.py
```

`CAIXA_VARIANTE` = `universal` (v1 e v2 cabem), `v1` ou `v2`. Saída em `montagem-caixa/<variante>/`:
`caixa_tccelta.FCStd` (com planilha `Parametros` e os fantasmas das placas), STEP da montagem e um STL por peça **já na
orientação de impressão** (base de pé; tampa de teto para baixo, com o "TCCELTA" nas primeiras camadas; inserto com a face
externa na mesa; espaçadores de pé).

O script imprime o relatório de verificação: validade dos sólidos, interferência de cada peça com os fantasmas da v1 e da v2
(placa, componentes, botão, cabo), sólidos válidos, espaço dos dedos nas pontas do ESP32, distância mínima da antena
(≥ 1,5 mm) e alinhamento dos furos. Estado atual: **0 falhas nas três variantes**.

| Variante | Externo (L x C x A) | Peças |
|---|---|---|
| universal | 66,9 x 92,4 x 52,1 mm | base, tampa, inserto, 5 espaçadores |
| v1 | 66,9 x 92,4 x 52,1 mm | base, tampa, inserto, 3 espaçadores |
| v2 | 66,9 x 83,5 x 39,6 mm | base, tampa, inserto, 4 espaçadores |

## Montagem

1. Porca M3 por baixo, apertada no alojamento sextavado de cada furo usado pela placa (fica presa). Espaçador sobre o
   boss (o rebaixo o centra), PCB por cima e **parafuso M3 x 10 descendo por cima**, até a porca. A placa sai e entra só com
   a chave de fenda, sem virar a caixa.
2. Cabo OBD pelo furo do inserto; abraçadeira nas duas fendas da orelha, por baixo do cabo. O inserto desce pelos trilhos
   da porta, na parede da frente da base.
3. Botão no berço (entra por cima, pelas guias), capinha pelo furo da frente.
4. Tampa: empurre até estalar; abre puxando as quatro travas laterais com a mão. Entalhe na parede oeste para o USB.

## Na feira: trocar o ESP32

Com a tampa fora, o ESP32 fica inteiro acima da borda da base. Desligue os 12 V e o USB, segure o ESP32 pelas duas pontas
(USB e antena; o lado do LM2596 não dá para pegar) e puxe balançando de leve. Encaixe o novo com a antena para leste e
pareie de novo (botão da frente, 5 s): cada ESP32 tem chaves e bond próprios. O script confere esse espaço dos dedos.

## Valores assumidos (conferir e ajustar no topo do script)

- `CABO_D = 8,0` mm: **medido** (cabo OBD, 2026-11-09). O furo do inserto tem `CABO_D + 0,4` mm.
- `BOTAO_CAPA_H = 1,5` mm: falta medir a altura da capinha.
- `BOTAO_X = 56` (o plano dizia ~54): 54 colocava a guia do berço a 0,1 mm do conector dupont do J5 da v2.
- Alturas dos componentes sem medida (C1, F1, D1, J-x): estimativas nos fantasmas; as medidas dadas (ESP32, LM2596, SN65) estão
  no plano.
- `FOLGA_ENCAIXE`, `ANG_SAIDA` e a deflexão das travas: calibrar no corpo de prova.

## Pendente

Corpo de prova impresso, impressão da `universal` e montagem com as placas reais, renders PNG e a linha em
`montagem-kicad/MATERIAIS.md` (J5 da v2 passa a barra macho 1x2 de 90°).
