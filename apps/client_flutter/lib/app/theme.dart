import 'package:flutter/material.dart';

ThemeData buildLightTheme() => ThemeData(
  colorScheme: ColorScheme.fromSeed(seedColor: const Color(0xFFB42318)),
  useMaterial3: true,
);

ThemeData buildDarkTheme() => ThemeData(
  colorScheme: ColorScheme.fromSeed(
    seedColor: const Color(0xFFE85D4A),
    brightness: Brightness.dark,
  ),
  useMaterial3: true,
);

