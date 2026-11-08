# Materiais e ferramentas

O que é preciso para fabricar e montar a placa do dongle, além dos componentes que já estão nos diagramas de
montagem (ESP32 DOIT DevKit V1, LM2596, SN65HVD230 Waveshare, conector OBD-II J1962 e jumpers). O procedimento
está em [PASSO_A_PASSO.md](PASSO_A_PASSO.md).

## Componentes da placa

| Qtd | Item | Para quê |
|---|---|---|
| 1 | Placa de fenolite face simples, pelo menos 60 x 76 mm (compre umas duas, caso precise refazer) | a placa |
| 2 | Barra de pinos **fêmea** 1x40, passo 2,54 mm | soquetes do ESP32 (1x15 e 1x14), do transceptor (1x4) e 4 soquetes avulsos do LM2596. Cortar barra fêmea desperdiça posições, por isso duas |
| 1 | Barra de pinos **macho** 1x40, passo 2,54 mm | J3 (1x2), JP1 (1x2) e os 4 pinos do LM2596 |
| 1 | Borne de parafuso KRE/KF301, 2 vias, passo 5,08 mm | J1: +12 V e GND |
| 1 | Borne de parafuso KRE/KF301, 3 vias, passo 5,08 mm | J2: GND, CAN_H e CAN_L |
| 1 | Fusível rearmável PTC radial 0,5 A, **60 V ou mais** (RXEF050, MF-R050 ou "PTC 72V 0,5A"), terminais a 5,1 mm | F1: abre num curto na placa e volta sozinho. Não use os de 16 V |
| 1 | Diodo Schottky 1N5819 | D1: proteção contra inversão de polaridade |
| 1 | Capacitor eletrolítico 100 uF / 25 V (radial, 6,3 mm de diâmetro) | C1: filtro da entrada |
| 1 | Resistor de filme metálico 120 ohms, 1/4 W, 1 % | R1: terminação CAN, montado em pé |
| 1 | Jumper de encaixe (shunt) 2,54 mm | fecha o JP1: colocado na bancada, retirado no carro |

## Fixação e ligação

| Qtd | Item | Para quê |
|---|---|---|
| 3 | Parafuso M3 x 10-12 mm, com porca e espaçador (ou pés de borracha) | furos de fixação |
| - | Fio 22-24 AWG e termo-retrátil | emenda do cabo OBD-II até o borne, se precisar |
| 1 | Caixa (opcional), altura interna de pelo menos ~40 mm | o transceptor em pé chega a ~37 mm; precisa de abertura para o cabo e para o USB |

## Fabricação da placa

### Impressão

- Impressora **a laser** (jato de tinta não serve; uma copiadora ou gráfica resolve).
- Papel couché/glossy de 90 a 150 g, página de revista ou papel transfer próprio para PCB.

### Corte e preparo

- Microretífica com disco de corte, ou estilete e régua de metal (riscar fundo e quebrar).
- Lixa d'água fina, para acabar as bordas.
- Palha de aço fina ou esponja abrasiva.
- Detergente, álcool isopropílico e papel-toalha.
- Fita adesiva.

### Transferência

- Ferro de passar roupa (sem usar o vapor).
- Tábua de madeira e um pano fino, como base.
- Vasilha com água, para soltar o papel.
- Caneta permanente para retroprojetor/CD de ponta fina, ou esmalte, para retoques.

### Corrosão

- Percloreto de ferro (em pó ou solução pronta).
- Bandeja rasa de **plástico ou vidro** (nunca de metal).
- Pote com tampa, para guardar a solução usada.
- Bacia com água quente, para o banho-maria.
- Pinça de plástico.
- Luvas nitrílicas, óculos de proteção e avental ou roupa velha.
- Acetona ou thinner e algodão, para tirar o toner.

### Furação

- Microretífica com suporte vertical, ou furadeira de bancada para PCB.
- Brocas de **0,8, 1,0, 1,3 e 3,2 mm**.
- Punção ou prego, para marcar o centro dos furos.

### Proteção do cobre (recomendado)

- Breu e álcool (verniz), ou só estanho, para estanhar as trilhas.

### Serigrafia na face de cima (opcional)

Além do material da transferência do cobre (impressora a laser, papel couché, ferro de passar):

- Lixa d'água fina (400 a 600), para tirar o brilho da fenolite.
- Agulha e alfinetes (ou pinos macho soltos), para alinhar o papel pelos furos.
- Lanterna ou janela clara, para ver os furos através do papel.
- Caneta permanente de ponta fina, para retoques.
- Verniz spray transparente, para proteger o desenho.

O procedimento está em [PASSO_A_PASSO_SERIGRAFIA.md](PASSO_A_PASSO_SERIGRAFIA.md).

## Soldagem e montagem

- Ferro de solda de 30 a 60 W, de preferência estação com temperatura ajustável (~330 a 350 °C), com ponta fina.
- Estanho 60/40 ou 63/37, de 0,5 a 0,8 mm, com fluxo.
- Pasta ou caneta de fluxo.
- Esponja ou lã de latão, para limpar a ponta.
- Sugador de solda e malha dessoldadora.
- Alicate de corte rente e alicate de bico.
- Suporte "terceira mão" e lupa.
- Álcool isopropílico e escova velha, para limpar o fluxo.
- Chave de fenda pequena, para os bornes e o trimpot do LM2596.

## Medição e teste

- Paquímetro, para conferir as medidas dos módulos e o teste em papel.
- Multímetro, para continuidade, curtos e o ajuste dos 5 V.
- Fonte de 12 V (ou o próprio simulador), para a primeira energização.
