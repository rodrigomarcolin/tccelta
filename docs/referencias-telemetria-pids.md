# Referências de telemetria e PIDs coletados

Este documento lista as fontes consultadas sobre telemática veicular, OBD-II e consumo, e liga cada PID coletado pelo dongle ao que a literatura sustenta. Foi escrito como base para o texto do TCC. Chaves BibTeX em `latex/referencias-telemetria.bib`.

Convenções:

- Cada referência é citada como `[n] (ano)`. O ano principal é o do volume ou da edição; a data de publicação online, quando difere, vem entre parênteses.
- "Fonte" é o que a referência afirma. "Inferência do projeto" é o que o projeto conclui por conta própria e não vem das fontes.
- Nenhum metadado foi inventado. O que não foi possível confirmar está dito no texto.

## Lista de referências

| N. | Ano | Referência | O que a fonte trata | Chave BibTeX |
|---|---|---|---|---|
| [1] | 2024 | Boylan, Meyer, Chen. *A systematic review of the use of in-vehicle telematics in monitoring driving behaviours*. Accident Analysis & Prevention 199. DOI 10.1016/j.aap.2024.107519 | Revisão de 45 estudos; aceleração, frenagem e velocidade são as variáveis mais comuns. | `boylan2024telematics` |
| [2] | 2024 | Guillén, Pérez-Marín, Nielsen. *Pricing weekly motor insurance drivers' with behavioral and contextual telematics data*. Heliyon 10(16), e36501. DOI 10.1016/j.heliyon.2024.e36501 | 19.214 motoristas, 55 semanas, 181,4 milhões de km; excesso de velocidade, tipo de via e horário ajudam na precificação. | `guillen2024pricing` |
| [3] | 2025 (online em 2024 segundo a OpenAlex) | Strada, Costantini, Formentin, Savaresi. *Machine learning based car accident risk prediction for usage-based insurance*. Intelligent Data Analysis 29(1), 156-170. DOI 10.3233/ida-230971 | Quase 100 mil veículos; registra que o UBI costuma usar regras simples baseadas na distância. | `strada2025ubi` |
| [4] | 2021 | Guillén, Nielsen, Pérez-Marín. *Near-miss telematics in motor insurance*. Journal of Risk & Insurance 88(3), 569-589. DOI 10.1111/jori.12340 | Frenagem e aceleração bruscas e uso do celular aumentam o custo. | `guillen2021nearmiss` |
| [5] | 2026 | Florea, Perțicaș, Juma. *Managing Energy Transfer Inefficiency in Personal Diesel Vehicles Using Telematics: A Behavioral and Spatial Analysis*. Sustainability 18(12), 6212. DOI 10.3390/su18126212 | Análise espacial com mapa de calor de ineficiência; diesel de uso pessoal. | `florea2026diesel` |
| [6] | 2023 | Zhang, Demir, Mason, Di Cairano-Gilfedder. *Understanding freight drivers' behavior and the impact on vehicles' fuel consumption and CO2e emissions*. Operational Research 23(4). DOI 10.1007/s12351-023-00798-2 | Mais de 4.000 registros; aceleração, frenagem e curvas bruscas, rotação excessiva e limites legais. Recorte: frete. | `zhang2023freight` |
| [7] | 2025 | Michailidis, Panagiotopoulou, Papadakis. *A Review of OBD-II-Based Machine Learning Applications for Sustainable, Efficient, Secure, and Safe Vehicle Driving*. Sensors 25(13), 4057. DOI 10.3390/s25134057 | Mapeia parâmetros OBD por aplicação; não ranqueia por frequência. | `michailidis2025review` |
| [8] | 2020 | Massaro, Selicato, Galiano. *Predictive Maintenance of Bus Fleet by Intelligent Smart Electronic Board Implementing Artificial Intelligence*. IoT 1(2), 180-197. DOI 10.3390/iot1020012 | Correlaciona comportamento do motorista e esforço do motor para manutenção de ônibus. Recorte: ônibus. | `massaro2020bus` |
| [9] | 2025 | Molina-Campoverde, Zurita-Jara, Molina-Campoverde. *Driving Pattern Analysis, Gear Shift Classification, and Fuel Efficiency in Light-Duty Vehicles: A Machine Learning Approach Using GPS and OBD II PID Signals*. Sensors 25(13), 4043. DOI 10.3390/s25134043 | Sinais VSS, RPM, MAP, MAF, IAT, ECT, TPS e GPS a 1 Hz; a razão VSS/RPM sozinha não basta para identificar a marcha; três veículos em uma cidade. | `molinacampoverde2025gears` |
| [10] | 2024 | Fadzil, Fadzir, Mansor, Rahardja. *Driver Behaviour Classification: A Research using OBD-II Data and Machine Learning*. JARASET 56(3), 147-157. DOI 10.37934/araset.56.3.147157 | Dados do Kaggle e rótulos de K-Means; a acurácia de 99% não comprova desempenho em campo. | `fadzil2024behaviour` |
| [11] | 2022 (online antecipada em 2020) | Oh, LeBlanc, Peng. *Vehicle Energy Dataset (VED), A Large-Scale Dataset for Vehicle Energy Consumption Research*. IEEE T-ITS 23(4), 3302-3312. DOI 10.1109/tits.2020.3035596 | 383 veículos; `correction = (1 + STFT/100 + LTFT/100)/AFR`; início e fim de GPS revelam dados privados. | `oh2022ved` |
| [12] | 2023 (online em 24/12/2022) | Hu, Frey, Boroujeni. *Contribution of Cold Starts to Real-World Trip Emissions for Light-Duty Gasoline Vehicles*. Atmosphere 14(1), 35. DOI 10.3390/atmos14010035 | 37 veículos; partida a frio dirigindo representa cerca de 4,5% do CO₂ da viagem. | `hu2023coldstart` |
| [13] | 2026 | Ogidan, Joseph, Onofeghara. *Development of an IOT-Based Fuel Level and Geolocation Monitoring System*. AZOJETE 22(2). DOI 10.63958/azojete/2026/22/02/008 | Fonte fraca, usada por falta de base melhor para o nível do tanque. | `ogidan2026fuellevel` |
| [14] | 2025 | Zhang et al. *Extended Vehicle Energy Dataset (eVED)*. IEEE VTC2025-Spring. DOI 10.1109/vtc2025-spring65109.2025.11174415 | Enriquecimento do VED com limite de velocidade e dados de mapa. A versão arXiv (2022) é `zhang2022evedarxiv`. | `zhang2025eved` |

