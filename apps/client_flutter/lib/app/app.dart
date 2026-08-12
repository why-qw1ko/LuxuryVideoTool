import 'package:douyin_capture/app/app_state.dart';
import 'package:douyin_capture/app/router.dart';
import 'package:douyin_capture/app/theme.dart';
import 'package:douyin_capture/core/version/app_version.dart';
import 'package:douyin_capture/core/storage/settings_store.dart';
import 'package:douyin_capture/features/bootstrap/presentation/bootstrap_page.dart';
import 'package:flutter/material.dart';

class DouyinCaptureApp extends StatefulWidget {
  const DouyinCaptureApp({super.key});
  @override State<DouyinCaptureApp> createState() => _DouyinCaptureAppState();
}
class _DouyinCaptureAppState extends State<DouyinCaptureApp> {
  late final AppState _state = AppState();
  late final _router = buildRouter(_state);
  @override void initState() { super.initState(); _state.addListener(_changed); _state.initialise(); }
  void _changed() { if (mounted) setState(() {}); }
  @override Widget build(BuildContext context) {
    final mode = switch (_state.settings.themeMode) { AppThemeMode.light => ThemeMode.light, AppThemeMode.dark => ThemeMode.dark, AppThemeMode.system => ThemeMode.system };
    if (_state.initialising) return MaterialApp(debugShowCheckedModeBanner: false, theme: buildLightTheme(), darkTheme: buildDarkTheme(), home: const BootstrapPage(version: AppVersion.current));
    return MaterialApp.router(title: 'Douyin Capture', debugShowCheckedModeBanner: false, theme: buildLightTheme(), darkTheme: buildDarkTheme(), themeMode: mode, routerConfig: _router);
  }
  @override void dispose() { _state.removeListener(_changed); _state.dispose(); super.dispose(); }
}
