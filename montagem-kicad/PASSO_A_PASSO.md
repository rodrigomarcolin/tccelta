# Passo a passo: da impressão à placa montada

Fabricação caseira da placa do dongle (fenolite de face simples, transferência de toner, corrosão com percloreto
de ferro) e montagem dos módulos. A lista de ferramentas e materiais está em [MATERIAIS.md](MATERIAIS.md). Os
arquivos citados ficam em `tccelta/fabricacao/`.

## Antes de começar: como o método funciona

**Toner** é o pó preto que a impressora **a laser** usa no lugar de tinta. Na impressão ele derrete e gruda no
papel. Ele é praticamente um plástico, e é isso que faz o método funcionar:

1. Com o calor do ferro de passar, o toner derrete de novo e passa do papel para o cobre da placa.
2. O percloreto de ferro não ataca o cobre que está coberto de toner. Esse cobre vira as trilhas.
3. Onde não tem toner, o percloreto dissolve o cobre.

Por isso a impressora **tem que ser a laser** (ou uma copiadora). A tinta de jato de tinta é líquida, não derrete
de novo e não transfere.

---

## Parte 1: fabricar a placa

### 1. Teste em papel (não pule)

1. Imprima `teste_papel_1x1.pdf` em **100 %** (tamanho real, sem "ajustar à página").
2. Meça com o paquímetro: o contorno da placa deve ter 54,2 x 72,5 mm. Se não tiver, a impressora está mudando
   a escala. Corrija isso antes de seguir.
3. Recorte o papel, apoie sobre um isopor ou papelão e espete os pinos do ESP32, do LM2596 e do transceptor nos
   furos desenhados. Tudo deve encaixar sem forçar.
4. Se algo não encaixar, ajuste as constantes `CONFERIR` em `tccelta/scripts/gerar_placa.py`, rode o script de
   novo (veja o [README](README.md)) e repita o teste.

### 2. Imprimir as trilhas

1. Imprima `cobre_B_Cu_toner.pdf` na impressora a laser, em papel couché/glossy (ou papel transfer próprio para
   PCB):
   - escala **100 %**;
   - **sem espelhar**: o arquivo já está na orientação certa;
   - melhor qualidade de impressão, **com a economia de toner desligada**.
2. Não encoste os dedos no desenho impresso: a gordura atrapalha a transferência.
3. Recorte a folha deixando uns 2 cm de margem em volta do desenho.

### 3. Preparar a placa de fenolite

1. Corte a fenolite com uns 3 mm de sobra em cada lado do desenho (cerca de 60 x 76 mm). O contorno final é
   cortado depois da corrosão.
2. Lixe o cobre com palha de aço fina, em movimentos circulares, até ficar brilhante e uniforme, sem manchas
   escuras.
3. Lave com água e detergente, seque com papel-toalha e passe álcool isopropílico.
4. Daqui em diante, **segure a placa só pelas bordas**.

### 4. Transferir o toner com o ferro de passar

1. Coloque o papel sobre a placa com **o lado impresso (o desenho preto) encostado no cobre**, e o verso do papel
   para cima. Funciona como um carimbo: a face com o desenho encosta na placa.
2. Centralize o desenho na placa. Dobre as margens do papel por cima das bordas da placa, como quem embrulha um
   presente, e prenda essas abas com fita adesiva **no verso da placa** (a face sem cobre). Assim o papel não
   escorrega e a fita fica longe do ferro, sem derreter sobre o desenho.
3. Apoie a placa numa superfície firme e que aguente calor (uma tábua de madeira com um pano fino por cima).
4. Ferro na temperatura de **algodão**, **sem vapor** e sem água no reservatório.
5. Passe o ferro por **3 a 5 minutos**:
   - no primeiro minuto, só pressione o ferro parado sobre a placa, com força, para o toner grudar sem
     escorregar;
   - depois, esfregue com firmeza, em movimentos curtos, cobrindo toda a área;
   - capriche nas bordas e nos cantos, que são onde a transferência mais falha. A ponta do ferro ajuda.
6. **Deixe esfriar** até a temperatura ambiente, sem mexer no papel. Se você puxar o papel quente, o toner sai
   junto.

