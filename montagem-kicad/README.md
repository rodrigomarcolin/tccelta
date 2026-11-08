# Dongle OBD-II (TCC) - placa de face simples feita em casa

Projeto KiCad 10 em `tccelta/`. A placa liga o cabo OBD-II (12 V + CAN) a um ESP32 e a um transceptor CAN
SN65HVD230 (placa Waveshare "SN65HVD230 CAN Board"), com um buck LM2596 para gerar os 5 V. Os **módulos não são
soldados**: encaixam em soquetes fêmea (barras de pinos fêmea de 2,54 mm). Só os soquetes, os bornes, o F1, o D1,
o C1 e a barra macho J3 são soldados.

Entrada de 12 V, na ordem: borne -> **F1** (fusível rearmável PTC 0,5 A) -> **D1** (1N5819, inversão de polaridade)
-> **C1** (100 uF) -> LM2596.

Tamanho da placa: **54,1 x 72,4 mm** (cantos arredondados de 2 mm). Cobre só em B.Cu, **sem vias, sem jumpers**.

O procedimento completo de fabricação e montagem está em [PASSO_A_PASSO.md](PASSO_A_PASSO.md), e a lista de
ferramentas e materiais em [MATERIAIS.md](MATERIAIS.md). A função e a posição de cada elemento estão explicadas em
[ESCOLHAS_DO_PROJETO.md](ESCOLHAS_DO_PROJETO.md).

## Arquivos

| Arquivo | O que é |
|---|---|
| `tccelta/tccelta.kicad_sch` | esquemático (A4, 5 blocos) |
| `tccelta/tccelta.kicad_pcb` | PCB roteada, plano de GND em B.Cu |
| `tccelta/tccelta.kicad_sym`, `tccelta/tccelta.pretty/` | símbolos e footprints próprios (ESP32, LM2596, SN65 em pé) |
| `tccelta/scripts/gerar_placa.py` | gera tudo do zero (símbolos, footprints, esquemático, PCB, exportações) |
| `tccelta/fabricacao/` | PDFs 1:1, furação, renders, esquemático em PDF/SVG |

## BOM (lista de materiais)

| Qtd | Item | Observação |
|---|---|---|
| 1 | DOIT ESP32 DevKit V1, 30 pinos (2 x 15) | encaixa em 2 soquetes fêmea de 15 pinos |
| 1 | Módulo buck LM2596 (placa azul, 43 x 21 mm) | você solda 4 pinos macho nos furos do módulo |
| 1 | Módulo "SN65HVD230 CAN Board" (Waveshare, ~37 x 14 mm) | montado **em pé**; já tem o resistor de 120 ohms |
| 1 | Barra de pinos **fêmea** 1x15, 2,54 mm | soquete do ESP32, fileira de cima (EN na ponta da antena) |
| 1 | Barra de pinos **fêmea** 1x14, 2,54 mm | soquete do ESP32, fileira de baixo: **sem contato na posição do D23** (ponta da antena); alinhe a barra ao pino D22 |
| 1 | Barra de pinos **fêmea** 1x4, 2,54 mm, vertical comum | soquete do transceptor (recebe a barra de 4 pinos de 90 graus do módulo, como vem de fábrica) |
| 4 | Soquete **fêmea** de 1 pino (ou 4 pinos cortados de uma barra fêmea) | soquete do LM2596 |
| 4 | Pino macho 2,54 mm | soldar nos 4 furos do LM2596 |
| 1 | Barra de pinos **macho** 1x2, 2,54 mm, vertical | J3 "CAN p/ módulo" (H = CAN_H, L = CAN_L) |
| 2 | Jumper dupont fêmea, até 10 cm, trançados entre si | liga J3 aos pinos CANH/CANL do módulo (ou ao borne dele) |
| 1 | Borne de parafuso 2 vias, passo 5,08 mm (Phoenix MKDS 1,5/2) | J1: 12 V e GND |
| 1 | Borne de parafuso 3 vias, passo 5,08 mm (MKDS 1,5/3) | J2: GND, CAN_H, CAN_L; encaixa no J1 formando 5 vias |
| 1 | F1: fusível rearmável PTC radial, 0,5 A de manutenção, **60 V ou mais** (RXEF050, MF-R050 ou similar), terminais a 5,1 mm | abre num curto na placa e volta sozinho depois de esfriar. Não use os de 16 V |
| 1 | D1: diodo Schottky 1N5819 (DO-41) | proteção contra inversão de polaridade |
| 1 | C1: capacitor eletrolítico 100 uF / 25 V (radial, D 6,3 mm, passo 2,5 mm) | |
| 3 | Parafuso M3 + porca (ou espaçador) | furos de fixação H1, H2, H3 |
| - | Fenolite de face simples, ~60 x 76 mm | |

