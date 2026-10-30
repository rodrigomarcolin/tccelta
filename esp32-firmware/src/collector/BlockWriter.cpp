#include "collector/BlockWriter.h"
#include "collector/Events.h"
#include "collector/MarkerBlock.h"
#include "collector/SegmentFormat.h"

namespace collector {

namespace {
uint32_t elapsedUs(const IClock& clock, uint64_t since) { return static_cast<uint32_t>(clock.nowUs() - since); }
}  // namespace

bool BlockWriter::writeBootMarker(uint32_t fromSeq, uint32_t toSeq, WriteReport& report) {
    _bootFrom = fromSeq;
    _bootTo = toSeq;
    _bootPending = true;
    report = {};
    report.boot = true;
    // O BOOT abre sempre um segmento novo: o salto de seq do boot fica entre segmentos (I1).
    _store.startNewSegment();
    buildBootMarker(_boot, _reserve.peek(), _bootId, _clock.nowMs(), fromSeq, toSeq);
    if (!commit(_boot, report)) return false;
    _bootPending = false;
    return true;
}

bool BlockWriter::write(Block& block, WriteReport& report) {
    report = {};
    if (_bootPending) {
        WriteReport bootReport;
        if (!writeBootMarker(_bootFrom, _bootTo, bootReport)) {
            // Nenhum dado pode receber seq antes do BOOT.
            ++_failedBlocks;
            report.attempts = bootReport.attempts;
            return false;
        }
    }
    return commit(block, report);
}

bool BlockWriter::commit(Block& block, WriteReport& report) {
    const uint32_t seq = _reserve.peek();
    report.seq = seq;

    // Reserva na NVS antes da flash: sem ela o invariante "todo seq gravado < reserva" quebraria.
    if (_reserve.needsReserve(seq)) {
        const uint64_t t0 = _clock.nowUs();
        const uint32_t value = _reserve.reserveFor(seq);
        const bool saved = _persistence.saveSeqReserve(value);
        report.reserveUs = elapsedUs(_clock, t0);
        if (!saved) {
            ++_failedBlocks;
            return false;
        }
        _reserve.reserved(value);
        report.reserved = true;
    }

    stampHeader(block, seq, _bootId);

    // Assina e calcula o CRC uma vez; a nova tentativa grava os mesmos bytes.
    uint64_t t0 = _clock.nowUs();
    block.tagLength = static_cast<uint8_t>(_auth.tagSize());
    bool signed_ = false;
    for (int attempt = 1; attempt <= kSignAttempts && !signed_; ++attempt) {
        signed_ = _auth.sign(block.bytes, block.length, block.tag);
        if (!signed_) {
            Event("SIGN_FAIL").token("kind", "block").num("seq", seq).num("attempt", attempt)
                .num("rc", _auth.lastError());
        }
    }
    report.signUs = elapsedUs(_clock, t0);
    if (!signed_) {
        report.signOk = false;
        ++_failedBlocks;  // como a falha de gravação: o seq não é consumido
        return false;
    }

    t0 = _clock.nowUs();
    block.crc = computeCrc(block);
    report.crcUs = elapsedUs(_clock, t0);

    t0 = _clock.nowUs();
    for (int attempt = 0; attempt < kAppendAttempts; ++attempt) {
        report.attempts = attempt + 1;
        if (attempt > 0) _store.startNewSegment();  // a gravação que falhou selou o segmento (I1)
        if (!_store.append(block)) continue;
        report.writeUs = elapsedUs(_clock, t0);
        _reserve.commit();
        report.ok = true;
        return true;
    }
    report.writeUs = elapsedUs(_clock, t0);
    ++_failedBlocks;
    return false;
}

}  // namespace collector