### 5. Tirar o papel na água

Depois do ferro, o papel fica grudado no cobre e não dá para puxar a seco sem arrancar o toner junto. Por isso a
placa vai **inteira para a água, com o papel ainda grudado**.

1. Encha uma vasilha com água (morna ou da torneira) suficiente para cobrir a placa.
2. Coloque a placa com o papel na água e **espere de 10 a 20 minutos**, até o papel ficar mole e encharcado (ele
   fica translúcido e dá para ver o desenho através dele).
3. Ainda dentro da água, **esfregue de leve com o dedão** para soltar o papel aos poucos, como quem tira um
   adesivo molhado. A fibra do papel sai e o toner fica no cobre. Não use unha, faca ou esponja abrasiva.
4. Se o papel resistir, devolva a placa à água por mais alguns minutos e tente de novo.
5. Pode sobrar uma película fina e esbranquiçada de papel **por cima das trilhas**. Tudo bem, ela não atrapalha.
6. O que **não pode** sobrar é papel **entre as trilhas** (onde o cobre deve sumir): ali ele bloqueia o percloreto.
   Olhe com atenção nos vãos mais estreitos, onde a fibra do papel costuma ficar presa:
   - entre **pads vizinhos**: pad é o círculo ou oval de cobre onde vai a perna de um componente; nos soquetes
     do ESP32 eles ficam a 2,54 mm um do outro, e o vão entre dois deles tem menos de 1 mm;
   - entre as trilhas e o plano de GND (a área grande de cobre), separados por 0,6 a 0,8 mm.

   Esfregue esses pontos de leve até ver o cobre limpo no vão.
7. Seque a placa com papel-toalha, sem esfregar o desenho.

### 6. Conferir e retocar

1. Compare a placa com o PDF. Procure trilhas interrompidas, pads incompletos ou falhas no plano de GND.
2. Retoque as falhas com caneta permanente de retroprojetor (ou esmalte), cobrindo só o que deve continuar sendo
   cobre.
3. Se a transferência ficou ruim no geral, tire o toner com acetona, lixe de novo e repita os passos 3 a 5. Sai
   barato.

### 7. Corroer no percloreto

Use luvas, óculos e roupa velha, em local ventilado. O percloreto mancha roupa, pia e piso para sempre.

1. **Preparar a solução:** se o percloreto for em pó, dissolva-o em água conforme a embalagem, **sempre
   jogando o pó na água, aos poucos** (e não a água sobre o pó), porque esquenta.
2. **Aquecer:** a solução corrói bem mais rápido morna, a 40–50 °C. Coloque o pote ou a garrafa da solução em
   banho-maria, numa bacia de água quente, por alguns minutos antes de usar.
3. **Encher a bandeja:** use uma bandeja rasa de **plástico ou vidro (nunca de metal)**, com líquido suficiente
   para cobrir a placa por alguns milímetros.
4. **Colocar a placa:** com uma pinça de plástico, coloque a placa no fundo da bandeja, **com o cobre para cima**,
   para você enxergar o andamento.
5. **Balançar a bandeja:** a placa fica dentro do percloreto, e você **segura a bandeja pelas bordas e a inclina
   devagar para um lado e para o outro**, como uma gangorra, fazendo uma onda suave passar sobre a placa.
   - Por que balançar: o percloreto que já reagiu fica "cansado" grudado no cobre, e o movimento traz líquido novo
     para a superfície. Assim a corrosão fica mais rápida e por igual.
   - Não precisa ser frenético nem sem parar: um vai-e-vem lento e contínuo, ou uns 10 segundos de balanço a
     cada minuto, já resolve.
   - Cuidado para não derramar: incline pouco, só o bastante para o líquido correr sobre a placa.
   - Se a solução esfriar no meio, coloque a bandeja sobre a bacia de água quente e continue balançando.
6. **Acompanhar:** com solução nova e morna, leva uns **10 a 20 minutos**. Primeiro somem as áreas grandes de
   cobre; por último, os espaços estreitos entre os pads.
