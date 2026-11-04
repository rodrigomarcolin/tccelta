/**
 * Tipos do JSON da análise de viagem (spec 02 + 04).
 * Ausência de dado é sempre `null`, nunca `0`.
 */

export type FaixaId = "parado" | "ate_30" | "30_60" | "60_80" | "80_110" | "acima_110";
export type FaixaSegmento = FaixaId | "falha_gps" | "sem_velocidade";

export type CombustivelId =
  | "gasolina_e30"
  | "gasolina_e32"
  | "gasolina_e27"
  | "gasolina_pura"
  | "etanol_hidratado"
  | "diesel";

export type StatusViagem = "pendente" | "aguardando_dados" | "pronta" | "falhou";

export type EntradaBoot = {
  tipo: "boot";
  dongle_id: number;
  boot_generation?: number | null;
  boot_id: number;
};

export type EntradaIntervalo = {
  tipo: "intervalo";
  dongle_id: number;
  t0: string;
  t1: string;
};

export type Entrada = EntradaBoot | EntradaIntervalo;

export type Relogio = { deslocamento_ms: number; incerteza_ms: number };

export type Parte = {
  boot_generation: number;
  boot_id: number;
  dados_inicio_t_ms: number;
  dados_fim_t_ms: number;
  inicio_t_ms: number;
  fim_t_ms: number;
};

export type MotoristaRef = { id: string; nome: string };

export type Viagem = {
  entrada: Entrada;
  motorista: MotoristaRef;
  categoria_viagem: string;
  inicio_t_ms: number | null;
  fim_t_ms: number | null;
  inicio_em: string | null;
  fim_em: string | null;
  relogio: Relogio | null;
  partes: Parte[];
};

export type Parametros = {
  combustivel: CombustivelId;
  afr: number;
  densidade_g_l: number;
  marcha_lenta_margem_rpm: number;
  gps_falha_s: number;
  [chave: string]: string | number | boolean | null;
};

export type Aviso = { codigo: string; detalhe?: string | null };

export type Resumo = {
  distancia_km: number | null;
  duracao_s: number | null;
  sem_dados_s: number | null;
  em_movimento_s: number | null;
  parado_s: number | null;
  velocidade_media_kmh: number | null;
  velocidade_maxima_kmh: number | null;
  motor_ligado_s: number | null;
  marcha_lenta_s: number | null;
  marcha_lenta_pct: number | null;
  combustivel_l: number | null;
  km_por_l: number | null;
  n_acel_bruscas: number | null;
  n_fren_bruscas: number | null;
  acel_por_100km: number | null;
  fren_por_100km: number | null;
};

export type Condutor = {
  pedal_medio_pct: number | null;
  pedal_alto_s: number | null;
  pedal_variacao_p95_pct_s: number | null;
  freio_motor_pct: number | null;
  aceleracao_rms_ms2: number | null;
  cruzeiro_s: number | null;
};

export type Motor = {
  aquecimento_s: number | null;
  temperatura_maxima_c: number | null;
  esforco_frio_s: number | null;
};

export type FaixaVelocidade = {
  id: FaixaId;
  de_kmh: number;
  ate_kmh: number | null;
  tempo_s: number;
  distancia_km: number;
  distancia_pct: number | null;
};

export type Rota = {
  t_ms: (number | null)[];
  lat: (number | null)[];
  lon: (number | null)[];
  segmentos: {
    velocidade_kmh: (number | null)[];
    faixa: FaixaSegmento[];
  };
};

export type Parada = {
  t_inicio_ms: number;
  duracao_s: number;
  motor_ligado: boolean;
  lat: number | null;
  lon: number | null;
};

export type TipoEvento = "frenagem_brusca" | "aceleracao_brusca" | "marcha_lenta_longa";

export type Evento = {
  tipo: TipoEvento;
  t_inicio_ms: number;
  duracao_s: number;
  pico: number;
  pico_unidade: "m/s2" | "s";
  velocidade_inicial_kmh: number | null;
  velocidade_final_kmh: number | null;
  lat: number | null;
  lon: number | null;
};

