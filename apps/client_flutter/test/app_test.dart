import 'package:douyin_capture/app/app.dart';
import 'package:flutter_test/flutter_test.dart';

void main() {
  testWidgets('renders the M0 baseline', (WidgetTester tester) async {
    await tester.pumpWidget(const DouyinCaptureApp());

    expect(find.text('Douyin Capture'), findsOneWidget);
    expect(find.text('工程基线已就绪'), findsOneWidget);
  });
}

