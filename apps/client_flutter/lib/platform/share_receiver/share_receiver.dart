import 'dart:async';

import 'package:flutter/services.dart';

class ShareReceiver {
  ShareReceiver() { _channel.setMethodCallHandler(_onMethodCall); }
  static const _channel = MethodChannel('com.whysoftware.douyincapture/share');
  final StreamController<String> _controller = StreamController<String>.broadcast();
  Stream<String> get texts => _controller.stream;

  Future<void> initialise() async {
    final initial = await _channel.invokeMethod<String>('getInitialShare');
    if (initial != null && initial.trim().isNotEmpty) _controller.add(initial.trim());
  }

  Future<void> _onMethodCall(MethodCall call) async {
    if (call.method == 'onShareText' && call.arguments is String) _controller.add((call.arguments as String).trim());
  }

  Future<void> dispose() => _controller.close();
}
