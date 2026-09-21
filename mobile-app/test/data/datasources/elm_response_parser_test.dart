import 'package:flutter_test/flutter_test.dart';
import 'package:tccelta_mobile/src/data/datasources/elm_response_parser.dart';
import 'package:tccelta_mobile/src/domain/obd2/elm_response.dart';

void main() {
  group('ElmResponseParser', () {
    test('parses a headerless single-frame PID response', () {
      final result = ElmResponseParser.parse(
        '41 0C 17 70',
        responseService: 0x41,
        pid: 0x0C,
      );

      expect(result.completion, ElmCompletionReason.prompt);
      expect(result.responses, hasLength(1));
      expect(result.responses.single.ecuId, isNull);
      expect(result.responses.single.payload, [0x17, 0x70]);
    });

    test('parses ECU headers and preserves multiple lines', () {
      final result = ElmResponseParser.parse(
        '7E8 06 41 0C 17 70\n7E9 06 41 0C 18 00',
        responseService: 0x41,
        pid: 0x0C,
      );

      expect(result.responses.map((response) => response.ecuId), [0x7E8, 0x7E9]);
      expect(result.responses.map((response) => response.payload), [
        [0x17, 0x70],
        [0x18, 0x00],
      ]);
    });

    test('keeps the DTC count byte in the service payload', () {
      final result = ElmResponseParser.parse(
        '7E8 06 43 01 03 01',
        responseService: 0x43,
      );

      expect(result.responses.single.ecuId, 0x7E8);
      expect(result.responses.single.payload, [1, 3, 1]);
    });

    test('distinguishes no data and rejected responses', () {
      expect(
        ElmResponseParser.parse(
          'NO DATA',
          responseService: 0x41,
          pid: 0x0C,
        ).completion,
        ElmCompletionReason.noData,
      );
      expect(
        ElmResponseParser.parse(
          '?',
          responseService: 0x41,
          pid: 0x0C,
        ).completion,
        ElmCompletionReason.rejected,
      );
    });
  });
}