7. **Saber quando parar:** a cada poucos minutos, levante a placa com a pinça e olhe contra a luz. Quando não
   houver mais nenhum fio de cobre entre as trilhas e a luz passar por todos os espaços, a corrosão terminou.
8. **Não passe do ponto:** se deixar muito tempo além disso, o percloreto começa a comer as trilhas por baixo do
   toner e elas ficam finas.
9. **Lavar:** tire a placa com a pinça e lave bem em água corrente.
10. **Guardar a solução:** devolva o percloreto ao pote, fechado, para reutilizar. **Não jogue na pia.** Quando ele
    ficar lento demais, leve a um ponto de coleta de resíduos químicos (escolas técnicas e lojas de eletrônica
    costumam receber).

### 8. Limpar

1. Tire o toner com algodão e acetona (ou thinner). O cobre das trilhas aparece por baixo.
2. Dê uma passada leve de palha de aço fina para deixar o cobre brilhante e pronto para soldar.

### 9. Furar

1. Marque o centro de cada furo com um punção ou prego e uma batida leve. O desenho já indica onde:
   - cada pad (o círculo ou oval de cobre onde vai uma perna) tem no centro um **pontinho sem cobre**, de
     0,35 mm. Ele vem de um ponto branco no PDF, onde não há toner, e o percloreto abre esse furinho no cobre;
   - apoie a ponta do punção nesse pontinho e bata de leve: a marca guia a broca e evita que ela escorregue;
   - os 3 furos de fixação M3 não têm pad. O centro deles é o centro do círculo sem cobre que o plano de GND
     deixa em volta de cada um.
2. Fure **pelo lado do cobre**, com a broca a 90° (use suporte vertical na microretífica ou furadeira de bancada):

   | Broca | Onde | Quantidade |
   |---|---|---|
   | 1,0 mm | soquetes do ESP32, do transceptor, do LM2596, J3 e F1 | 41 |
   | 1,0 a 1,1 mm | D1 | 2 |
   | 0,8 a 1,0 mm | C1 (alargue se as pernas não entrarem) | 2 |
   | 1,3 mm | bornes J1 e J2 | 5 |
   | 3,2 mm | furos de fixação M3 | 3 |

   A lista completa está em `furacao.txt`.
3. Tire as rebarbas passando uma lixa fina de leve nos dois lados.

### 10. Cortar o contorno

1. A linha do contorno sai impressa e corroída junto com as trilhas. Corte a sobra por ela e lixe a borda.
2. **Do lado da antena do ESP32**, a borda fica a apenas 2 mm do pino 1. Não corte para dentro dessa linha.

### 11. Proteger o cobre (recomendado)

Sem proteção o cobre oxida em semanas e fica difícil de soldar. Escolha uma opção:

- estanhar as trilhas passando uma camada fina de solda com o ferro; ou
- pincelar um verniz de breu dissolvido em álcool, que também funciona como fluxo na hora de soldar.

---

## Parte 2: soldar e montar

### Soldar direto ou usar soquete?

Os furos de 1,0 mm no passo de 2,54 mm aceitam tanto a barra fêmea quanto o pino do próprio módulo. Então você pode
decidir na hora da montagem, módulo por módulo:

| Módulo | Pode soldar direto? | Recomendação |
|---|---|---|
| **ESP32** | Pode, mas **precisa abrir um furo de 1,0 mm (sem pad) na posição do D23**, ou cortar esse pino, senão ele bate na placa | **Soquete.** Tirar 30 pinos soldados da fenolite quase sempre arranca pads, e o ESP32 é o módulo que você mais vai querer tirar ou trocar |
| **LM2596** | Sim: pinos macho soldados no módulo e direto na placa | Pode soldar. Ajuste os 5 V **antes** de soldar, com o ESP32 fora da placa |
| **Transceptor** | Sim: a barra de 90 graus solda direto nos 4 furos | Soquete é melhor. A orientação é crítica, e o resistor de 120 ohms precisa sair para usar no carro real |
| F1, D1, C1, bornes, J3 | Sempre soldados | - |

### Cuidados com a fenolite

O pad de fenolite descola com calor demais. Fique **2 a 3 segundos** por solda, sem fazer força com a ponta do
ferro. Se a solda não pegou, espere esfriar e tente de novo.

