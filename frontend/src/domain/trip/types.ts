/**
 * Types of the trip analysis JSON (spec 02 + 04).
 * Missing data is always `null`, never `0`.
 */

export type BandId = "parado" | "ate_30" | "30_60" | "60_80" | "80_110" | "acima_110";
export type SegmentBand = BandId | "falha_gps" | "sem_velocidade";

export type FuelId =
  | "gasolina_e30"
  | "gasolina_e32"
  | "gasolina_e27"
  | "gasolina_pura"
  | "etanol_hidratado"
  | "diesel";

export type TripStatus = "pendente" | "aguardando_dados" | "pronta" | "falhou";

export type BootInput = {
  tipo: "boot";
  dongle_id: number;
  boot_generation?: number | null;
  boot_id: number;
};

export type IntervalInput = {
  tipo: "intervalo";
  dongle_id: number;
  t0: string;
  t1: string;
};

export type TripInput = BootInput | IntervalInput;

export type Clock = { deslocamento_ms: number; incerteza_ms: number };

export type TripPart = {
  boot_generation: number;
  boot_id: number;
  dados_inicio_t_ms: number;
  dados_fim_t_ms: number;
  inicio_t_ms: number;
  fim_t_ms: number;
};

export type DriverRef = { id: string; nome: string };

export type Trip = {
  entrada: TripInput;
  motorista: DriverRef;
  categoria_viagem: string;
  inicio_t_ms: number | null;
  fim_t_ms: number | null;
  inicio_em: string | null;
  fim_em: string | null;
  relogio: Clock | null;
  partes: TripPart[];
};

export type Parameters = {
  combustivel: FuelId;
  afr: number;
  densidade_g_l: number;
  marcha_lenta_margem_rpm: number;
  gps_falha_s: number;
  [key: string]: string | number | boolean | null;
};

export type TripWarning = { codigo: string; detalhe?: string | null };

export type TripSummary = {
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

export type DrivingStats = {
  pedal_medio_pct: number | null;
  pedal_alto_s: number | null;
  pedal_variacao_p95_pct_s: number | null;
  freio_motor_pct: number | null;
  aceleracao_rms_ms2: number | null;
  cruzeiro_s: number | null;
};

export type EngineStats = {
  aquecimento_s: number | null;
  temperatura_maxima_c: number | null;
  esforco_frio_s: number | null;
};

export type SpeedBand = {
  id: BandId;
  de_kmh: number;
  ate_kmh: number | null;
  tempo_s: number;
  distancia_km: number;
  distancia_pct: number | null;
};

export type Route = {
  t_ms: (number | null)[];
  lat: (number | null)[];
  lon: (number | null)[];
  segmentos: {
    velocidade_kmh: (number | null)[];
    faixa: SegmentBand[];
  };
};

export type Stop = {
  t_inicio_ms: number;
  duracao_s: number;
  motor_ligado: boolean;
  lat: number | null;
  lon: number | null;
};

export type EventType = "frenagem_brusca" | "aceleracao_brusca" | "marcha_lenta_longa";

export type TripEvent = {
  tipo: EventType;
  t_inicio_ms: number;
  duracao_s: number;
  pico: number;
  pico_unidade: "m/s2" | "s";
  velocidade_inicial_kmh: number | null;
  velocidade_final_kmh: number | null;
  lat: number | null;
  lon: number | null;
};

export type GearPlateau = {
  marcha: number;
  rpm_por_kmh: number;
  tempo_s: number;
  tempo_pct: number;
};

export type GearShift = {
  t_ms: number;
  direcao: "subida" | "reducao";
  de: number;
  para: number;
  rpm_antes: number;
  rpm_depois: number;
  velocidade_kmh: number;
};

export type GearStats = {
  marcha_lenta: {
    rpm: number;
    origem: "estimada" | "informada" | "reserva";
    margem_rpm: number;
  } | null;
  patamares: GearPlateau[];
  ponto_morto_embreagem_s: number | null;
  trocas: GearShift[];
  subida_rpm: { mediana: number | null; p25: number | null; p75: number | null; n: number };
  reducao_rpm: { mediana: number | null; n: number };
};

export type DistributionBin = {
  de: number;
  ate: number | null;
  rotulo?: string;
  tempo_s: number;
  pct: number;
};

export type Distributions = {
  rotacao: DistributionBin[];
  pedal: DistributionBin[];
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

export type SlowSeries = {
  temperatura: { t_ms: (number | null)[]; c: (number | null)[] };
  nivel_tanque: {
    t_ms: (number | null)[];
    pct: (number | null)[];
    mediana_movel_pct: (number | null)[];
  };
};

export type TripQuality = {
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

export type TripAnalysis = {
  versao_calculo: string;
  viagem: Trip;
  parametros: Parameters;
  avisos: TripWarning[];
  resumo: TripSummary;
  condutor: DrivingStats;
  motor: EngineStats;
  faixas_velocidade: SpeedBand[];
  rota: Route | null;
  paradas: Stop[];
  eventos: TripEvent[];
  marchas: GearStats;
  distribuicoes: Distributions;
  series: Series;
  series_lentas: SlowSeries;
  qualidade: TripQuality;
};

export type TripStatusResponse = {
  viagem_id: string;
  status: TripStatus;
  provisoria: boolean;
  motorista: { nome: string };
  categoria_viagem: string;
  combustivel: FuelId;
  entrada: TripInput;
  criada_em: string;
  calculada_em: string | null;
};

export type CreateTripRequest = {
  motorista: { nome: string };
  categoria_viagem: string;
  combustivel: string;
  entrada: TripInput;
};

export type CreateTripResponse = { viagem_id: string; status: TripStatus };

export type Problem = {
  type: string;
  title: string;
  status: number;
  codigo: string;
  detail?: string;
  viagem_status?: TripStatus;
  /** Extras of `409 viagem_repetida`: the trip that already exists and its driver. */
  viagem_id?: string;
  motorista?: { nome: string };
};

/** Route summary: `[lat, lon]` points grouped in stretches (a GPS gap starts a new stretch). */
export type RoutePreviewData = [number, number][][];

export type TripListItem = {
  viagem_id: string;
  status: TripStatus;
  provisoria: boolean;
  motorista: { nome: string };
  categoria_viagem: string;
  entrada_tipo: "boot" | "intervalo";
  dongle_id: number;
  boot_id: number | null;
  boot_generation: number | null;
  inicio_em: string | null;
  fim_em: string | null;
  distancia_km: number | null;
  duracao_s: number | null;
  criada_em: string;
  rota_resumo: RoutePreviewData | null;
};

export type TripListResponse = {
  viagens: TripListItem[];
  total: number;
  /** Opaque; `null` on the last page. */
  proximo_cursor: string | null;
};

/** Query of `GET /v1/viagens`. */
export type ListQuery = { limite?: number; cursor?: string; busca?: string };

/** `retryAfter` (in seconds) comes from the API `Retry-After` header, when sent. */
export type ApiResponse<T> = { status: number; body: T | Problem; retryAfter?: number };

export function isProblem(value: unknown): value is Problem {
  return (
    typeof value === "object" &&
    value !== null &&
    "codigo" in value &&
    "title" in value &&
    "status" in value
  );
}