Referências cujo ano varia conforme a base consultada:

- [3]: a OpenAlex traz publicação em 19/07/2024, mas o volume 29, número 1 é de 01/2025 na Crossref (online em 28/03/2025). Foi adotado o ano do volume (2025). Conferir na página do periódico antes de fechar o TCC.
- [11]: o volume 23(4) é de 04/2022 (Crossref); a OpenAlex indica 2020 pela publicação antecipada (17/11/2020).
- [12]: a Crossref e a OpenAlex trazem só a data online (24/12/2022). O ano 2023 vem do volume 14 do periódico; a Crossref não informa a data de impressão. Conferir.

As demais fontes consultadas, sem número de referência, estão no `.bib` com o ano no início do comentário (inclusive Winlaw 2019, Reig Torra 2023, Vagg 2013, DeFries 2014 e o estudo CSME 2022, este citado em `docs/pids-candidatos-consumo.md`).

## Nível de leitura

Antes de citar números ou detalhes de qualquer fonte, leia o texto completo. O que foi feito até agora:

| N. | Ano | Nível de leitura |
|---|---|---|
| [9] | 2025 | Texto completo ou quase |
| [11] | 2022 | Texto completo ou quase no trecho sobre consumo e privacidade |
| [7] | 2025 | Parcial: seções 1 a 5.1 e início da 5.2 |
| [1] | 2024 | Metadados e resumo |
| [2] | 2024 | Metadados e resumo |
| [3] | 2025 | Metadados e resumo |
| [4] | 2021 | Metadados e resumo |
| [5] | 2026 | Metadados e resumo |
| [6] | 2023 | Metadados e resumo |
| [8] | 2020 | Metadados e resumo |
| [10] | 2024 | Metadados e resumo |
| [12] | 2023 | Metadados e resumo |
| [13] | 2026 | Metadados e resumo |
| [14] | 2025 | Metadados e resumo |

Autores, título, ano e DOI foram conferidos na OpenAlex e na Crossref.

## Recortes das fontes

O texto do TCC deve declarar o recorte de cada fonte ao citá-la:

