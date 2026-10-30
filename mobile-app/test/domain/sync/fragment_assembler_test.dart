import 'dart:typed_data';

import 'package:flutter_test/flutter_test.dart';
import 'package:tccelta_mobile/src/domain/sync/fragment_assembler.dart';

import '../../support/sync_known_vectors.dart';

Uint8List _frag(int control, List<int> data) =>
    Uint8List.fromList([control, ...data]);

void main() {
  late FragmentAssembler assembler;
  const t0 = Duration.zero;

  setUp(() => assembler = FragmentAssembler());

  test('vetor de 3 fragmentos remonta a resposta de 53 B', () {
    final f = SyncVectors.responseFragments;
    expect(assembler.add(f[0], t0).single, isA<AssemblyPending>());
    expect(assembler.assembling, isTrue);
    expect(assembler.add(f[1], t0).single, isA<AssemblyPending>());
    final done = assembler.add(f[2], t0).single as AssemblyComplete;
    expect(done.tlv.length, 53);
    expect(done.tlv.sublist(0, 3), [0x02, 0x32, 0x00]);
    expect(assembler.assembling, isFalse);
  });

  test('mensagem de um fragmento', () {
    final out = assembler.add(
      [0x80, 0x06, 0x01, 0x00, 0x04],
      t0,
    );
    expect((out.single as AssemblyComplete).tlv, [0x06, 0x01, 0x00, 0x04]);
  });

  test('fragmento vazio é ignorado', () {
    expect(assembler.add(const [], t0), isEmpty);
  });

  test('início com contador diferente de zero é counterGap', () {
    final out = assembler.add([0x81, 0x06], t0).single as AssemblyError;
    expect(out.kind, AssemblyErrorKind.counterGap);
  });

  test('salto de contador descarta a montagem', () {
    final f = SyncVectors.responseFragments;
    assembler.add(f[0], t0);
    final out =
        assembler.add(_frag(0x02, f[2].sublist(1)), t0).single as AssemblyError;
    expect(out.kind, AssemblyErrorKind.counterGap);
    expect(assembler.assembling, isFalse);
  });

  test('continuação sem início', () {
    final out = assembler.add([0x01, 0xAA], t0).single as AssemblyError;
    expect(out.kind, AssemblyErrorKind.continuationWithoutStart);
  });

  test('início durante montagem dá duas saídas', () {
    assembler.add(SyncVectors.responseFragments[0], t0);
    final out = assembler.add([0x80, 0x06, 0x01, 0x00, 0x04], t0);
    expect(out, hasLength(2));
    expect(
      (out[0] as AssemblyError).kind,
      AssemblyErrorKind.startWhileAssembling,
    );
    expect(out[1], isA<AssemblyComplete>());
  });

  test('L acima do máximo é tooLong', () {
    final out =
        assembler.add([0x80, 0x03, 0x41, 0x04], t0).single as AssemblyError;
    expect(out.kind, AssemblyErrorKind.tooLong);
  });

  test('bytes além de 3 + L é overflow', () {
    final out =
        assembler.add([0x80, 0x06, 0x01, 0x00, 0x04, 0x05], t0).single
            as AssemblyError;
    expect(out.kind, AssemblyErrorKind.overflow);
  });

  test('timeout de 2 s durante a montagem', () {
    assembler.add(SyncVectors.responseFragments[0], t0);
    expect(assembler.checkTimeout(const Duration(milliseconds: 1999)), isNull);
    final out = assembler.checkTimeout(const Duration(seconds: 2));
    expect((out! as AssemblyError).kind, AssemblyErrorKind.timeout);
    expect(assembler.assembling, isFalse);
    expect(assembler.checkTimeout(const Duration(seconds: 10)), isNull);
  });

  test('contador volta de 127 para 0', () {
    // L = 400: início com 3 B de cabeçalho + 16 B, depois 127 fragmentos.
    final body = List<int>.generate(403, (i) => i & 0xFF);
    body[0] = 0x03;
    body[1] = 0x90;
    body[2] = 0x01; // L = 400
    var off = 0;
    var i = 0;
    AssemblyOutcome? last;
    while (off < body.length) {
      final n = (off + 3 > body.length) ? body.length - off : 3;
      final control = (i == 0 ? 0x80 : 0) | (i & 0x7F);
      last = assembler.add(_frag(control, body.sublist(off, off + n)), t0).last;
      off += n;
      i++;
    }
    expect(i, greaterThan(128));
    expect(last, isA<AssemblyComplete>());
    expect((last! as AssemblyComplete).tlv, body);
  });
}
