import 'package:douyin_capture/features/bootstrap/presentation/bootstrap_page.dart';
import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';

void main() {
  testWidgets('renders the M5 bootstrap while stores initialise', (WidgetTester tester) async {
    await tester.pumpWidget(const MaterialApp(home: BootstrapPage(version: 'test')));

    expect(find.text('Douyin Capture'), findsOneWidget);
  });
}

