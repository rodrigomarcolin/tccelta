Agora eu quero partir para a etapa de implementação, que deve englobar fases, cada uma com um conjunto de specs. As specs devem ser salvas em uma pasta .spec na raiz do repo e devem ter um aspecto mais técnico e próximo a um plano de implementação, uma vez que especificação mais aslto nível está em @latex/aux-docs/seguranca-dongle-server/segunraca-dongle-server.tex e @latex/aux-docs/firmware-coletor/Investigação-3.pdf

  # Firmware + App - integração de segurança e telemetria

  No firmware, vc deverá planejar na spec:
  - Armazenamento das chaves K_auth e K_rec na flash encriptada com todos os níveis de segunraça especificados no documento de segurança
  - Adequação do bloco de dados segundo a Figura 10
  - Autenticar com HMAC de 32 bytes via K_rec e calcular CRC32 de blocos quando eles são salvos na flash, respeitando o formato definido na Figura 8 do documento. Quero que essa autenticação via HMAC seja feita a partir de uma interface genérica, pois a ideia é implementar um perfil de desenvolvimento que use algortimo de assinatura digital com cripto assimétrica
  - Trocar blocos de medição BLE com o app no serviço SYNC, encapsulando blocos usando TLV e fragmentando com bit de controle e de size (Figura 7) a fim de trocar blocos grandes com o BLE usando MTU engociado. Lembrar de realizar a verificação do CRC antes de mandar para o app, mandando um bloco LOSS no lugar. Também é necessário alterar para armazenar blocos do tipo BOOT (pode tirar o bloco do tipo BATCH mencionado no doc d einvestigação). Os tipos de blocos (dados, LOSS e boot) estão na representados como um dos blocos da figura 11na figura 11
  - Trocar mensagens de controle relacionadas a handshake via serviço BLE SYNC: desafio, resposta, handler de pedido de lote, tratamento de erro (envio dos erros tipo Dongle -> App da Tabela 5 e processamento dos erros App -> Dongle), envio de aviso de novo bloco
  - Processar o desafio dando uma resposta ao handshake: verificar HMAC de 32 bytes do desafio, calcular a resposta como definino na figura 5 com os parâmetros da tabela 2, enviar uma resposta com tag HMAC usando K_auth a partir de um HMAC de 16 bytes
  - Uso de NVS para conseguir gravar seq + N (deixe N parametrizável, que é o chekpoint de seq) e recuperação em caso de boot
  - Implementação do cursor de envio para envio de dados do app seguindo a lógica de que no app terá um buffer para armazenar os dados
  - Lembre-se de cada evento que for fazer (calcular tag, calcular CRC e outros eventos relevantes etc devem ser registradas no mesmo padrão com logs estruturados para debugging no modo debug)

  No app, vc implementa:
  - Conexão com o serviço SYNC
  - Implementação das parametrizações com uma tela na paga de configurações: escolher T_lat e B_max, sendo espera entre tentativas configurada via constante no código
   - Implementação do envio e leitura das mensagens BLE, inlcuindo os blocos SYNC de desafio (remontando o bloco a partir de MTU negociado) e os outros de desafio, resposta, pedido de novo lote, aviso de novo bloco; no pedido e envio de novo bloco, implementar o cursor de envio
  - Envio de erros de erros App -> Dongle e processamento dos erros Dongle -> App
  - Envio de dados de GPS via comando AT a cada tempo parametrizado T_GPS ( adicionar esse tempo nas configs também)
  - Ainda não precisa implementa ro gatihlo de handshake, deixe apenas o molde para implementar o trigger de um hadshake real na etapa final de integração com server



  Entrega:
  Deve ser entregue o fluxo de leitura contínua (Figura 15) e de handshake (Figura 5, mas só da parte entre app e dongle)
  A ideia é ter toda a comunicação dongle app via BLE, ainda sem integração HTTPs com servidor, mas com prontidão do app para receber a comunicação e cotninue no fluxo do lado do BLE

  # Firmware + App - LESC

  A ideia dessa spec/plano é implementar toda a parte de conexão BLE com LESC Just Works seguindo o que está especificado no documento.
  Firmware:
  - Botão GPIO0 via botão boot da placa deve ser pressionar por 5s antes de o dongle ficar disponível para conexão durante 45s (tempos configuráveis em variáveis de ambiente)
  - Lógica para derrubar todos os bonds e conexões quando o botão for apertado
  - Lógica para evitar que novos dongles abram conexão para pegar disponibilidade se o botão não estiver apertado

  App:
  - Pareamento via BLE Just Works, fim a fim

  # Servidor - App + API e database + KMS
  A parte de server (API, database, KMS) deve ser implementado em umas pasta separa backend e sempre usando docker.
  A ideia desse spec é planejar a implementação do servidor segundo a seção 6 e também sua integração com o app
  Também deve ser especificado o servidor KMS e sua integração com o server da API, por conseirando mesmo server, mas dps será via VPN
  Server:
  - PostgreSQL com a implementação das tabelas documentadas via migration
  - API em Golang com goroutines para processmento assíncrono de altos volumes
  - Queries em plain SQL mas usando as melhores práticas da linguagem go para gerenciamento de queries via codegen e sem ter que escrever todo SQL como string no código
  - IMplementação do endpoint de ingestão de blocos e dos de desafio segundo os diagramas montados
  - Implementação do worker assíncrono para populção de tabelas mais granulares e processamento de âncora de tempo
  - Leitura das chaves K_auth e K_rec a partir de um KMS
  - Configuração do KMS para server poder apenas verificar HMAC com K_rec, podendo gerar e verificar com K)auth (lembrar que HMAC de K_auth é 16 bytes, truncado na metade)
  - Emissão do token KWT para autenticação do app
  App:
  - Integração dos endpoints de desafio e resposta para obteção de token JWT. Garanta que será possível decodificar o base64 e usar binário quando necessário para o BLE e vice versa
  - Integração do endpoint de envio de lotes de blocos e recebimento de ACK
  - Integração dos gatilhos de handshake a partir de expiração de token e outros eventos especficados no documento


  COMO FAZER TODOS ESSES SPECS
  Use esse chat de orquestrador para dar orientações a partir dos docs de referência e use subagentes com opus 5.5 para fazer cada um desses planos. Esses subagentes devem usar fable como advisor internamente. Esse chat deve dar aos subagesntes, a partir do prompt de cada plano, o contexto necessário dos documentos de referência para que cada spec saia como um plano preparado para implementação