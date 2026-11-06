import type { TripListItem, TripListResponse } from "@/domain/trip/types";

/** A list item with sensible defaults; `n` makes the id and name distinct. */
export function listItem(n: number, extra: Partial<TripListItem> = {}): TripListItem {
  return {
    viagem_id: `7d1f3a10-2000-4c5e-9b1a-${String(n).padStart(12, "0")}`,
    status: "pronta",
    provisoria: false,
    motorista: { nome: `Motorista ${n}` },
    categoria_viagem: "feira",
    entrada_tipo: "boot",
    dongle_id: 1,
    boot_id: n,
    boot_generation: 0,
    inicio_em: "2026-10-28T17:03:00.000Z",
    fim_em: "2026-10-28T17:33:20.000Z",
    distancia_km: 12.3,
    duracao_s: 1820,
    criada_em: "2026-10-28T17:40:00.000Z",
    rota_resumo: [
      [
        [-23.55, -46.64],
        [-23.54, -46.63],
        [-23.53, -46.62],
      ],
    ],
    ...extra,
  };
}

export function listPage(
  items: TripListItem[],
  total: number,
  cursor: string | null = null,
): TripListResponse {
  return { viagens: items, total, proximo_cursor: cursor };
}

/** Stand-in for `IntersectionObserver` that tests fire by hand. */
export class FakeObserver {
  static instances: FakeObserver[] = [];
  static active(): FakeObserver[] {
    return FakeObserver.instances.filter((o) => !o.disconnected);
  }
  static reset(): void {
    FakeObserver.instances = [];
  }
  disconnected = false;
  observed: Element[] = [];
  constructor(
    private readonly callback: IntersectionObserverCallback,
    readonly options?: IntersectionObserverInit,
  ) {
    FakeObserver.instances.push(this);
  }
  observe(el: Element) {
    this.observed.push(el);
  }
  unobserve() {}
  disconnect() {
    this.disconnected = true;
  }
  takeRecords() {
    return [];
  }
  /** The observed element enters (or stays in) the viewport. */
  fire(isIntersecting = true) {
    this.callback(
      [{ isIntersecting } as IntersectionObserverEntry],
      this as unknown as IntersectionObserver,
    );
  }
}
