import 'package:flutter/material.dart';

ThemeData buildLightTheme() => ThemeData(
  colorScheme: ColorScheme.fromSeed(seedColor: const Color(0xFF6750A4)),
  useMaterial3: true,
);

ThemeData buildDarkTheme() => ThemeData(
  colorScheme: ColorScheme.fromSeed(
    seedColor: const Color(0xFF6750A4),
    brightness: Brightness.dark,
  ),
  useMaterial3: true,
);

