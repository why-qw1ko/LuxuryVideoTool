import 'dart:convert';

import 'package:shared_preferences/shared_preferences.dart';

enum AppThemeMode { system, light, dark }

class ClientSettings {
  const ClientSettings({
    this.serverUrl = 'http://127.0.0.1:8080',
    this.downloadDirectory,
    this.notificationsEnabled = true,
    this.themeMode = AppThemeMode.system,
  });

  factory ClientSettings.fromJson(Map<String, dynamic> json) => ClientSettings(
    serverUrl: json['serverUrl'] as String? ?? 'http://127.0.0.1:8080',
    downloadDirectory: json['downloadDirectory'] as String?,
    notificationsEnabled: json['notificationsEnabled'] as bool? ?? true,
    themeMode: AppThemeMode.values.firstWhere(
      (value) => value.name == json['themeMode'],
      orElse: () => AppThemeMode.system,
    ),
  );

  final String serverUrl;
  final String? downloadDirectory;
  final bool notificationsEnabled;
  final AppThemeMode themeMode;

  ClientSettings copyWith({String? serverUrl, String? downloadDirectory, bool? notificationsEnabled, AppThemeMode? themeMode}) => ClientSettings(
    serverUrl: serverUrl ?? this.serverUrl,
    downloadDirectory: downloadDirectory ?? this.downloadDirectory,
    notificationsEnabled: notificationsEnabled ?? this.notificationsEnabled,
    themeMode: themeMode ?? this.themeMode,
  );

  Map<String, dynamic> toJson() => <String, dynamic>{
    'serverUrl': serverUrl,
    'downloadDirectory': downloadDirectory,
    'notificationsEnabled': notificationsEnabled,
    'themeMode': themeMode.name,
  };
}

class SettingsStore {
  static const _settingsKey = 'client_settings_v1';
  static const _jobsKey = 'job_cache_v1';
  final SharedPreferencesAsync _preferences = SharedPreferencesAsync();

  Future<ClientSettings> read() async {
    final value = await _preferences.getString(_settingsKey);
    if (value == null) return const ClientSettings();
    try { return ClientSettings.fromJson(jsonDecode(value) as Map<String, dynamic>); } on FormatException { return const ClientSettings(); }
  }

  Future<void> write(ClientSettings settings) => _preferences.setString(_settingsKey, jsonEncode(settings.toJson()));
  Future<List<Map<String, dynamic>>> readJobs() async {
    final value = await _preferences.getString(_jobsKey);
    if (value == null) return <Map<String, dynamic>>[];
    try { return (jsonDecode(value) as List<dynamic>).map((item) => Map<String, dynamic>.from(item as Map<dynamic, dynamic>)).toList(); } on Object { return <Map<String, dynamic>>[]; }
  }
  Future<void> writeJobs(List<Map<String, dynamic>> jobs) => _preferences.setString(_jobsKey, jsonEncode(jobs.take(100).toList()));
  Future<void> clearUserCache() => _preferences.remove(_jobsKey);
}
