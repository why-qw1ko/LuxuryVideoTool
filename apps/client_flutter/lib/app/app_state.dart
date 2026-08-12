import 'dart:async';
import 'dart:io';

import 'package:douyin_capture/core/api/api_client.dart';
import 'package:douyin_capture/core/auth/token_store.dart';
import 'package:douyin_capture/core/errors/app_failure.dart';
import 'package:douyin_capture/core/storage/settings_store.dart';
import 'package:douyin_capture/core/version/app_version.dart';
import 'package:douyin_capture/features/jobs/data/jobs_repository.dart';
import 'package:douyin_capture/features/jobs/domain/job.dart';
import 'package:douyin_capture/platform/downloads/download_service.dart';
import 'package:douyin_capture/platform/notifications/notification_service.dart';
import 'package:douyin_capture/platform/share_receiver/share_receiver.dart';
import 'package:flutter/foundation.dart';

class AppState extends ChangeNotifier {
  AppState({TokenStore? tokenStore, SettingsStore? settingsStore})
    : tokens = tokenStore ?? TokenStore(),
      settingsStore = settingsStore ?? SettingsStore();

  final TokenStore tokens;
  final SettingsStore settingsStore;
  final ShareReceiver shareReceiver = ShareReceiver();
  final DownloadService downloads = DownloadService();
  final NotificationService notifications = NotificationService();
  ClientSettings settings = const ClientSettings();
  AuthSession? session;
  late ApiClient api;
  late JobsRepository jobs;
  bool initialising = true;
  String? pendingShareText;
  String? notificationJobId;
  StreamSubscription<String>? _shareSubscription;

  bool get authenticated => session != null;

  Future<void> initialise() async {
    settings = await settingsStore.read();
    session = await tokens.read();
    _rebuildApi();
    _shareSubscription = shareReceiver.texts.listen((text) { pendingShareText = text; notifyListeners(); });
    try { await shareReceiver.initialise(); } on Exception { /* non-Android platforms do not provide the channel */ }
    try { await notifications.initialise((id) { notificationJobId = id; notifyListeners(); }); } on Exception { /* notification support is optional at runtime */ }
    if (session != null && session!.refreshExpiresAt.isBefore(DateTime.now())) { await tokens.clear(); session = null; }
    initialising = false;
    notifyListeners();
  }

  void _rebuildApi() {
    api = ApiClient(baseUrl: settings.serverUrl, tokenStore: tokens, onSessionChanged: (value) async { session = value; notifyListeners(); });
    jobs = JobsRepository(api: api, cache: settingsStore);
  }

  Future<void> login(String username, String password) async {
    final deviceId = await tokens.deviceId();
    final data = await api.request('POST', '/api/v1/auth/login', authenticated: false, data: <String, dynamic>{
      'username': username.trim(),
      'password': password,
      'device': <String, dynamic>{
        'id': deviceId,
        'name': Platform.localHostname.isEmpty ? (Platform.isAndroid ? 'Android 设备' : 'Windows 电脑') : Platform.localHostname,
        'platform': Platform.isAndroid ? 'android' : 'windows',
        'appVersion': AppVersion.current,
      },
    });
    session = AuthSession.fromJson(data);
    await tokens.write(session!);
    notifyListeners();
  }

  Future<void> logout() async {
    try { if (session != null) await api.request('POST', '/api/v1/auth/logout'); } on AppFailure { /* local logout must always succeed */ }
    await tokens.clear();
    await settingsStore.clearUserCache();
    session = null;
    notifyListeners();
  }

  Future<void> updateSettings(ClientSettings value) async {
    final serverChanged = settings.serverUrl != value.serverUrl;
    settings = value;
    await settingsStore.write(value);
    if (serverChanged) {
      await tokens.clear();
      await settingsStore.clearUserCache();
      session = null;
    }
    _rebuildApi();
    notifyListeners();
  }
  String? takePendingShare() { final value = pendingShareText; pendingShareText = null; return value; }
  String? takeNotificationJob() { final value = notificationJobId; notificationJobId = null; return value; }

  @override
  void dispose() { _shareSubscription?.cancel(); shareReceiver.dispose(); super.dispose(); }
}