export type Patamar = { marcha: number; rpm_por_kmh: number; tempo_s: number; tempo_pct: number };

export type Troca = {
  t_ms: number;
  direcao: "subida" | "reducao";
  de: number;
  para: number;
  rpm_antes: number;
  rpm_depois: number;
  velocidade_kmh: number;
};

export type Marchas = {
  marcha_lenta: {
    rpm: number;
    origem: "estimada" | "informada" | "reserva";
    margem_rpm: number;
  } | null;
  patamares: Patamar[];
  ponto_morto_embreagem_s: number | null;
  trocas: Troca[];
  subida_rpm: { mediana: number | null; p25: number | null; p75: number | null; n: number };
  reducao_rpm: { mediana: number | null; n: number };
};

export type FaixaDistribuicao = {
  de: number;
  ate: number | null;
  rotulo?: string;
  tempo_s: number;
  pct: number;
};

export type Distribuicoes = {
  rotacao: FaixaDistribuicao[];
  pedal: FaixaDistribuicao[];
  rotacao_pedal: { rpm_bin: number; pedal_bin: number; tempo_s: number[][] };
};

export type Series = {
  passo_ms: number;
  exposicao_passo_ms: number;
  t_ms: (number | null)[];
  velocidade_kmh: (number | null)[];
  rpm: (number | null)[];
  rpm_por_kmh: (number | null)[];
  marcha: (number | null)[];
  pedal_pct: (number | null)[];
  aceleracao_ms2: (number | null)[];
  carga_pct: (number | null)[];
  consumo_l_h: (number | null)[];
  combustivel_acumulado_l: (number | null)[];
  distancia_acumulada_km: (number | null)[];
};

export type SeriesLentas = {
  temperatura: { t_ms: (number | null)[]; c: (number | null)[] };
  nivel_tanque: {
    t_ms: (number | null)[];
    pct: (number | null)[];
    mediana_movel_pct: (number | null)[];
  };
};

export type Qualidade = {
  leituras_total: number;
  leituras_por_s: number | null;
  periodos_mediana_ms: Record<string, number | null>;
  combustivel_cobertura_pct: number | null;
  gps: {
    periodo_mediana_s: number | null;
    cobertura_pct: number | null;
    n_falhas: number | null;
    maior_falha_s: number | null;
    distancia_km: number | null;
    diferenca_distancia_pct: number | null;
  };
};

export type Analise = {
  versao_calculo: string;
  viagem: Viagem;
  parametros: Parametros;
  avisos: Aviso[];
  resumo: Resumo;
  condutor: Condutor;
  motor: Motor;
  faixas_velocidade: FaixaVelocidade[];
  rota: Rota | null;
  paradas: Parada[];
  eventos: Evento[];
  marchas: Marchas;
  distribuicoes: Distribuicoes;
  series: Series;
  series_lentas: SeriesLentas;
  qualidade: Qualidade;
};

export type StatusResposta = {
  viagem_id: string;
  status: StatusViagem;
  provisoria: boolean;
  motorista: { nome: string };
  categoria_viagem: string;
  combustivel: CombustivelId;
  entrada: Entrada;
  criada_em: string;
  calculada_em: string | null;
};

export type CriarViagemEntrada = {
  motorista: { nome: string };
  categoria_viagem: string;
  combustivel: string;
  entrada: Entrada;
};

export type CriarViagemResposta = { viagem_id: string; status: StatusViagem };

export type Problema = {
  type: string;
  title: string;
  status: number;
  codigo: string;
  detail?: string;
  viagem_status?: StatusViagem;
};

export type Resposta<T> = { status: number; corpo: T | Problema };

export function ehProblema(corpo: unknown): corpo is Problema {
  return (
    typeof corpo === "object" &&
    corpo !== null &&
    "codigo" in corpo &&
    "title" in corpo &&
    "status" in corpo
  );
}
