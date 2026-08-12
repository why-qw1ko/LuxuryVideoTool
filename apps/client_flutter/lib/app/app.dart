import 'package:douyin_capture/app/theme.dart';
import 'package:douyin_capture/core/version/app_version.dart';
import 'package:douyin_capture/features/bootstrap/presentation/bootstrap_page.dart';
import 'package:flutter/material.dart';

class DouyinCaptureApp extends StatelessWidget {
  const DouyinCaptureApp({super.key});

  @override
  Widget build(BuildContext context) {
    return MaterialApp(
      title: 'Douyin Capture',
      debugShowCheckedModeBanner: false,
      theme: buildLightTheme(),
      darkTheme: buildDarkTheme(),
      home: const BootstrapPage(version: AppVersion.current),
    );
  }
}