- [5] (2026): diesel de uso pessoal. Não se aplica sem ressalva a um carro a gasolina ou flex.
- [6] (2023): motoristas do setor de frete.
- [8] (2020): ônibus.
- [12] (2023): foco em emissões de veículos leves a gasolina; também mede o incremento de combustível na partida a frio em marcha lenta.
- [10] (2024): dados do Kaggle com rótulos gerados por K-Means. A acurácia de 99% mede a concordância com um agrupamento, não o acerto em campo.
- [9] (2025): três veículos em uma cidade.

## Definições: apoio direto e indireto

- **Direta**: a literatura de seguro (ou de frota) usa esse sinal, ou uma grandeza derivada dele, como variável do estudo ou como base para medir a necessidade daquela aplicação (por exemplo, o MAF para estimar consumo).
- **Indireta**: o sinal se justifica por consumo, estilo de condução ou estado do motor, mas as fontes não o ligam a sinistro nem a custo de seguro. Quando a coluna "Seguro" ou "Frota" diz "Indireta", o texto do TCC deve usar essa palavra e a justificativa de engenharia, sem inflar a evidência.
- **Não se aplica**: o sinal não tem uso nessa aplicação.

A ressalva central: a literatura de seguro apoia diretamente velocidade, aceleração e frenagem derivadas, distância e contexto (via, horário). Rotação, pedal, carga, MAF e temperatura se justificam por consumo, estilo de condução e estado do motor. **O texto do TCC não deve afirmar que esses sinais preveem sinistro.**

## Tabela PID → referência

Os períodos nominais usam `Tmax` = 50 ms (`Pᵢ = kᵢ·Tmax`). As medianas vêm do PDF de referência `Análise de Viagem - volante.pdf` (8.202 leituras, boot 8).

| PID | Período nominal / mediana | Permite mostrar | Seguro | Frota | Base |
|---|---|---|---|---|---|
| `0x0D` Velocidade | 250 ms / 130 ms | Cor da rota, distância, velocidade, aceleração, frenagem | Direta | Direta | [1] (2024), [2] (2024), [6] (2023) |
| `0x0C` Rotação | 250 ms / 130 ms | Marcha estimada, trocas, faixas de rotação | Indireta | Direta | [6] (2023), [7] (2025), [9] (2025), [10] (2024) |
| `0x49` Pedal | 250 ms / 130 ms | Agressividade, pedal solto, freio-motor | Indireta | Indireta | [7] (2025), [10] (2024) |
| `0x04` Carga | 600 ms / 300 ms | Esforço do motor, contexto do consumo | Indireta | Indireta | [7] (2025), [8] (2020), [10] (2024) |
| `0x10` MAF | 600 ms / 300 ms | Consumo estimado | Não se aplica | Direta | [7] (2025), [11] (2022) |
| `0x05` Líquido de arrefecimento | 5 s / 5 s | Aquecimento, esforço com motor frio | Não se aplica | Indireta | [7] (2025), [12] (2023) |
| `0x2F` Nível do tanque | 10 s / 10 s | Abastecimento, queda anormal | Não se aplica | Indireta (base fraca) | [13] (2026) |
| GPS `0xFF` | 1 s (até 60 s) | Traçado, altitude, contexto de via | Direta | Direta | [2] (2024), [5] (2026), [9] (2025), [11] (2022) |

Notas:

- A mediana de 5 s e 10 s dos PIDs lentos é a observada no PDF; o período nominal é o mesmo.
- O GPS (`0xFF`) vem do app, tem padrão de 1 s e é configurável de 1 a 60 s (`mobile-app/lib/src/domain/sync/sync_settings.dart`). O `t_ms` do GPS é marcado na chegada ao dongle, não na leitura: a 130 km/h, 1 s de atraso equivale a cerca de 36 m.
- Os períodos nominais são os padrões de `esp32-firmware/src/collector/CollectorConfig.cpp`.

## Justificativa por PID

**`0x0D` Velocidade.** É o sinal central. Gera distância (integral), aceleração e frenagem (derivada) e a cor da rota. É a variável mais comum nos estudos de [1] (2024) e entra em [2] (2024) (excesso de velocidade) e [6] (2023) (limites legais). A velocidade OBD é inteira (km/h), o que limita a resolução da aceleração derivada.

**`0x0C` Rotação.** Com a velocidade, estima a marcha pela razão rotação/velocidade. Ressalva de [9] (2025): essa razão sozinha não basta para identificar a marcha. [9] usa sinais adicionais e tem apenas três veículos em uma cidade. Em [6] (2023) a rotação excessiva é um fator de consumo. Para o seguro, o apoio é indireto.