### 12. Soldar, do componente mais baixo para o mais alto

Componentes por cima da placa; solda e cobre embaixo.

1. **D1 (1N5819):** dobre as pernas, encaixe com a **faixa (catodo) no pad de cima** e solde.
2. **J3 (barra macho 1x2).**
3. **Barras fêmea do ESP32:** 1x15 na fileira de cima e 1x14 na de baixo, alinhada ao D22 (a posição do D23 fica
   sem barra). Para elas ficarem retas e com o espaçamento certo:
   - encaixe o próprio ESP32 nas barras antes de soldar;
   - solde só um pino de cada ponta e confira se as barras estão retas e encostadas na placa;
   - só então solde o resto.

   O pino D23 do ESP32 fica no ar: a barra fêmea levanta o módulo uns 8 mm, e o pino não chega na placa.
4. **Soquete 1x4 do transceptor:** mesmo truque, com o módulo encaixado enquanto solda.
5. **Soquetes do LM2596 e pinos do módulo:**
   - corte 4 soquetes avulsos de uma barra fêmea (o corte inutiliza uma posição de cada lado);
   - solde os 4 soquetes na placa;
   - espete os 4 pinos macho nesses soquetes e apoie o LM2596 por cima, usando a placa como gabarito;
   - solde os pinos no LM2596.
6. **C1 (100 uF):** perna longa (+) no pad marcado com +.
7. **F1 (fusível PTC):** não tem lado certo. Fica no canto de baixo à esquerda, entre o borne e o D1. Solde com o
   disco **uns 3 a 4 mm acima da placa**: as dobras dos terminais já seguram nessa altura. Assim a cabeça do
   parafuso do furo de fixação vizinho passa por baixo da borda dele.
8. **Bornes J1 e J2:** encaixe um no outro (forma um borne de 5 vias), com a entrada dos fios virada para a borda
   da placa, e solde.

### 13. Conferir antes de ligar

1. Corte as pernas que sobraram, rente à solda.
2. Limpe o fluxo com álcool isopropílico e uma escova velha.
3. Com uma lupa, procure pontes de solda, principalmente entre os pads e o plano de GND (estão a só 0,6 mm).
4. Com o multímetro em continuidade, confira que **nenhum** destes está em curto com o GND: +12 V, +5 V e 3V3.

### 14. Primeira energização, por etapas

1. **Sem nenhum módulo encaixado**, ligue 12 V no borne (+12V e GND). Depois do F1 e do D1, meça cerca de 11,6 V.
   Se der 0 V e o F1 estiver quente, ele abriu por causa de um curto: desligue, procure o curto e espere o F1
   esfriar antes de tentar de novo.
2. **Encaixe só o LM2596** e ajuste o trimpot até a saída marcar **5,0 V** no multímetro. Muitos módulos vêm de
   fábrica com a saída quase igual à entrada, e isso queimaria o ESP32.
3. **Desligue.** Encaixe o ESP32: antena para fora da placa, USB do lado esquerdo.
4. Encaixe o transceptor **em pé**, com os pinos de 90 graus para baixo e **o lado dos componentes e o borne dele
   virados para dentro da placa** (siga a seta do desenho). Ao contrário, ele recebe 3V3 no pino de CAN TX.
5. Ligue dois jumpers dupont **trançados** entre si de J3 (H = CAN_H, L = CAN_L) aos pinos CANH/CANL do módulo.
6. Ligue o cabo OBD-II no borne: pino 16 = +12 V, pinos 4 e 5 = GND, pino 6 = CAN_H, pino 14 = CAN_L.
7. Ligue e teste com o simulador.

### Avisos

- **Não ligue o USB do ESP32 ao mesmo tempo que os 12 V**, a menos que a placa do ESP32 tenha diodo no VBUS.
- No carro real: **remova o resistor de 120 ohms do transceptor** (o barramento já tem terminação). O fusível já
  está na placa (F1); não é preciso emendar porta-fusível no cabo.
- Mais detalhes sobre a placa, as medidas a conferir e o script estão no [README](README.md).