Total de pinos de soquete a soldar: 29 (ESP32) + 4 (transceptor) + 4 (LM2596) + 2 (J3) = 39.

Sobre o módulo Waveshare: a barra de 4 pinos de 90 graus que ele já traz é justamente o que permite a montagem em
pé. Os pinos saem paralelos à placa do módulo e, encaixados apontando para baixo num soquete fêmea vertical, o módulo
fica **perpendicular à PCB**. Não é preciso dobrar nem trocar a barra. O módulo é montado com o **lado dos componentes
(e o borne de parafuso CAN) voltado para DENTRO da placa (norte)**; olhando de dentro da placa para a borda de baixo,
os pinos ficam, da esquerda para a direita: 3V3, GND, CAN RX, CAN TX (na PCB, de oeste para leste: CAN_TX, CAN_RX,
GND, 3V3, como mostra a serigrafia). O borne do módulo fica a cerca de 27-37 mm de altura, avançando ~10 mm para dentro
da placa, sem tocar nada.

## Medidas a conferir (paquímetro) e como regenerar

As medidas dos módulos são estimativas. Confira no topo de `tccelta/scripts/gerar_placa.py` (constantes marcadas
`CONFERIR COM PAQUIMETRO`) e ajuste se for diferente:

| Constante | Valor atual |
|---|---|
| `ESP32_PINOS_POR_FILEIRA` | 15 |
| `ESP32_ESPACAMENTO_FILEIRAS` | 22,86 mm (0,9", típico do DOIT V1; ainda não medido) |
| `ESP32_COMPRIMENTO` / `ESP32_LARGURA` | 51,5 / 28,3 mm |
| `ESP32_ANTENA_AVANCO` (além do pino 1) | 8,0 mm |
| `ESP32_ANTENA_INICIO_APOS_PINO1` | 1,5 mm (nenhum cobre além de pino 1 + 1,0 mm; `ESP32_KEEPOUT_INICIO`) |
| `LM2596_COMPRIMENTO` / `LM2596_LARGURA` | 43,0 / 21,0 mm |
| `LM2596_FURO_BORDA_CURTA` | 2,0 mm |
| `LM2596_FUROS_LARGURA` / `LM2596_FUROS_COMPRIMENTO` | 17,0 / 39,0 mm |
| `SN65_MODULO_LARGURA` / `SN65_MODULO_ALTURA` | 14,0 / 37,0 mm |
| `SN65_MODULO_ESPESSURA` | 2,0 mm |
| `SN65_BORNE_SALIENCIA` / `SN65_BORNE_LARGURA` | 10,0 / 10,0 mm |
| `SN65_ORDEM_PINOS` | CAN_TX, CAN_RX, GND, 3V3 (oeste -> leste na PCB) |

Duas suposições que só o módulo físico responde (use o teste em papel abaixo):

- **LM2596**: IN+ e OUT+ estão na mesma lateral longa (a de baixo, voltada ao ESP32) e IN-/OUT- na outra,
  com IN à esquerda e OUT à direita.
- **SN65HVD230 Waveshare**: olhando para o lado dos componentes (o borne CAN vem na sua direção) com a barra de
  pinos embaixo, a ordem da esquerda para a direita é 3V3, GND, CAN RX, CAN TX. Na PCB o lado dos componentes e
  o borne ficam voltados **para dentro** da placa (norte). Outra ordem exige rever o roteamento (o script avisa).

Para regenerar tudo depois de mudar uma constante (use o Python do KiCad):

```
C:/Users/davif/AppData/Local/Programs/KiCad/10.0/bin/python.exe montagem-kicad/tccelta/scripts/gerar_placa.py
```

O script reescreve símbolos, footprints, esquemático, PCB e `fabricacao/`, e imprime o resultado do ERC, do DRC
(com paridade esquemático x PCB) e a conferência de escala 1:1 dos PDFs.

## Teste em papel 1:1

1. Imprima `tccelta/fabricacao/teste_papel_1x1.pdf` em **100 % (tamanho real, sem "ajustar à página")**.
2. Recorte e espete os pinos dos módulos nos furos do papel: ESP32, LM2596 e soquete do transceptor devem
   encaixar sem folga. O desenho do transceptor mostra a linha fina do módulo em pé e a seta "COMPONENTES E BORNE PARA DENTRO".
3. Meça o papel impresso com o paquímetro: a placa deve ter 54,2 x 72,5 mm. Se não, a impressora está escalando.
4. Se algo não encaixar, ajuste as constantes, rode o script de novo e repita.

## Impressão e transferência (toner)

1. Imprima `tccelta/fabricacao/cobre_B_Cu_toner.pdf` em impressora **laser**, em papel glossy/de transferência,
   **100 % de escala, sem espelhar**. O arquivo já está na orientação certa: a transferência inverte a imagem
   e o texto de cobre fica legível na placa pronta, vista pelo lado do cobre.
2. Limpe o fenolite com palha de aço fina e álcool isopropílico. Alinhe o papel com o toner voltado para o cobre.
3. Transfira com ferro quente (algumas passadas firmes) ou laminadora. Deixe esfriar e solte o papel na água.
4. Retoque falhas com caneta de retroprojetor ou esmalte.

Pontos pretos no arquivo marcam os furos (marcas pequenas): servem de guia para o punção.

## Corrosão e furação

1. Corroa em percloreto de ferro morno (cerca de 40 C), agitando. Retire quando todo o cobre exposto sumir.
2. Lave, remova o toner com acetona e passe palha de aço fina.
3. Fure da face do cobre:
   - **1,0 mm**: soquetes do ESP32 (29), do transceptor (4), do LM2596 (4), J3 (2) e F1 (2): 41 furos.
   - **1,0 a 1,1 mm** para os pads do D1 (2) e **0,8 a 1,0 mm** para o C1 (2): alargue se as pernas não entrarem.
   - **1,3 mm**: bornes J1 e J2 (5 furos).
   - **3,2 mm**: furos de fixação M3 (3).
   Veja `tccelta/fabricacao/furacao.txt` para a lista completa (e `tccelta.drl`, formato Excellon).

A serigrafia (referências e rótulos dos pinos) está em `serigrafia_F_Silk.pdf` como guia de montagem, e em
`serigrafia_F_Silk_espelhada.pdf` para quem quiser transferi-la por toner no lado dos componentes.

## Montagem

1. Solde **somente**: as barras fêmea (ESP32 1x15 + 1x14, transceptor), os 4 soquetes do LM2596, os bornes J1 e J2, a barra
   macho J3, o F1, o D1 e o C1. Componentes por cima; solda e cobre embaixo.
   - F1: não tem polaridade. Fica no canto de baixo à esquerda, entre o borne e o D1. Solde com o disco **uns 3 a 4 mm
     acima da placa** (as dobras dos terminais já seguram nessa altura), para a cabeça do parafuso do furo H2 passar
     por baixo da borda dele.
   - D1: **faixa (catodo) voltada para cima** na placa (pad de cima), anodo para baixo, ligado ao F1.
   - C1: perna longa (+) no pad da esquerda, marcada no desenho.
   - Encaixe o J2 ao lado do J1 (borne de 5 vias): `+12V, GND, GND, CAN_H, CAN_L`. A entrada dos fios
     fica voltada para a borda de baixo da placa.
2. Solde os 4 pinos macho nos furos do LM2596.
3. Encaixe o LM2596 e **ajuste a saída para 5,0 V com o multímetro ANTES de encaixar o ESP32** (ligue 12 V no borne).
4. Desligue e encaixe o ESP32 (antena para a direita, para fora da placa; USB do lado esquerdo; o pino EN fica na
   ponta da antena e o VIN na ponta do USB).
5. Encaixe o transceptor **em pé** no soquete 1x4, com os pinos de 90 graus apontando para baixo, **com o lado dos
   componentes e o borne voltados para DENTRO da placa** (siga a seta no desenho). Se entrar ao contrário,
   3V3/GND/CAN ficam trocados.
6. Ligue dois fios dupont **trançados** (até 10 cm) de J3 (H = CAN_H, L = CAN_L) aos pinos CANH/CANL do módulo
   (ou ao borne de parafuso dele).
7. Cabo OBD-II: pino 16 = +12 V, pinos 4 e 5 = GND, pino 6 = CAN_H, pino 14 = CAN_L, nos 5 vias do borne.

O firmware usa TX = GPIO21 e RX = GPIO22 (pinos D21 e D22 do ESP32 DevKit V1). O VIN do ESP32 recebe os 5 V do buck.

## Avisos

- **Não ligue o USB do ESP32 ao mesmo tempo que os 12 V**, a menos que a placa do ESP32 tenha diodo no VBUS.
  Os 5 V do buck entram pelo pino VIN, que no DOIT V1 é o mesmo nó dos 5 V do USB.
- O resistor de 120 ohms do módulo SN65 serve para o simulador. **No carro real o barramento já tem terminação:
  remova o resistor.**
- **Só encaixe o transceptor na orientação certa** (lado dos componentes e borne para dentro): o soquete 1x4 não tem chave
  e invertê-lo liga o 3V3 no pino de CAN TX.
- O transceptor em pé chega a ~37 mm acima da placa; deixe a caixa/uso compatível com essa altura. O borne dele
  avança ~10 mm para dentro da placa (sobre a área livre entre o transceptor e o ESP32, no alto).
- A antena do ESP32 avança além da borda direita da placa (a borda fica 2,0 mm depois do centro do pino 1) e **não
  há cobre além de pino 1 + 1,0 mm**: nem trilha, nem plano de GND. Há uma área proibida de cobre sob a antena.
- O contato do D23 (ponta da antena, fileira de baixo) não existe na PCB: o D23 não é usado e, sem esse pad, o
  plano de GND passa entre o D22 e a borda e liga o GND do transceptor ao restante, sem bolsões. O pino D23 do
  ESP32 fica sem contato (use a barra 1x14 na fileira de baixo).
- Dois pads de GND (GND do transceptor e C1 -) usam conexão sólida ao plano em vez de alívio térmico: só há espaço
  para um raio no primeiro e o C1 é o capacitor de entrada. Os demais mantêm alívio térmico de 0,8 mm.
- C1 ficou logo à direita do D1 (parte de baixo-esquerda), ligado ao catodo por uma diagonal curta de +12 V_PROT.
  Colocá-lo junto ao IN+ do buck custaria cerca de 8 mm a mais de largura.
- **F1 (PTC)** é o primeiro componente depois do borne, então protege tudo o que vem depois (D1, C1, buck, ESP32,
  transceptor) contra curto. O consumo normal é de ~0,15 A em 12 V; o PTC só abre perto de 1 A e, num carro quente
  (~70 °C), ainda segura uns 0,35 A. Quando abre, esquenta (é assim que ele corta) e volta sozinho alguns minutos
  depois de desligar. Ele não protege o cabo entre o conector OBD-II e a placa: esse trecho continua só com o
  fusível do próprio carro.
- O furo de fixação H2 fica colado ao F1: a cabeça de um parafuso M3 comum (5,5 mm) avança ~0,5 mm sob a borda do
  disco do PTC visto de cima. Por isso o F1 vai soldado um pouco alto (ver Montagem).

## Validação (resultado atual)

ERC: 0 violações. DRC com paridade esquemático x PCB: 0 violações, 0 itens desconectados, 0 problemas de paridade.
Netlist conferida contra o modelo. Escala 1:1 dos PDFs conferida.