**`0x49` Pedal.** Representa a intenção do motorista. A posição da borboleta também responde à ECU, e esse é um argumento de engenharia do projeto, não vem dos artigos. A associação do pedal a [7] (2025) e [10] (2024) vem do levantamento inicial (parâmetros OBD por aplicação); o que cada uma diz exatamente sobre pedal ou borboleta ainda precisa ser conferido no texto.

**`0x04` Carga.** Dá o esforço do motor e o contexto do consumo. [8] (2020) relaciona comportamento do motorista e esforço do motor, no recorte de ônibus.

**`0x10` MAF.** É a base do consumo no VED [11] (2022) quando não há taxa de combustível: combustível = MAF dividido pela relação ar/combustível. O resultado depende do combustível usado e dos trims (sobre os trims, ver `docs/pids-candidatos-consumo.md`).

**`0x05` Temperatura do líquido.** Indica o uso do motor (aquecimento, esforço com motor frio), não o consumo. Em [12] (2023) a partida a frio pesa nas emissões, mas pouco no CO₂ (cerca de 4,5% do CO₂ da viagem em dirigir a frio).

**`0x2F` Nível do tanque.** Tem a base mais fraca: apenas [13] (2026), uma fonte fraca usada por falta de base melhor. Serve para detectar abastecimento e queda anormal, não para calcular consumo (resolução baixa, 10 s, e o nível oscila com o movimento do líquido; esta última frase é inferência do projeto).

**GPS `0xFF`.** Dá o traçado e a altitude. Contexto de via e horário aparecem em [2] (2024); a análise espacial com mapa de calor, em [5] (2026), no recorte de diesel pessoal. Em [9] (2025) o GPS a 1 Hz entra junto com os PIDs. Em [11] (2022) o início e o fim de GPS revelam dados privados.

## Necessidade × Seguro × Frota × Evidência

| Necessidade | Seguro | Frota | Evidência |
|---|---|---|---|
| Quanto rodou | Sim | Sim | [3] (2025) |
| Como dirigiu | Sim | Sim | [1] (2024), [2] (2024), [4] (2021) |
| Onde e quando | Sim | Sim | [2] (2024), [5] (2026) |
| Quanto gastou | Indireto | Sim | [6] (2023), [7] (2025) |
| Como o veículo foi usado | Indireto | Sim | [6] (2023), [8] (2020) |

"Indireto" repete a definição acima. [3] (2025) registra que o UBI costuma usar regras simples baseadas na distância, o que torna "quanto rodou" a necessidade com mais apoio.

## Lacunas sem PID

Informações que a literatura usa e que os PIDs atuais não entregam:

| Lacuna | Fonte | Como seria atendida |
|---|---|---|
| Limite de velocidade | [14] (2025) enriquece dados com limite de velocidade e mapa | Dados de mapa; fora do escopo atual |
| Tipo de via | [2] (2024) | Dados de mapa |
| Curvas bruscas | [6] (2023) | Acelerômetro do celular ou GPS |
| Uso do celular | [4] (2021) | Fora do alcance do dongle |
| Códigos de falha | (sem fonte) | Modo diagnóstico OBD, não é PID de telemetria |

## Normas a conferir

- **Código de Trânsito Brasileiro (Lei 9.503/1997), art. 61**: velocidade. Não foi consultado nesta pesquisa; conferir o texto vigente. A tela de análise de viagem não depende desta norma: suas faixas de velocidade são fixas e não representam o limite da via.
- **LGPD (Lei 13.709/2018)**: dados pessoais. Localização de início e fim de viagem pode revelar locais sensíveis [11] (2022). Não foi consultada nesta pesquisa; conferir o texto vigente.
- **SAE J1979**: definição dos PIDs. Números de PID e fatores de decodificação citados nos documentos do projeto vêm de memória, não da norma, e devem ser conferidos nela. A página "OBD-II PIDs" da Wikipédia serve só para consulta rápida; no TCC, cite a norma.

## Pendências

- Ler o texto completo de [1] (2024), [2] (2024), [3] (2025), [4] (2021), [5] (2026), [6] (2023), [8] (2020), [10] (2024), [12] (2023), [13] (2026) e [14] (2025), e concluir a leitura de [7] (2025), antes de citar números.
- Confirmar o ano de [3] (2025) e [12] (2023) (ver divergências acima).
- Confirmar o nome do periódico e as páginas de [13] (a OpenAlex traz só a primeira página).
