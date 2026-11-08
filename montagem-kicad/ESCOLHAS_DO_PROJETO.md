# Escolhas do projeto: função e posição de cada elemento

Por que cada peça da placa existe e por que está onde está. O procedimento de fabricação está em
[PASSO_A_PASSO.md](PASSO_A_PASSO.md), a lista de materiais em [MATERIAIS.md](MATERIAIS.md) e os detalhes do
projeto KiCad no [README](README.md).

## O caminho da energia

Toda a energia vem do pino 16 do OBD (12 V da bateria) e passa por uma corrente de proteções antes de chegar ao
ESP32. Cada peça protege a seguinte, e essa sequência explica tanto a função de cada uma quanto a posição dela na
placa.

```
OBD pino 16 (+12 V) → borne J1 → F1 fusível → D1 diodo → C1 capacitor → U1 LM2596 (12→5 V) → VIN do ESP32
                                                                                    ↓ regulador interno do ESP32
                                                                                  3V3 → transceptor U3
```

## Fusível F1: contra curto-circuito

**Função:** limitar a corrente se algo der errado na placa, por exemplo:

- uma ponte de solda;
- o C1 ou o LM2596 queimarem em curto;
- dois fios do borne se encostarem.

Sem ele, só restaria o fusível do carro, de 10 a 20 A. Essa corrente queima as trilhas de fenolite muito antes de
abrir aquele fusível. O pino 16 fica ligado direto na bateria, mesmo com o carro desligado, então um curto parcial
poderia esquentar a placa ou descarregar a bateria sem ninguém ver.

**Por que PTC (rearmável):** num curto, o PTC esquenta e a resistência dele sobe muito, cortando a corrente.
Quando o curto é desfeito e ele esfria, volta a conduzir sozinho. Não precisa de porta-fusível nem de fusível de
reposição, e ocupa o espaço de um capacitor de disco.

**Por que esses valores:**

- **0,5 A:** o dongle consome uns 60 a 150 mA em 12 V, então ele nunca desarma no uso normal e desarma num curto.
- **60 V:** a tensão do carro passa bastante dos 14 V em picos, por exemplo na partida ou quando uma carga grande
  desliga.

**Por que é o primeiro da fila:** tudo o que vem depois dele fica protegido, inclusive o D1. Ele está colado no
borne para que o trecho de cobre sem proteção, entre o borne e o F1, seja o menor possível.

**Limite:** o F1 protege a placa, mas não o cabo entre o conector OBD e a placa. Esse trecho fica só com o fusível
do carro. A alternativa seria um porta-fusível de linha emendado no cabo, perto do conector, mas os fios do cabo
são finos e ficam dentro de um envoltório de borracha, o que torna a emenda trabalhosa.

## Diodo D1: contra polaridade invertida

**Função:** deixar a corrente passar só no sentido certo. Se o +12 V e o GND forem trocados, o diodo bloqueia e
nada recebe tensão invertida. Sem ele, uma inversão queimaria o LM2596 e o C1 na hora.

O conector OBD tem encaixe único e não inverte. O risco real está na bancada: ligar a fonte do simulador no borne
ao contrário é um erro fácil.

**Por que Schottky (1N5819) e não um diodo comum (1N4007):**

- a queda de tensão do Schottky é de cerca de 0,3 a 0,4 V, contra 0,7 a 1 V do comum;
- isso significa menos calor e mais tensão sobrando para o LM2596, que precisa de alguns volts acima dos 5 V de
  saída para funcionar.

**Por que depois do fusível e antes do capacitor:** o C1 é eletrolítico, e com tensão invertida ele estufa ou
estoura. Com o diodo na frente, ele nunca recebe tensão invertida.

## Capacitor C1: reservatório de energia na entrada

**Função:** o LM2596 é uma fonte chaveada e puxa corrente em pulsos, dezenas de milhares de vezes por segundo. O
ESP32 também tem picos de consumo quando transmite por Bluetooth ou Wi-Fi. O cabo OBD é fino e comprido e não
consegue entregar esses picos instantaneamente, então a tensão oscilaria na entrada. O C1 fica ao lado e entrega a
energia desses picos. Ele também amortece pequenos ruídos e oscilações da rede elétrica do carro.

