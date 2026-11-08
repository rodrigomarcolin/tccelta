# Passo a passo: serigrafia na face de cima (opcional)

Como transferir para a face de cima da placa (a fenolite, sem cobre) os contornos dos módulos, as referências
(U1, U2, U3, F1, D1, C1, J3), os nomes dos pinos e as marcas de orientação (o "+" do C1, a seta do transceptor).
É o mesmo método de toner e ferro de passar usado no cobre, descrito em [PASSO_A_PASSO.md](PASSO_A_PASSO.md).

A serigrafia não tem função elétrica: é só um guia de montagem impresso na própria placa. Se preferir pular esta
etapa, imprima `tccelta/fabricacao/serigrafia_F_Silk.pdf` em papel comum e deixe ao lado na hora de montar.

## Qual arquivo usar e por quê

| Arquivo | Para quê |
|---|---|
| `serigrafia_F_Silk.pdf` | guia em papel; mostra a placa vista de cima, com os textos legíveis |
| `serigrafia_F_Silk_espelhada.pdf` | **este é o da transferência** |

A transferência funciona como um carimbo: a imagem sai invertida na placa. Por isso o arquivo da transferência
vem espelhado no papel: depois de passar para a placa, os textos ficam legíveis. Se usar o arquivo normal, tudo
sai ao contrário, como num espelho.

O desenho tem dois guias de alinhamento:

- a **linha do contorno da placa**;
- um **pontinho no centro de cada furo** (53 ao todo), na mesma posição dos furos que você já fez.

## Quando fazer

**Depois** de terminar o cobre: corrosão, limpeza com acetona, furação e corte do contorno (passos 1 a 10 do
[PASSO_A_PASSO.md](PASSO_A_PASSO.md)). E **antes** de proteger o cobre (passo 11).

- Antes da limpeza com acetona não dá: a acetona usada para tirar o toner do cobre apagaria a serigrafia junto.
- Depois da furação é melhor, porque os furos servem para alinhar o desenho.
- Antes do verniz de breu no cobre, porque o calor do ferro derreteria o verniz que fica apoiado embaixo.

---

## 1. Imprimir

1. Imprima `serigrafia_F_Silk_espelhada.pdf` na impressora **a laser**, no mesmo papel couché/glossy usado no
   cobre:
   - escala **100 %** (tamanho real, sem "ajustar à página");
   - **não espelhe de novo**: o arquivo já vem espelhado;
   - melhor qualidade, **com a economia de toner desligada**.
2. Confira com o paquímetro: o contorno deve ter 54,2 x 72,5 mm. Se não tiver, a impressora está mudando a escala.
3. Não encoste os dedos no desenho.
4. Recorte a folha deixando uns 2 cm de margem em volta do contorno.

## 2. Preparar a face de cima

O toner gruda pior na fenolite lisa do que no cobre. Uma superfície levemente áspera e limpa ajuda muito.

1. Lixe a face de cima (a fenolite, sem cobre) **de leve** com lixa d'água fina (400 a 600), só para tirar o
   brilho. Não precisa tirar material.
2. Limpe o pó, passe álcool isopropílico e deixe secar.
3. Daqui em diante, segure a placa só pelas bordas.

## 3. Alinhar o desenho

Esta é a parte que exige mais cuidado: o desenho tem que cair exatamente sobre os furos.

1. Coloque o papel sobre a face de cima com **o lado impresso encostado na fenolite** (o verso do papel para
   cima).
2. Segure a placa com o papel contra uma luz forte (janela ou lanterna do celular por trás). A luz passa pelos
   furos da placa e aparece através do papel.
3. Mova o papel até que:
   - a linha do contorno coincida com as bordas da placa;
   - os pontinhos do desenho fiquem no centro dos furos iluminados. Confira em pontos distantes entre si, por
     exemplo um furo do LM2596 em cima, um soquete do ESP32 no meio e o furo do F1 embaixo.
