#include "IsoTpClient.h"
#include <cstring>

// Plataforma: no ESP32 (Arduino) usa millis()/delay()/taskYIELD() nativos.
// Em build nativo (host, usado pelos testes em test/) não há Arduino.h nem
// FreeRTOS — fornece equivalentes baseados em <chrono> para que este
// arquivo compile e se comporte da mesma forma nos dois ambientes.
#if defined(ARDUINO)
#include <Arduino.h>
#else
#include <chrono>
#include <thread>
namespace {
uint32_t millis() {
    using namespace std::chrono;
    static const auto start = steady_clock::now();
    return (uint32_t)duration_cast<milliseconds>(steady_clock::now() - start).count();
}
void delay(uint32_t ms) { std::this_thread::sleep_for(std::chrono::milliseconds(ms)); }
void delayMicroseconds(uint32_t us) { std::this_thread::sleep_for(std::chrono::microseconds(us)); }
void taskYIELD() {}
}  // namespace
#endif

namespace IsoTp {
namespace {

constexpr uint8_t N_PCI_SF = 0x00;
constexpr uint8_t N_PCI_FF = 0x10;
constexpr uint8_t N_PCI_CF = 0x20;
constexpr uint8_t N_PCI_FC = 0x30;

constexpr uint8_t FS_CTS      = 0x00;
constexpr uint8_t FS_WAIT     = 0x01;
constexpr uint8_t FS_OVERFLOW = 0x02;

// Convenção padrão de endereçamento OBD-II 11-bit (também usada no
// simulador, ver simulador/ECUSim/ECUSim.h): ID de resposta física = ID de
// requisição física + 8 (0x7E0/0x7E8, 0x7E1/0x7E9, ...). O Flow Control é
// sempre endereçado à ECU especifica que mandou o First Frame, nunca ao
// broadcast funcional (0x7DF) usado para a requisição original.
constexpr uint32_t physicalRequestIdFor(uint32_t physicalResponseId) {
    return physicalResponseId - 0x008;
}

bool inRange(uint32_t id, uint32_t lo, uint32_t hi) { return id >= lo && id <= hi; }

void sendSf(ICanBus* can, uint32_t id, const uint8_t* data, uint8_t len) {
    CanFrame f = {};
    f.id  = id;
    f.dlc = 8;
    f.data[0] = (uint8_t)(N_PCI_SF | len);
    memcpy(&f.data[1], data, len);
    can->send(f);
}

void sendFf(ICanBus* can, uint32_t id, const uint8_t* data, uint16_t totalLen) {
    CanFrame f = {};
    f.id  = id;
    f.dlc = 8;
    f.data[0] = (uint8_t)(N_PCI_FF | ((totalLen >> 8) & 0x0F));
    f.data[1] = (uint8_t)(totalLen & 0xFF);
    memcpy(&f.data[2], data, 6);
    can->send(f);
}

void sendCf(ICanBus* can, uint32_t id, const uint8_t* data, uint8_t len, uint8_t seq) {
    CanFrame f = {};
    f.id  = id;
    f.dlc = 8;
    f.data[0] = (uint8_t)(N_PCI_CF | (seq & 0x0F));
    memcpy(&f.data[1], data, len);
    can->send(f);
}

void sendFc(ICanBus* can, uint32_t id, uint8_t fs, uint8_t bs, uint8_t stmin) {
    CanFrame f = {};
    f.id  = id;
    f.dlc = 8;
    f.data[0] = (uint8_t)(N_PCI_FC | fs);
    f.data[1] = bs;
    f.data[2] = stmin;
    can->send(f);
}

// Decodifica STmin (byte 2 do FC) em microssegundos.
uint32_t stMinToMicros(uint8_t stmin) {
    if (stmin <= 0x7F) return (uint32_t)stmin * 1000u;
    if (stmin >= 0xF1 && stmin <= 0xF9) return (uint32_t)(stmin - 0xF0) * 100u;
    return 0x7Fu * 1000u;  // reservado -> trata como 127ms
}

void waitStMin(uint8_t stmin) {
    uint32_t us = stMinToMicros(stmin);
    if (us == 0) return;
    if (us >= 1000) delay(us / 1000);
    else            delayMicroseconds(us);
}

// Resultado interno do envio segmentado (lado emissor).
enum class SendOutcome { OK, TIMEOUT, OVERFLOW };

// Envia `len` (>7) bytes como FF + CF(s), obedecendo o(s) FC(s) do peer
// (FlowStatus, Block Size, STmin). `fcIdMin`/`fcIdMax` é a faixa de IDs em
// que se aceita o FC.
SendOutcome sendSegmented(ICanBus* can, uint32_t txId, uint32_t fcIdMin, uint32_t fcIdMax,
                           const uint8_t* data, size_t len) {
    sendFf(can, txId, data, (uint16_t)len);
    size_t  sent = 6;
    uint8_t seq  = 1;
    uint8_t fcWaitCount = 0;

    while (sent < len) {
        uint32_t deadline = millis() + N_BS_MS;
        CanFrame f;
        bool    gotFc = false;
        uint8_t fs = FS_CTS, bs = 0, stmin = 0;

        while (millis() < deadline) {
            if (!can->receive(f)) { taskYIELD(); continue; }
            if (!inRange(f.id, fcIdMin, fcIdMax)) continue;
            if ((f.data[0] & 0xF0) != N_PCI_FC) continue;
            fs    = f.data[0] & 0x0F;
            bs    = f.data[1];
            stmin = f.data[2];
            gotFc = true;
            break;
        }
        if (!gotFc) return SendOutcome::TIMEOUT;
        if (fs == FS_OVERFLOW) return SendOutcome::OVERFLOW;
        if (fs == FS_WAIT) {
            if (++fcWaitCount >= MAX_FC_WAIT) return SendOutcome::TIMEOUT;
            continue;  // aguarda outro FC, sem consumir bytes
        }
        fcWaitCount = 0;  // FS_CTS

        uint8_t sentThisBlock = 0;
        while (sent < len) {
            uint8_t chunk = (uint8_t)((len - sent) < 7 ? (len - sent) : 7);
            waitStMin(stmin);
            sendCf(can, txId, data + sent, chunk, seq);
            sent += chunk;
            seq   = (uint8_t)((seq + 1) & 0x0F);
            sentThisBlock++;
            if (bs != 0 && sentThisBlock >= bs) break;  // aguarda novo FC
        }
    }
    return SendOutcome::OK;
}

}  // namespace

int request(ICanBus* can, uint32_t reqId, uint32_t respIdMin, uint32_t respIdMax,
            const uint8_t* req, size_t reqLen,
            uint8_t* outBuf, size_t maxLen,
            uint32_t timeoutMs) {
    // Drena qualquer frame que já esteja na fila de recepção antes de mandar
    // esta requisição. O uso deste cliente é sempre síncrono (manda, espera
    // a resposta, só então a próxima chamada manda a próxima requisição) —
    // então nada deveria estar pendente na fila neste ponto; qualquer coisa
    // que esteja é, por construção, sobra de uma transação anterior (ex.:
    // a 2ª ECU que respondeu a um broadcast funcional, cuja resposta não foi
    // a escolhida). O filtro por SID esperado abaixo já cobre o caso de uma
    // sobra chegar DURANTE a espera desta transação; mas quando duas
    // requisições seguidas esperam o MESMO SID de resposta (ex.: várias
    // leituras de PID do freeze frame, todas com SID 0x42), o filtro por SID
    // não consegue distinguir a sobra da resposta de verdade — só o dreno
    // aqui resolve isso.
    CanFrame stale;
    while (can->receive(stale)) { /* descarta */ }

    if (reqLen <= 7) {
        sendSf(can, reqId, req, (uint8_t)reqLen);
    } else {
        SendOutcome rc = sendSegmented(can, reqId, respIdMin, respIdMax, req, reqLen);
        if (rc == SendOutcome::OVERFLOW) return OVERFLOW_ABORT;
        if (rc != SendOutcome::OK)       return TIMEOUT;
    }

    // SID de resposta esperado por convenção OBD-II/SAE-J1979 (camada acima do
    // ISO-TP): positiva = SID da requisição | 0x40; negativa = sempre 0x7F.
    // Requisições funcionais (broadcast 0x7DF) recebem resposta de CADA ECU
    // que "escutou" o pedido — inclusive para serviços de outro request feito
    // um instante antes, se aquela resposta ainda não tiver sido drenada da
    // fila. Sem checar o SID aqui, a primeira ECU a responder a uma
    // transação anterior (ainda não lida) seria confundida com a resposta
    // da transação atual. Descartar e continuar esperando (dentro do
    // timeout) drena esses frames obsoletos até achar o que realmente
    // corresponde a este pedido.
    uint8_t expectedReplySid = (uint8_t)(req[0] | 0x40u);

    // Espera a primeira resposta (SF ou FF) do peer.
    uint32_t deadline = millis() + timeoutMs;
    CanFrame f;
    while (true) {
        if (millis() >= deadline) return TIMEOUT;
        if (!can->receive(f)) { taskYIELD(); continue; }
        if (!inRange(f.id, respIdMin, respIdMax)) continue;

        uint8_t pciType = f.data[0] & 0xF0;

        if (pciType == N_PCI_SF) {
            uint8_t len = f.data[0] & 0x0F;
            if (len == 0) continue;  // SF vazio: ignora e continua esperando
            if (f.data[1] == 0x7F) {
                if (maxLen < 2) return TIMEOUT;
                outBuf[0] = f.data[2];  // origService
                outBuf[1] = f.data[3];  // NRC
                return NEGATIVE_RESPONSE;
            }
            if (f.data[1] != expectedReplySid) continue;  // resposta de outra transação: ignora
            size_t toCopy = (size_t)len < maxLen ? (size_t)len : maxLen;
            memcpy(outBuf, &f.data[1], toCopy);
            return (int)toCopy;
        }

        if (pciType == N_PCI_FF) {
            if (f.data[2] != expectedReplySid) continue;  // idem, resposta de outra transação

            uint32_t rxId    = f.id;
            uint32_t fcTargetId = physicalRequestIdFor(rxId);
            uint16_t totalLen = (uint16_t)(((f.data[0] & 0x0F) << 8) | f.data[1]);
            if (totalLen < 8) return TIMEOUT;  // FF malformado

            size_t toCopyFirst = (size_t)6 < maxLen ? 6 : maxLen;
            memcpy(outBuf, &f.data[2], toCopyFirst);
            size_t assembled = 6;

            sendFc(can, fcTargetId, FS_CTS, /*bs=*/0, /*stmin=*/0);

            uint8_t expectedSeq = 1;
            while (assembled < totalLen) {
                uint32_t cfDeadline = millis() + N_CR_MS;
                CanFrame cf;
                bool     got = false;
                while (millis() < cfDeadline) {
                    if (!can->receive(cf)) { taskYIELD(); continue; }
                    if (cf.id != rxId) continue;
                    if ((cf.data[0] & 0xF0) != N_PCI_CF) continue;
                    got = true;
                    break;
                }
                if (!got) return TIMEOUT;

                uint8_t seq = cf.data[0] & 0x0F;
                if (seq != (expectedSeq & 0x0F)) return TIMEOUT;

                size_t  remaining = totalLen - assembled;
                uint8_t chunk     = (uint8_t)(remaining < 7 ? remaining : 7);
                if (assembled < maxLen) {
                    size_t toCopy = (assembled + chunk <= maxLen) ? chunk : (maxLen - assembled);
                    memcpy(outBuf + assembled, &cf.data[1], toCopy);
                }
                assembled  += chunk;
                expectedSeq = (uint8_t)((expectedSeq + 1) & 0x0F);
            }

            return (int)((size_t)totalLen < maxLen ? totalLen : maxLen);
        }

        // FC ou CF fora de contexto (sem FF em andamento): ignora.
    }
}

}  // namespace IsoTp