**Por que 100 µF / 25 V:**

- **100 µF** é uma reserva confortável para esse consumo;
- **25 V** dá folga sobre os cerca de 14,4 V do carro ligado (alternador carregando a bateria).

**Por que fica junto do D1 e não junto do LM2596:** o ideal seria colado na entrada IN+ do LM2596. Mas o módulo
LM2596 já traz o próprio capacitor de entrada, e levar o C1 até lá aumentaria a placa uns 8 mm. Junto do D1 ele
cumpre bem o papel de reservatório da entrada. É uma concessão consciente.

## LM2596 (U1): de 12 V para 5 V

**Função:** abaixar a tensão de 12 a 14 V para 5 V.

**Por que um regulador chaveado, e não um linear como o 7805:**

- o linear transforma a diferença de tensão em calor: (12 − 5) V × corrente;
- o chaveado tem eficiência de cerca de 80 a 90 % e quase não esquenta.

**Por que 5 V no VIN, e não 3,3 V direto no ESP32:**

- a placa DOIT tem o próprio regulador de 3,3 V, feito para receber 5 V no VIN;
- se o LM2596 alimentasse o pino 3V3 direto, um erro no ajuste do trimpot queimaria o ESP32 na hora;
- pelo VIN, existe uma segunda regulação no meio.

**Por que no alto da placa:** a energia percorre um caminho só, sem cruzar os sinais:

- os 12 V sobem pela lateral esquerda até a entrada do LM2596;
- a saída de 5 V vai até o VIN do ESP32.

O CAN fica embaixo, do lado direito. Assim a parte de potência, que é ruidosa, fica separada dos sinais.

## Transceptor SN65HVD230 (U3)

**Função:** o ESP32 tem o controlador CAN (TWAI), mas trabalha com níveis lógicos de 0 e 3,3 V. O barramento CAN
do carro usa sinal diferencial em dois fios, CAN_H e CAN_L. O transceptor faz a tradução entre os dois.

**Por que o SN65HVD230:** ele funciona em 3,3 V, a mesma tensão do ESP32. Não precisa de conversor de nível, como
precisaria um transceptor de 5 V (MCP2551 ou TJA1050). Ele se alimenta do 3V3 do próprio ESP32.

**Por que em pé, na borda de baixo:**

- em pé, ocupa só a faixa do soquete 1x4;
- na borda de baixo, fica perto do borne, então o caminho do CAN é curto;
- CTX e CRX sobem direto até o D21 e o D22, sem cruzar outras trilhas.

**Resistor de 120 Ω do módulo:** é a terminação do barramento. Serve no simulador, mas no carro o barramento já
tem terminação nas duas pontas, então ele deve ser removido.

## J3 e os jumpers trançados

O módulo Waveshare não leva o CAN_H/CAN_L pela barra de 4 pinos, só pelo borne de parafuso dele. Por isso a placa
leva o CAN do borne principal até o J3, ao lado do módulo, e dois fios curtos completam a ligação.

Os fios são trançados porque o CAN é diferencial. Quando os dois fios andam juntos, um ruído externo afeta os dois
por igual e se cancela.

## Borne J1+J2 (5 vias)

- Um borne de parafuso aceita os fios finos do cabo OBD sem solda e permite trocar o cabo.
- São dois GND porque o OBD tem dois terras: pino 4 (chassi) e pino 5 (sinal). Os dois chegam juntos ao plano de
  GND da placa.

## Decisões gerais de posicionamento

- **Plano de GND cobrindo a placa:** dá um caminho de retorno de baixa resistência para todas as correntes e
  ajuda a blindar contra ruído. De quebra, sobra menos cobre para o percloreto dissolver.
- **Antena do ESP32 para fora da placa, sem cobre embaixo:** cobre perto da antena atrapalha o Bluetooth e o
  Wi-Fi. Por isso ela fica além da borda, e há uma área proibida de cobre a partir de 1 mm do pino 1.
- **Proteções agrupadas no canto do borne:** F1, D1 e C1 ficam todos no canto de baixo à esquerda. O 12 V "sujo",
  ainda sem proteção, fica restrito a poucos milímetros, e o resto da placa só recebe energia já filtrada e
  protegida.