4. Sem deixar o papel sair do lugar, dobre as margens por cima das bordas da placa, como quem embrulha um
   presente, e prenda as abas com fita adesiva **no verso** (a face do cobre). Assim a fita fica longe do ferro.
5. Confira de novo contra a luz. Se algo saiu do lugar, solte a fita e repita.

Dica: para travar o alinhamento, fure o papel com uma agulha em 2 ou 3 pontinhos distantes entre si e espete
alfinetes ou pinos macho soltos de uma barra, atravessando o papel até os furos da placa. Prenda a fita e **tire
os pinos antes de passar o ferro**.

## 4. Transferir com o ferro de passar

1. Apoie a placa numa superfície firme e que aguente calor (tábua de madeira com um pano fino por cima). Agora o
   **cobre fica para baixo**, encostado no pano, e o papel para cima.
2. Ferro na temperatura de **algodão**, **sem vapor** e sem água no reservatório.
3. Passe o ferro por **3 a 5 minutos**:
   - no primeiro minuto, só pressione o ferro parado sobre a placa, com força, para o toner grudar sem
     escorregar;
   - depois, esfregue com firmeza, em movimentos curtos, cobrindo toda a área;
   - capriche nos textos pequenos (nomes dos pinos), que são os que mais falham.
4. Não passe muito do tempo: a fenolite aquecida demais pode empenar ou formar bolhas.
5. **Deixe esfriar** até a temperatura ambiente, sem mexer no papel. Se puxar o papel quente, o toner sai junto.

## 5. Tirar o papel na água

Igual ao cobre: o papel fica grudado e não sai a seco sem arrancar o toner junto. A placa vai **inteira para a
água, com o papel ainda grudado**.

1. Encha uma vasilha com água (morna ou da torneira) suficiente para cobrir a placa.
2. Coloque a placa com o papel na água e **espere de 10 a 20 minutos**, até o papel ficar mole e translúcido.
3. Ainda dentro da água, **esfregue de leve com o dedão** para soltar o papel aos poucos. Na fenolite o toner gruda
   menos do que no cobre, então vá com ainda mais delicadeza nos textos pequenos. Não use unha nem esponja.
4. Se o papel resistir, devolva a placa à água por mais alguns minutos e tente de novo.
5. Uma película esbranquiçada de papel por cima dos textos não atrapalha. Se quiser tirar, esfregue de leve com o
   dedo molhado.
6. Os furos podem ficar com papel dentro: desentupa com um palito ou com a própria broca girando com a mão.
7. Seque com papel-toalha, sem esfregar o desenho, e deixe a placa **secar bem por algumas horas**. A fenolite
   absorve um pouco de água, e a umidade atrapalha na hora de soldar.

## 6. Conferir e retocar

1. Compare com `serigrafia_F_Silk.pdf` (o normal). Os textos devem estar legíveis e os contornos em volta dos
   furos certos.
2. Retoque o que falhou com caneta permanente de ponta fina. As marcas mais importantes são as de orientação:
   - o **"+" do C1**;
   - a **faixa do D1** (catodo);
   - a **seta "COMPONENTES E BORNE PARA DENTRO"** do transceptor;
   - o **pino 1** e os nomes 3V3/GND/CTX/CRX do soquete do transceptor.
3. Se a transferência ficou ruim no geral, tire o toner com acetona **só na face de cima** (sem encostar no
   cobre se ele já estiver protegido) e repita os passos 2 a 5.

## 7. Proteger (recomendado)

O toner na fenolite sai com o manuseio e com o álcool usado para limpar o fluxo da solda.

1. Passe uma camada fina de **verniz spray transparente** na face de cima, a uns 20 cm de distância.
2. Cubra os furos com fita no verso ou passe um palito depois, para o verniz não entupir os furos.
3. Deixe secar conforme a embalagem.
4. Na hora de limpar o fluxo da solda, passe o álcool **só na face do cobre**.

Depois disso, volte ao [PASSO_A_PASSO.md](PASSO_A_PASSO.md) a partir do passo 11 (proteger o cobre).
